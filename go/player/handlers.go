// go/player-data-service/handlers.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Ftotnem/Backend/go/shared/models" // Import shared models
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

// CreatePlayerHandler handles requests to create a new player.
// POST /players
func (ps *PlayerService) CreatePlayerHandler(w http.ResponseWriter, r *http.Request) {
	var player models.Player
	if err := json.NewDecoder(r.Body).Decode(&player); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if player.UUID == "" || player.Username == "" || player.Team == "" {
		http.Error(w, "UUID, Username, and Team are required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := ps.store.CreatePlayer(ctx, &player); err != nil {
		// Differentiate between generic error and duplicate key error if you prefer
		// For simplicity, we just return Internal Server Error here.
		http.Error(w, "Failed to create player: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(player) // Return the created player data
}

// GetPlayerHandler handles requests to retrieve a player by UUID.
// GET /players/{uuid}
func (ps *PlayerService) GetPlayerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		http.Error(w, "Player UUID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	player, err := ps.store.GetPlayerByUUID(ctx, uuid)
	if err != nil {
		if err.Error() == fmt.Sprintf("player %s not found", uuid) {
			http.Error(w, "Player not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to retrieve player: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(player)
}

// UpdatePlayerPlaytimeHandler handles requests to update a player's playtime.
// PUT /players/{uuid}/playtime
// Request body: {"ticksToAdd": 100.5}
type UpdatePlaytimeRequest struct {
	TicksToSet float64 `json:"ticksToSet"`
}

func (ps *PlayerService) UpdatePlayerPlaytimeHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		http.Error(w, "Player UUID is required", http.StatusBadRequest)
		return
	}

	var req UpdatePlaytimeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := ps.store.UpdatePlayerPlaytime(ctx, uuid, req.TicksToSet)
	if err != nil {
		if err.Error() == fmt.Sprintf("player %s not found for playtime update", uuid) {
			http.Error(w, "Player not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to update playtime: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Playtime updated for player %s\n", uuid)
}

// UpdatePlayerDeltaPlaytimeHandler handles requests to update a player's delta playtime (ticks per tick).
// PUT /players/{uuid}/playtime
// Request body: {"deltaTicksToSet": 100.5}
type UpdateDeltaPlaytimeRequest struct {
	TicksToSet float64 `json:"ticksToSet"`
}

func (ps *PlayerService) UpdatePlayerDeltaPlaytimeHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		http.Error(w, "Player UUID is required", http.StatusBadRequest)
		return
	}

	var req UpdatePlaytimeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := ps.store.UpdatePlayerDeltaPlaytime(ctx, uuid, req.TicksToSet)
	if err != nil {
		if err.Error() == fmt.Sprintf("player %s not found for delta playtime update", uuid) {
			http.Error(w, "Player not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to update delta playtime: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Delta playtime updated for player %s\n", uuid)
}

// UpdatePlayerBanStatusHandler handles requests to update a player's ban status.
// PUT /players/{uuid}/ban
// Request body: {"banned": true, "banExpiresAt": "2025-12-31T23:59:59Z"}
type UpdateBanStatusRequest struct {
	Banned       bool       `json:"banned"`
	BanExpiresAt *time.Time `json:"banExpiresAt"` // Use pointer to allow null
}

func (ps *PlayerService) UpdatePlayerBanStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		http.Error(w, "Player UUID is required", http.StatusBadRequest)
		return
	}

	var req UpdateBanStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := ps.store.UpdatePlayerBanStatus(ctx, uuid, req.Banned, req.BanExpiresAt)
	if err != nil {
		if err.Error() == fmt.Sprintf("player %s not found for ban status update", uuid) {
			http.Error(w, "Player not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to update ban status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Ban status updated for player %s\n", uuid)
}
