package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Ftotnem/Backend/go/shared/api"     // Import your shared API module as 'api'
	"github.com/Ftotnem/Backend/go/shared/service" // Import the shared service client
	"github.com/gorilla/mux"                       // Still needed for mux.Vars
	"go.minekube.com/gate/pkg/util/uuid"           // Required for parsing UUIDs
	"go.mongodb.org/mongo-driver/mongo"            // To check for ErrNoDocuments
)

// GameService holds dependencies for HTTP handlers (like the RedisClient and PlayerServiceClient)
type GameService struct {
	redisClient         *RedisClient
	playerServiceClient *service.PlayerServiceClient // New: Client for Player Data Service
	config              *Config                      // To access config values like RedisOnlineTTL if needed by handlers
}

// BanRequest is the structure for the request body for banning/unbanning.
type BanRequest struct {
	UUID        string `json:"uuid"`
	DurationSec int64  `json:"duration_seconds"` // Duration in seconds. 0 for permanent, -1 to unban.
	Reason      string `json:"reason,omitempty"`
}

// NewGameService creates a new GameService instance.
func NewGameService(rc *RedisClient, psc *service.PlayerServiceClient, cfg *Config) *GameService {
	return &GameService{
		redisClient:         rc,
		playerServiceClient: psc, // Assign the new client
		config:              cfg,
	}
}

// OnlineStatusRequest is the structure for the request body of /game/online and /game/offline.
type OnlineStatusRequest struct {
	UUID string `json:"uuid"`
}

// PlaytimeResponse is the structure for the JSON response for playtime requests.
type PlaytimeResponse struct {
	Playtime float64 `json:"playtime"`
}

// DeltaPlaytimeResponse is the structure for the JSON response for delta playtime requests.
type DeltaPlaytimeResponse struct {
	Deltatime float64 `json:"deltatime"`
}

// --- NEW HANDLER METHODS START ---

// handleGetPlaytime handles requests to retrieve a player's total playtime from Redis.
// GET /playtime/{uuid}
func (gs *GameService) handleGetPlaytime(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	playerUUIDStr := vars["uuid"]
	if playerUUIDStr == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	playerUUID, err := uuid.Parse(playerUUIDStr)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid UUID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	playtime, err := gs.redisClient.GetPlayerPlaytime(ctx, playerUUID.String())
	if err != nil {
		if err == ErrRedisKeyNotFound { // Assuming you have a custom error for key not found
			api.WriteError(w, http.StatusNotFound, "Playtime data not found for player")
		} else {
			log.Printf("Error getting total playtime for %s from Redis: %v", playerUUID.String(), err)
			api.WriteError(w, http.StatusInternalServerError, "Failed to retrieve total playtime")
		}
		return
	}

	api.WriteJSON(w, http.StatusOK, PlaytimeResponse{Playtime: playtime})
}

// handleGetDeltaPlaytime handles requests to retrieve a player's delta playtime from Redis.
// GET /deltatime/{uuid}
func (gs *GameService) handleGetDeltaPlaytime(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	playerUUIDStr := vars["uuid"]
	if playerUUIDStr == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	playerUUID, err := uuid.Parse(playerUUIDStr)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid UUID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	deltaPlaytime, err := gs.redisClient.GetDeltaPlaytime(ctx, playerUUID.String())
	if err != nil {
		if err == ErrRedisKeyNotFound { // Assuming you have a custom error for key not found
			// Return 0.0 with 200 OK if delta playtime is not found, as per Java client expectation
			api.WriteJSON(w, http.StatusOK, DeltaPlaytimeResponse{Deltatime: 0.0})
		} else {
			log.Printf("Error getting delta playtime for %s from Redis: %v", playerUUID.String(), err)
			api.WriteError(w, http.StatusInternalServerError, "Failed to retrieve delta playtime")
		}
		return
	}

	api.WriteJSON(w, http.StatusOK, DeltaPlaytimeResponse{Deltatime: deltaPlaytime})
}

