// updater.go
package main

import (
	"context"
	"log"
	"time"
)

// GameUpdater handles the periodic updates for online players' playtime.
type GameUpdater struct {
	redisClient *RedisClient // Assuming you pass your RedisClient here
	config      *Config      // Game service configuration
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewGameUpdater creates a new instance of GameUpdater.
func NewGameUpdater(rc *RedisClient, cfg *Config) *GameUpdater {
	ctx, cancel := context.WithCancel(context.Background())
	return &GameUpdater{
		redisClient: rc,
		config:      cfg,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start initiates the game update loop. This should be run in a goroutine.
func (gu *GameUpdater) Start() {
	log.Printf("Game Updater starting with tick interval: %v", gu.config.TickInterval)
	ticker := time.NewTicker(gu.config.TickInterval)
	defer ticker.Stop()

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

// performGameTick executes the logic for a single game tick.
func (gu *GameUpdater) performGameTick() {

	onlineUUIDs, err := gu.redisClient.GetAllOnlineUUIDs(gu.ctx)
	if err != nil {
		log.Printf("Error getting online UUIDs for game tick: %v", err)
		return
	}

	if len(onlineUUIDs) == 0 {
		//log.Println("No players online to update playtime.") // Uncomment for verbose debugging if needed
		return
	}

	//log.Printf("Performing game tick for %d online players. Adding %.4f seconds playtime.", len(onlineUUIDs), ticksToApply)

	for _, uuid := range onlineUUIDs {
		// Increment both total playtime and delta playtime
		if err := gu.redisClient.IncrementPlayerPlaytime(gu.ctx, uuid); err != nil {
			log.Printf("Error incrementing total playtime for %s: %v", uuid, err)
		}
	}
}
