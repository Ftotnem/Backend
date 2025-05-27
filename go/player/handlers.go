package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/mongo" // Import mongo to check for ErrNoDocuments

	"github.com/Ftotnem/Backend/go/shared/api" // Import models for Player struct
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

// CreateProfileRequest defines the request body for creating a player profile.
type CreateProfileRequest struct {
	UUID string `json:"uuid"` // UUID is typically provided by the client (e.g., Minecraft client)
}

// CreateProfileHandler handles requests to create a new player profile.
// POST /profiles
func (ps *PlayerService) CreateProfileHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateProfileRequest
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

	createdProfile, err := ps.store.CreateProfile(ctx, req.UUID)
	if err != nil {
		if err.Error() == fmt.Sprintf("player profile %s already exists", req.UUID) {
			api.WriteError(w, http.StatusConflict, fmt.Sprintf("Profile with UUID %s already exists", req.UUID))
			return
		}
		log.Printf("Error creating player profile %s: %v", req.UUID, err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to create player profile: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusCreated, createdProfile) // 201 Created
	log.Printf("Player profile %s created successfully and profile returned.", createdProfile.UUID)
}

// GetProfileHandler handles requests to retrieve a player profile by UUID.
// GET /profiles/{uuid}
func (ps *PlayerService) GetProfileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	profile, err := ps.store.GetProfileByUUID(ctx, uuid)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			api.WriteError(w, http.StatusNotFound, fmt.Sprintf("Player profile with UUID %s not found", uuid))
			return
		}
		log.Printf("Error getting player profile %s: %v", uuid, err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to retrieve player profile: "+err.Error())
		return
	}

	// It's generally a good practice to update the last login on a successful retrieval
	// or specific login event, rather than every GET. For now, keeping it here.
	go func() {
		updateCtx, updateCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer updateCancel()
		if err := ps.store.UpdateProfileLastLogin(updateCtx, uuid); err != nil {
			log.Printf("WARN: Failed to update last login for player profile %s: %v", uuid, err)
		}
	}()

	api.WriteJSON(w, http.StatusOK, profile)
	log.Printf("Player profile %s retrieved successfully.", profile.UUID)
}

// UpdateProfilePlaytimeHandler handles requests to update a player's playtime.
// PUT /profiles/{uuid}/playtime
type UpdatePlaytimeRequest struct {
	TicksToSet float64 `json:"ticksToSet"`
}

func (ps *PlayerService) UpdateProfilePlaytimeHandler(w http.ResponseWriter, r *http.Request) {
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

	err := ps.store.UpdateProfilePlaytime(ctx, uuid, req.TicksToSet)
	if err != nil {
		if err.Error() == fmt.Sprintf("player profile %s not found for playtime update", uuid) {
			api.WriteError(w, http.StatusNotFound, "Player profile not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "Failed to update playtime: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Playtime updated for player profile %s", uuid)})
}

// UpdateProfileDeltaPlaytimeHandler handles requests to update a player's delta playtime.
// PUT /profiles/{uuid}/deltaplaytime
type UpdateDeltaPlaytimeRequest struct {
	TicksToSet float64 `json:"ticksToSet"`
}

func (ps *PlayerService) UpdateProfileDeltaPlaytimeHandler(w http.ResponseWriter, r *http.Request) {
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

	err := ps.store.UpdateProfileDeltaPlaytime(ctx, uuid, req.TicksToSet)
	if err != nil {
		if err.Error() == fmt.Sprintf("player profile %s not found for delta playtime update", uuid) {
			api.WriteError(w, http.StatusNotFound, "Player profile not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "Failed to update delta playtime: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Delta playtime updated for player profile %s", uuid)})
}

// UpdateProfileBanStatusHandler handles requests to update a player's ban status.
// PUT /profiles/{uuid}/ban
type UpdateBanStatusRequest struct {
	Banned       bool       `json:"banned"`
	BanExpiresAt *time.Time `json:"banExpiresAt"`
}

func (ps *PlayerService) UpdateProfileBanStatusHandler(w http.ResponseWriter, r *http.Request) {
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

	err := ps.store.UpdateProfileBanStatus(ctx, uuid, req.Banned, req.BanExpiresAt)
	if err != nil {
		if err.Error() == fmt.Sprintf("player profile %s not found for ban status update", uuid) {
			api.WriteError(w, http.StatusNotFound, "Player profile not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "Failed to update ban status: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Ban status updated for player profile %s", uuid)})
}

// UpdateProfileLastLoginHandler handles requests to update only a player's last login timestamp.
// PUT /profiles/{uuid}/lastlogin
func (ps *PlayerService) UpdateProfileLastLoginHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "Player UUID is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := ps.store.UpdateProfileLastLogin(ctx, uuid)
	if err != nil {
		if err.Error() == fmt.Sprintf("player profile %s not found for last login update", uuid) {
			api.WriteError(w, http.StatusNotFound, "Player profile not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "Failed to update last login: "+err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Last login updated for player profile %s", uuid)})
}
