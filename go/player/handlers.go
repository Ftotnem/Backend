package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Ftotnem/Backend/go/shared/api"
	"github.com/gorilla/mux"
)

// PlayerService holds dependencies for HTTP handlers (like the PlayerStore)
type PlayerService struct {
	store *PlayerStore
}

// NewPlayerService creates a new PlayerService instance
func NewPlayerService(store *PlayerStore) *PlayerService {
	return &PlayerService{store: store}
}

// GetPlayerHandler handles requests to retrieve a player by UUID.
// If the player is not found, it will create a new profile.
// GET /players/{uuid}
// Returns 200 OK if found, 201 Created if newly created.
func (ps *PlayerService) GetPlayerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required") // Use api.WriteError
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	player, created, err := ps.store.GetOrCreatePlayer(ctx, uuid)
	if err != nil {
		log.Printf("Error getting/creating player %s: %v", uuid, err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to get or create player profile: "+err.Error()) // Use api.WriteError
		return
	}

	statusCode := http.StatusOK
	if created {
		statusCode = http.StatusCreated // 201 Created if the player was new
		log.Printf("Player %s created successfully and profile returned.", player.UUID)
	} else {
		log.Printf("Player %s found and profile returned.", player.UUID)
	}
	api.WriteJSON(w, statusCode, player) // Use api.WriteJSON
}

// UpdatePlayerPlaytimeHandler handles requests to update a player's playtime.
// PUT /players/{uuid}/playtime
type UpdatePlaytimeRequest struct {
	TicksToSet float64 `json:"ticksToSet"`
}

func (ps *PlayerService) UpdatePlayerPlaytimeHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	var req UpdatePlaytimeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := ps.store.UpdatePlayerPlaytime(ctx, uuid, req.TicksToSet)
	if err != nil {
		if err.Error() == fmt.Sprintf("player %s not found for playtime update", uuid) {
			api.WriteError(w, http.StatusNotFound, "Player not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "Failed to update playtime: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Playtime updated for player %s", uuid)})
}

// UpdatePlayerDeltaPlaytimeHandler handles requests to update a player's delta playtime.
// PUT /players/{uuid}/deltaplaytime
type UpdateDeltaPlaytimeRequest struct {
	TicksToSet float64 `json:"ticksToSet"`
}

func (ps *PlayerService) UpdatePlayerDeltaPlaytimeHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	var req UpdateDeltaPlaytimeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := ps.store.UpdatePlayerDeltaPlaytime(ctx, uuid, req.TicksToSet)
	if err != nil {
		if err.Error() == fmt.Sprintf("player %s not found for delta playtime update", uuid) {
			api.WriteError(w, http.StatusNotFound, "Player not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "Failed to update delta playtime: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Delta playtime updated for player %s", uuid)})
}

// UpdatePlayerBanStatusHandler handles requests to update a player's ban status.
// PUT /players/{uuid}/ban
type UpdateBanStatusRequest struct {
	Banned       bool       `json:"banned"`
	BanExpiresAt *time.Time `json:"banExpiresAt"`
}

func (ps *PlayerService) UpdatePlayerBanStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	var req UpdateBanStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := ps.store.UpdatePlayerBanStatus(ctx, uuid, req.Banned, req.BanExpiresAt)
	if err != nil {
		if err.Error() == fmt.Sprintf("player %s not found for ban status update", uuid) {
			api.WriteError(w, http.StatusNotFound, "Player not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "Failed to update ban status: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Ban status updated for player %s", uuid)})
}

// UpdatePlayerLastLoginHandler handles requests to update only a player's last login timestamp.
// PUT /players/{uuid}/lastlogin
func (ps *PlayerService) UpdatePlayerLastLoginHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := ps.store.UpdatePlayerLastLogin(ctx, uuid)
	if err != nil {
		if err.Error() == fmt.Sprintf("player %s not found for last login update", uuid) {
			api.WriteError(w, http.StatusNotFound, "Player not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "Failed to update last login: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Last login updated for player %s", uuid)})
}