// --- NEW HANDLER METHODS END ---

// HandleOnline handles requests to mark a player as online.
// POST /game/online
// Body: { "uuid": "<player_uuid>" }
func (gs *GameService) HandleOnline(w http.ResponseWriter, r *http.Request) {
	var req OnlineStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	playerUUID, err := uuid.Parse(req.UUID)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid UUID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second) // Increased timeout for external service call
	defer cancel()

	// Check if player's playtime data already exists in Redis
	playtimeExists, deltaPlaytimeExists, err := gs.redisClient.CheckPlaytimeKeysExist(ctx, playerUUID.String())
	if err != nil {
		log.Printf("Error checking playtime keys for %s in Redis: %v", playerUUID.String(), err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to check player data status")
		return
	}

	if !playtimeExists || !deltaPlaytimeExists {
		log.Printf("Player %s is coming online for the first time this session. Loading/Initializing playtime data.", playerUUID.String())

		// Attempt to get player profile from Player Data Service
		profile, err := gs.playerServiceClient.GetProfile(ctx, playerUUID)
		if err != nil {
			if apiErr, ok := err.(*api.HTTPError); ok && apiErr.StatusCode == http.StatusNotFound {
				// Profile not found in MongoDB (player data service). Initialize with defaults.
				log.Printf("Profile for %s not found in Player Data Service. Initializing default playtime values in Redis.", playerUUID.String())
				err = gs.redisClient.SetPlayerPlaytime(ctx, playerUUID.String(), 0.0) // Default total playtime
				if err != nil {
					log.Printf("Error setting default total playtime for %s: %v", playerUUID.String(), err)
					api.WriteError(w, http.StatusInternalServerError, "Failed to set default playtime")
					return
				}
				err = gs.redisClient.SetDeltaPlaytime(ctx, playerUUID.String(), 1.0) // Default delta playtime
				if err != nil {
					log.Printf("Error setting default delta playtime for %s: %v", playerUUID.String(), err)
					api.WriteError(w, http.StatusInternalServerError, "Failed to set default delta playtime")
					return
				}
				// The client is responsible for creating the profile in MongoDB if needed.
				// This service just syncs with Redis or initializes local state.
			} else if mongo.IsDuplicateKeyError(err) {
				// This case should ideally not happen for a GET operation, but as a safeguard.
				log.Printf("WARN: Duplicate key error during GET for %s, this is unexpected: %v", playerUUID.String(), err)
				api.WriteError(w, http.StatusInternalServerError, "Unexpected error during profile retrieval")
				return
			} else {
				// Other errors getting profile from Player Data Service
				log.Printf("Error getting player profile %s from Player Data Service: %v", playerUUID.String(), err)
				api.WriteError(w, http.StatusInternalServerError, "Failed to retrieve player profile for playtime sync")
				return
			}
		} else {
			// Profile found in Player Data Service. Load existing values into Redis.
			log.Printf("Profile for %s found in Player Data Service. Loading playtime: %.2f, delta: %.2f into Redis.",
				playerUUID.String(), profile.TotalPlaytimeTicks, profile.DeltaPlaytimeTicks)
			err = gs.redisClient.SetPlayerPlaytime(ctx, playerUUID.String(), profile.TotalPlaytimeTicks)
			if err != nil {
				log.Printf("Error setting total playtime from DB for %s: %v", playerUUID.String(), err)
				api.WriteError(w, http.StatusInternalServerError, "Failed to set playtime from DB")
				return
			}
			err = gs.redisClient.SetDeltaPlaytime(ctx, playerUUID.String(), profile.DeltaPlaytimeTicks)
			if err != nil {
				log.Printf("Error setting delta playtime from DB for %s: %v", playerUUID.String(), err)
				api.WriteError(w, http.StatusInternalServerError, "Failed to set delta playtime from DB")
				return
			}
			// Also store the team if available in the profile
			if profile.Team != "" {
				err = gs.redisClient.SetPlayerTeam(ctx, playerUUID.String(), profile.Team)
				if err != nil {
					log.Printf("WARN: Failed to set player team %s for %s in Redis: %v", profile.Team, playerUUID.String(), err)
					// Not a critical error to prevent login, just log
				}
			}
		}
	} else {
		log.Printf("Player %s playtime data already in Redis for this session. Skipping DB load.", playerUUID.String())
	}

	// Mark player as online in Redis (always done after playtime sync)
	err = gs.redisClient.SetOnlineStatus(ctx, playerUUID.String())
	if err != nil {
		log.Printf("Error setting player %s online status: %v", playerUUID.String(), err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to set player online status")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": "Player set online", "uuid": playerUUID.String()})
	log.Printf("Player %s is now online.", playerUUID.String())
}

