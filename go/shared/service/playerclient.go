// File: github.com/Ftotnem/Backend/go/shared/service/profileservice/client.go
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid" // Assuming you use google/uuid for UUIDs
)

// Player represents the player data structure returned by the player-service.
// This should mirror the structure of your Player model in player-service.
type Player struct {
	UUID          string     `json:"uuid" bson:"_id"`
	Username      string     `json:"username" bson:"username"`
	PlaytimeTicks float64    `json:"playtimeTicks" bson:"playtimeTicks"`
	Banned        bool       `json:"banned" bson:"banned"`
	BanExpiresAt  *time.Time `json:"banExpiresAt" bson:"banExpiresAt"`
	LastLogin     time.Time  `json:"lastLogin" bson:"lastLogin"`
	CreatedAt     time.Time  `json:"createdAt" bson:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt" bson:"updatedAt"`
}

// UpdateBanStatusRequest is the structure for the request body for updating ban status.
// This mirrors the UpdateBanStatusRequest in your player-service.
type UpdateBanStatusRequest struct {
	Banned       bool       `json:"banned"`
	BanExpiresAt *time.Time `json:"banExpiresAt"`
}

// GetPlayer fetches a player's profile by UUID.
// GET /players/{uuid}
func (c *Client) GetPlayer(ctx context.Context, playerUUID uuid.UUID) (*Player, error) {
	url := fmt.Sprintf("%s/players/%s", c.baseURL, playerUUID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for GetPlayer: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errorResponse struct {
			Message string `json:"message"`
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&errorResponse); decodeErr == nil && errorResponse.Message != "" {
			return nil, fmt.Errorf("received non-OK status code from %s: %d - %s", url, resp.StatusCode, errorResponse.Message)
		}
		return nil, fmt.Errorf("received non-OK status code from %s: %d", url, resp.StatusCode)
	}

	var player Player
	if err := json.NewDecoder(resp.Body).Decode(&player); err != nil {
		return nil, fmt.Errorf("failed to decode player response from %s: %w", url, err)
	}

	return &player, nil
}

// UpdatePlayerBanStatus sends a PUT request to update a player's ban status.
// PUT /players/{uuid}/ban
func (c *Client) UpdatePlayerBanStatus(ctx context.Context, playerUUID uuid.UUID, banned bool, banExpiresAt *time.Time) error {
	reqData := UpdateBanStatusRequest{
		Banned:       banned,
		BanExpiresAt: banExpiresAt,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON for UpdatePlayerBanStatus: %w", err)
	}

	url := fmt.Sprintf("%s/players/%s/ban", c.baseURL, playerUUID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request for UpdatePlayerBanStatus: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse struct {
			Message string `json:"message"`
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&errorResponse); decodeErr == nil && errorResponse.Message != "" {
			return fmt.Errorf("received non-OK status code from %s: %d - %s", url, resp.StatusCode, errorResponse.Message)
		}
		return fmt.Errorf("received non-OK status code from %s: %d", url, resp.StatusCode)
	}

	return nil
}

// UpdatePlayerLastLogin sends a PUT request to update a player's last login timestamp.
// PUT /players/{uuid}/lastlogin
func (c *Client) UpdatePlayerLastLogin(ctx context.Context, playerUUID uuid.UUID) error {
	url := fmt.Sprintf("%s/players/%s/lastlogin", c.baseURL, playerUUID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, nil) // No body needed for lastlogin update
	if err != nil {
		return fmt.Errorf("failed to create request for UpdatePlayerLastLogin: %w", err)
	}
	req.Header.Set("Content-Type", "application/json") // Still good practice to set this

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse struct {
			Message string `json:"message"`
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&errorResponse); decodeErr == nil && errorResponse.Message != "" {
			return fmt.Errorf("received non-OK status code from %s: %d - %s", url, resp.StatusCode, errorResponse.Message)
		}
		return fmt.Errorf("received non-OK status code from %s: %d", url, resp.StatusCode)
	}

	return nil
}
