package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/Ftotnem/Backend/go/shared/api" // Import your shared API module as 'api'
	"github.com/gorilla/mux"                   // Still needed for mux.Vars
)

// GameService holds dependencies for HTTP handlers (like the RedisClient)
type GameService struct {
	redisClient *RedisClient
	config      *Config // To access config values like RedisOnlineTTL if needed by handlers
}

// NewGameService creates a new GameService instance.
func NewGameService(rc *RedisClient, cfg *Config) *GameService {
	return &GameService{
		redisClient: rc,
		config:      cfg,
	}
}

// OnlineStatusRequest is the structure for the request body of /game/online and /game/offline.
type OnlineStatusRequest struct {
	UUID string `json:"uuid"`
}

// HandleOnline handles requests to mark a player as online.
// POST /game/online
// Body: { "uuid": "<player_uuid>" }
func (gs *GameService) HandleOnline(w http.ResponseWriter, r *http.Request) {
	var req OnlineStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.UUID == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second) // Use a timeout for Redis operations
	defer cancel()

	err := gs.redisClient.SetOnlineStatus(ctx, req.UUID)
	if err != nil {
		log.Printf("Error setting player %s online status: %v", req.UUID, err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to set player online status")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": "Player set online", "uuid": req.UUID})
}

// HandleOffline handles requests to mark a player as offline.
// POST /game/offline
// Body: { "uuid": "<player_uuid>" }
func (gs *GameService) HandleOffline(w http.ResponseWriter, r *http.Request) {
	var req OnlineStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.UUID == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second) // Use a timeout for Redis operations
	defer cancel()

	err := gs.redisClient.SetOfflineStatus(ctx, req.UUID)
	if err != nil {
		log.Printf("Error setting player %s offline status: %v", req.UUID, err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to set player offline status")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": "Player set offline", "uuid": req.UUID})
}

// GetTeamTotals handles requests to retrieve the total playtime for all teams.
// GET /game/teams/total
func (gs *GameService) GetTeamTotals(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second) // Use a timeout for Redis operations
	defer cancel()

	teamTotals, err := gs.redisClient.GetAllTeamTotalPlaytimes(ctx)
	if err != nil {
		log.Printf("Error retrieving team total playtimes: %v", err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to retrieve team total playtimes")
		return
	}

	// For a cleaner output, you might want to convert the map to a slice of structs
	// if you have a fixed number of teams or want a more structured response.
	// For now, a direct map is fine.
	api.WriteJSON(w, http.StatusOK, teamTotals)
}

// Example usage of a player-specific GET endpoint if needed (e.g., check online status)
// This is not explicitly in the prompt but could be useful for debugging/monitoring
func (gs *GameService) GetPlayerOnlineStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	isOnline, err := gs.redisClient.IsOnline(ctx, uuid)
	if err != nil {
		log.Printf("Error checking online status for %s: %v", uuid, err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to check player online status")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"uuid":     uuid,
		"isOnline": isOnline,
	})
}