// HandleOffline handles requests to mark a player as offline and persist playtime.
// POST /game/offline
// Body: { "uuid": "<player_uuid>" }
func (gs *GameService) HandleOffline(w http.ResponseWriter, r *http.Request) {
	var req OnlineStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	playerUUID, err := uuid.Parse(req.UUID)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid UUID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second) // Increased timeout for external service call
	defer cancel()

	// 1. Get playtime and delta playtime from Redis
	totalPlaytime, deltaPlaytime, err := gs.redisClient.GetPlayerPlaytimeAndDelta(ctx, playerUUID.String())
	if err != nil {
		log.Printf("Error retrieving playtime for %s from Redis: %v", playerUUID.String(), err)
		// Don't fail here if data is missing, we still want to mark offline
		api.WriteError(w, http.StatusInternalServerError, "Failed to retrieve player playtime from Redis")
		return
	}

	// 2. Persist playtime to Player Data Service (MongoDB)
	// Only attempt to update if playtime data was actually retrieved from Redis
	if totalPlaytime > 0 || deltaPlaytime > 0 { // Check if there's *some* data to persist
		log.Printf("Persisting playtime for %s: Total=%.2f, Delta=%.2f", playerUUID.String(), totalPlaytime, deltaPlaytime)
		// Update total playtime in MongoDB
		err = gs.playerServiceClient.UpdateProfilePlaytime(ctx, playerUUID, totalPlaytime)
		if err != nil {
			log.Printf("Error updating total playtime for %s in Player Data Service: %v", playerUUID.String(), err)
			// Log and continue, don't block offline process for this.
		}

		// Update delta playtime in MongoDB (often reset to 0 after persistence on player data service side)
		err = gs.playerServiceClient.UpdateProfileDeltaPlaytime(ctx, playerUUID, deltaPlaytime)
		if err != nil {
			log.Printf("Error updating delta playtime for %s in Player Data Service: %v", playerUUID.String(), err)
			// Log and continue
		}

		// Update LastLoginAt in MongoDB
		err = gs.playerServiceClient.UpdateProfileLastLogin(ctx, playerUUID)
		if err != nil {
			log.Printf("Error updating last login for %s in Player Data Service: %v", playerUUID.String(), err)
			// Log and continue
		}
	} else {
		log.Printf("No playtime data found in Redis for %s to persist to Player Data Service.", playerUUID.String())
	}

	// 3. Remove player-specific Redis keys (playtime, deltatime, team, online status)
	// This ensures a fresh load from DB next session
	err = gs.redisClient.RemovePlayerSessionData(ctx, playerUUID.String())
	if err != nil {
		log.Printf("Error removing session data for player %s from Redis: %v", playerUUID.String(), err)
		// Log and continue, attempt to mark offline anyway
	}

	// Finally, mark player as offline
	err = gs.redisClient.SetOfflineStatus(ctx, playerUUID.String())
	if err != nil {
		log.Printf("Error setting player %s offline status: %v", playerUUID.String(), err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to set player offline status")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": "Player set offline", "uuid": playerUUID.String()})
	log.Printf("Player %s is now offline. Data persisted and Redis session keys cleared.", playerUUID.String())
}

