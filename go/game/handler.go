package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Ftotnem/Backend/go/shared/api" // Import your shared API module as 'api'
	"github.com/gorilla/mux"                   // Still needed for mux.Vars
)

// GameService holds dependencies for HTTP handlers (like the RedisClient)
type GameService struct {
	redisClient *RedisClient
	config      *Config // To access config values like RedisOnlineTTL if needed by handlers
}

// BanRequest is the structure for the request body for banning/unbanning.
type BanRequest struct {
	UUID        string `json:"uuid"`
	DurationSec int64  `json:"duration_seconds"` // Duration in seconds. 0 for permanent, -1 to unban.
	Reason      string `json:"reason,omitempty"`
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

// HandleBanPlayer handles requests to ban a player.
// POST /game/ban
// Body: { "uuid": "<player_uuid>", "duration_seconds": <seconds>, "reason": "..." }
func (gs *GameService) HandleBanPlayer(w http.ResponseWriter, r *http.Request) {
	var req BanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.UUID == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Determine expiration time based on duration_seconds
	var banExpiresAt int64
	if req.DurationSec <= 0 {
		banExpiresAt = 0 // Treat 0 or negative as permanent ban (no Redis TTL, relies on explicit DEL)
	} else {
		banExpiresAt = time.Now().Add(time.Duration(req.DurationSec) * time.Second).Unix()
	}

	// Set ban status in Redis (real-time check)
	err := gs.redisClient.SetBanStatus(ctx, req.UUID, true, banExpiresAt)
	if err != nil {
		log.Printf("Error setting ban status for player %s in Redis: %v", req.UUID, err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to ban player in Redis")
		return
	}

	// TODO: Integrate with MongoDB for long-term persistence here
	// This would involve calling a method on your MongoDB client/store
	// e.g., gs.mongoStore.SaveBan(ctx, req.UUID, banExpiresAt, req.Reason)
	// For now, we'll just log it.
	log.Printf("Player %s banned for %d seconds (expires %v) with reason: %s. (MongoDB persistence TODO)",
		req.UUID, req.DurationSec, time.Unix(banExpiresAt, 0), req.Reason)

	api.WriteJSON(w, http.StatusOK, map[string]string{
		"message":    fmt.Sprintf("Player %s banned until %v", req.UUID, time.Unix(banExpiresAt, 0)),
		"uuid":       req.UUID,
		"expires_at": strconv.FormatInt(banExpiresAt, 10),
	})
}

// HandleUnbanPlayer handles requests to unban a player.
// POST /game/unban
// Body: { "uuid": "<player_uuid>" }
func (gs *GameService) HandleUnbanPlayer(w http.ResponseWriter, r *http.Request) {
	var req OnlineStatusRequest // Re-use OnlineStatusRequest as it only needs UUID
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.UUID == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Remove ban status from Redis
	err := gs.redisClient.SetBanStatus(ctx, req.UUID, false, 0) // banned=false means DEL
	if err != nil {
		log.Printf("Error unbanning player %s in Redis: %v", req.UUID, err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to unban player in Redis")
		return
	}

	// TODO: Integrate with MongoDB to remove or mark ban as inactive here
	// e.g., gs.mongoStore.RemoveBan(ctx, req.UUID) or gs.mongoStore.MarkBanInactive(ctx, req.UUID)
	log.Printf("Player %s unbanned. (MongoDB persistence TODO)", req.UUID)

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": "Player unbanned", "uuid": req.UUID})
}
