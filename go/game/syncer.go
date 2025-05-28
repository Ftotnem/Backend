package main

import (
	"context" // Import fmt for string formatting
	"log"
	"time"

	"github.com/Ftotnem/Backend/go/shared/service" // Import your shared player service client
)

// PlaytimeSyncer handles the periodic synchronization by calling the player service.
type PlaytimeSyncer struct {
	redisClient         *RedisClient // Assuming RedisClient has Set method (e.g., from go-redis)
	playerServiceClient *service.PlayerServiceClient
	syncInterval        time.Duration // How often to run the sync job (e.g., 1 minute)
	ctx                 context.Context
	cancel              context.CancelFunc
}

// NewPlaytimeSyncer creates a new instance of PlaytimeSyncer.
func NewPlaytimeSyncer(rc *RedisClient, psc *service.PlayerServiceClient, interval time.Duration) *PlaytimeSyncer {
	ctx, cancel := context.WithCancel(context.Background())
	return &PlaytimeSyncer{
		redisClient:         rc,
		playerServiceClient: psc,
		syncInterval:        interval,
		ctx:                 ctx,
		cancel:              cancel,
	}
}

// Start initiates the synchronization loop. This should be run in a goroutine.
func (ps *PlaytimeSyncer) Start() {
	log.Printf("Playtime Syncer starting with sync interval: %v", ps.syncInterval)
	ticker := time.NewTicker(ps.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ps.ctx.Done():
			log.Println("Playtime Syncer shutting down.")
			return
		case <-ticker.C:
			ps.triggerPlayerServiceSync()
		}
	}
}

// Stop gracefully stops the synchronization loop.
func (ps *PlaytimeSyncer) Stop() {
	ps.cancel()
}

// triggerPlayerServiceSync calls the player service to perform the actual playtime sync
// and then updates Redis with the returned team totals.
func (ps *PlaytimeSyncer) triggerPlayerServiceSync() {
	log.Println("Triggering player service to synchronize player playtimes and get team totals...")

	syncCtx, syncCancel := context.WithTimeout(context.Background(), 30*time.Second) // Give player service ample time
	defer syncCancel()

	// This call now expects a response containing the team totals
	resp, err := ps.playerServiceClient.SyncPlayerPlaytime(syncCtx)
	if err != nil {
		log.Printf("ERROR: Failed to trigger player service playtime sync or get team totals: %v", err)
		return // Exit if we couldn't get the data
	}

	log.Println("Successfully triggered player service playtime sync. Updating Redis with team totals...")

	if resp.TeamTotals == nil {
		log.Println("No team totals received from player service sync.")
		return
	}

	// Update Redis with the received team totals
	for teamID, totalPlaytime := range resp.TeamTotals {

		err := ps.redisClient.SetTeamTotal(syncCtx, teamID, totalPlaytime) // Set with no expiration
		if err != nil {
			log.Printf("ERROR: Failed to update Redis for team %s total playtime: %v", teamID, err)
		} else {
			log.Printf("INFO: Successfully updated Redis total playtime for team '%s' to %.2f ticks.", teamID, totalPlaytime)
		}
	}
	log.Println("Finished updating Redis with aggregated team totals.")
}