// GetTeamTotals handles requests to retrieve the total playtime for all teams.
// GET /game/teams/total
func (gs *GameService) GetTeamTotals(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	teamTotals, err := gs.redisClient.GetAllTeamTotalPlaytimes(ctx)
	if err != nil {
		log.Printf("Error retrieving team total playtimes: %v", err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to retrieve team total playtimes")
		return
	}

	api.WriteJSON(w, http.StatusOK, teamTotals)
}

// GetPlayerOnlineStatus handles requests to check player online status.
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

	playerUUID, err := uuid.Parse(req.UUID)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid UUID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var banExpiresAt time.Time
	isPermanent := false

	if req.DurationSec == -1 {
		// Unban is handled by HandleUnbanPlayer, this endpoint is for banning.
		api.WriteError(w, http.StatusBadRequest, "Use /game/unban to unban a player")
		return
	} else if req.DurationSec == 0 {
		isPermanent = true
		// For a permanent ban, store a zero timestamp in Redis if desired, or handle as a separate flag
		// Here, we'll indicate permanent with a zero timestamp
		banExpiresAt = time.Time{} // Zero time indicates permanent ban
	} else {
		banExpiresAt = time.Now().Add(time.Duration(req.DurationSec) * time.Second)
	}

	// Set ban status in Redis (real-time check)
	err = gs.redisClient.SetBanStatus(ctx, playerUUID.String(), true, banExpiresAt.Unix())
	if err != nil {
		log.Printf("Error setting ban status for player %s in Redis: %v", playerUUID.String(), err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to ban player in Redis")
		return
	}

	// Persist ban status to MongoDB via Player Data Service
	mongoBanExpiresAt := &banExpiresAt
	if isPermanent {
		mongoBanExpiresAt = nil // Nil for permanent bans in MongoDB, or a specific constant if you prefer
	}

	err = gs.playerServiceClient.UpdateProfileBanStatus(ctx, playerUUID, true, mongoBanExpiresAt)
	if err != nil {
		log.Printf("Error persisting ban status for player %s to MongoDB: %v", playerUUID.String(), err)
		// Log and continue, Redis is the immediate source of truth for bans
	} else {
		log.Printf("Player %s ban status (banned: true, expires: %v) persisted to MongoDB.", playerUUID.String(), mongoBanExpiresAt)
	}

	responseMsg := fmt.Sprintf("Player %s banned", playerUUID.String())
	if !isPermanent {
		responseMsg = fmt.Sprintf("Player %s banned until %v", playerUUID.String(), banExpiresAt)
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{
		"message":      responseMsg,
		"uuid":         playerUUID.String(),
		"expires_at":   strconv.FormatInt(banExpiresAt.Unix(), 10),
		"is_permanent": strconv.FormatBool(isPermanent),
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

	playerUUID, err := uuid.Parse(req.UUID)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid UUID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Remove ban status from Redis
	err = gs.redisClient.SetBanStatus(ctx, playerUUID.String(), false, 0) // banned=false means DEL
	if err != nil {
		log.Printf("Error unbanning player %s in Redis: %v", playerUUID.String(), err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to unban player in Redis")
		return
	}

	// Persist unban status to MongoDB via Player Data Service
	err = gs.playerServiceClient.UpdateProfileBanStatus(ctx, playerUUID, false, nil) // Set banned to false, expires to nil
	if err != nil {
		log.Printf("Error persisting unban status for player %s to MongoDB: %v", playerUUID.String(), err)
		// Log and continue
	} else {
		log.Printf("Player %s unban status persisted to MongoDB.", playerUUID.String())
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": "Player unbanned", "uuid": playerUUID.String()})
}
