package main

import (
	"context"
	"log"
	"time"

	"sync" // For mutex to protect the consistent hash ring

	cluster "github.com/Ftotnem/Backend/go/shared/cluster" // Your cluster package
	"github.com/stathat/consistent"                        // Import the consistent hashing library
)

// GameUpdater handles the periodic updates for online players' playtime.
type GameUpdater struct {
	redisClient *RedisClient // Assuming you pass your RedisClient here
	config      *Config      // Game service configuration
	registrar   *cluster.ServiceRegistrar
	ctx         context.Context
	cancel      context.CancelFunc

	// New fields for consistent hashing
	consistentHash *consistent.Consistent
	chMux          sync.RWMutex // Protects access to consistentHash
	myServiceID    string       // The ID of *this* game service instance
}

// NewGameUpdater creates a new GameUpdater instance.
func NewGameUpdater(redisClient *RedisClient, cfg *Config, registrar *cluster.ServiceRegistrar) *GameUpdater {
	log.Println("GameUpdater: Initialized with ServiceRegistrar.")
	ctx, cancel := context.WithCancel(context.Background())

	gu := &GameUpdater{
		redisClient:    redisClient,
		config:         cfg,
		registrar:      registrar,
		ctx:            ctx,
		cancel:         cancel,
		consistentHash: consistent.New(),         // Initialize the consistent hash ring
		myServiceID:    registrar.GetServiceID(), // Get this instance's ID
	}
	log.Printf("DEBUG: Configured TickInterval before updater start: %v", gu.config.TickInterval)
	return gu
}

// Start initiates the game update loop. This should be run in a goroutine.
func (gu *GameUpdater) Start() {
	log.Printf("Game Updater starting with tick interval: %v", gu.config.TickInterval)
	ticker := time.NewTicker(gu.config.TickInterval)
	defer ticker.Stop()

	// Start a goroutine to periodically update the consistent hash ring
	gu.chMux.Lock()
	gu.consistentHash.Add(gu.myServiceID) // Add self to the ring initially
	gu.chMux.Unlock()
	go gu.updateConsistentHashLoop() // New: Goroutine to keep the ring updated

	for {
		select {
		case <-gu.ctx.Done():
			log.Println("Game Updater shutting down.")
			return
		case <-ticker.C:
			gu.performGameTick()
		}
	}
}

// Stop gracefully stops the game update loop.
func (gu *GameUpdater) Stop() {
	gu.cancel()
}

// updateConsistentHashLoop periodically fetches active services and updates the consistent hash ring.
func (gu *GameUpdater) updateConsistentHashLoop() {
	// A reasonable interval for checking for new/removed services.
	// This should be longer than your HeartbeatInterval to avoid thrashing.
	checkInterval := gu.registrar.config.HeartbeatInterval * 2 // Example: twice the heartbeat
	if checkInterval == 0 {                                    // Fallback if heartbeat is 0 (shouldn't happen with defaults)
		checkInterval = 10 * time.Second
	}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	log.Printf("GameUpdater: Consistent Hash Updater starting with check interval: %v", checkInterval)

	for {
		select {
		case <-gu.ctx.Done():
			log.Println("GameUpdater: Consistent Hash Updater shutting down.")
			return
		case <-ticker.C:
			gu.updateConsistentHashRing()
		}
	}
}

// updateConsistentHashRing fetches current active game services and rebuilds the consistent hash ring.
func (gu *GameUpdater) updateConsistentHashRing() {
	activeServices, err := gu.registrar.GetActiveServices(gu.ctx, gu.registrar.GetConfig().ServiceType) // Use ServiceType from config
	if err != nil {
		log.Printf("ERROR: GameUpdater: Failed to get active game services for consistent hash: %v", err)
		return
	}

	members := make([]string, 0, len(activeServices))
	for id := range activeServices {
		members = append(members, id)
	}

	gu.chMux.Lock()
	defer gu.chMux.Unlock()

	// Get current members on the ring to compare
	currentMembers := gu.consistentHash.Members()

	// Check if members list has changed
	if !areStringSlicesEqual(members, currentMembers) {
		gu.consistentHash = consistent.New() // Create a new consistent hash instance
		for _, member := range members {
			gu.consistentHash.Add(member)
		}
		log.Printf("GameUpdater: Consistent Hash ring updated. Active members: %v", gu.consistentHash.Members())
	}
}

// Helper to compare string slices (order-independent)
func areStringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	mapA := make(map[string]bool)
	for _, x := range a {
		mapA[x] = true
	}
	for _, x := range b {
		if _, ok := mapA[x]; !ok {
			return false
		}
	}
	return true
}

// performGameTick executes the logic for a single game tick.
func (gu *GameUpdater) performGameTick() {
	onlineUUIDs, err := gu.redisClient.GetAllOnlineUUIDs(gu.ctx)
	if err != nil {
		log.Printf("Error getting online UUIDs for game tick: %v", err)
		return
	}

	if len(onlineUUIDs) == 0 {
		return
	}

	// Filter UUIDs using consistent hashing
	playersToUpdate := make([]string, 0, len(onlineUUIDs)/len(gu.consistentHash.Members())) // Estimate capacity
	gu.chMux.RLock()                                                                        // Read lock to access consistentHash
	defer gu.chMux.RUnlock()

	// Check if there are any members in the consistent hash ring
	if len(gu.consistentHash.Members()) == 0 {
		log.Println("WARNING: GameUpdater: Consistent hash ring is empty. Cannot determine player responsibility.")
		return
	}

	for _, uuid := range onlineUUIDs {
		// Determine which service is responsible for this UUID
		responsibleService, err := gu.consistentHash.Get(uuid)
		if err != nil {
			log.Printf("WARNING: GameUpdater: Failed to get responsible service for UUID %s: %v", uuid, err)
			continue
		}

		// If *this* service is responsible, add it to the list to update
		if responsibleService == gu.myServiceID {
			playersToUpdate = append(playersToUpdate, uuid)
		}
	}

	if len(playersToUpdate) == 0 {
		//log.Println("No players assigned to this instance for update in this tick.")
		return
	}

	log.Printf("Performing game tick for %d players assigned to this instance.", len(playersToUpdate))

	for _, uuid := range playersToUpdate {
		// Increment both total playtime and delta playtime
		if err := gu.redisClient.IncrementPlayerPlaytime(gu.ctx, uuid); err != nil {
			log.Printf("Error incrementing total playtime for %s: %v", uuid, err)
		}
	}
}
