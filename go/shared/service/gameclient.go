// File: github.com/Ftotnem/Backend/go/shared/service/client.go
// This new package client should be separate from your main 'service' package
// to avoid circular dependencies if 'service' also imports this client.

package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid" // Assuming you use google/uuid for UUIDs
)

// OnlineStatusRequest represents the payload for online/offline updates.
type OnlineStatusRequest struct {
	UUID string `json:"uuid"`
}

// BanRequest is the structure for the request body for banning/unbanning.
type BanRequest struct {
	UUID        string `json:"uuid"`
	DurationSec int64  `json:"duration_seconds"` // Duration in seconds. 0 for permanent, -1 to unban.
	Reason      string `json:"reason,omitempty"`
}

// SendPlayerOnline sends a POST request to the /game/online endpoint.
func (c *Client) SendPlayerOnline(playerUUID uuid.UUID) error {
	reqData := OnlineStatusRequest{
		UUID: playerUUID.String(),
	}
	return c.sendRequest(http.MethodPost, "/game/online", reqData)
}

// SendPlayerOffline sends a POST request to the /game/offline endpoint.
func (c *Client) SendPlayerOffline(playerUUID uuid.UUID) error {
	reqData := OnlineStatusRequest{
		UUID: playerUUID.String(),
	}
	return c.sendRequest(http.MethodPost, "/game/offline", reqData)
}

// BanPlayer sends a POST request to the /game/ban endpoint to ban a player.
func (c *Client) BanPlayer(playerUUID uuid.UUID, duration time.Duration, reason string) error {
	reqData := BanRequest{
		UUID:        playerUUID.String(),
		DurationSec: int64(duration.Seconds()),
		Reason:      reason,
	}
	return c.sendRequest(http.MethodPost, "/game/ban", reqData)
}

// UnbanPlayer sends a POST request to the /game/unban endpoint to unban a player.
func (c *Client) UnbanPlayer(playerUUID uuid.UUID) error {
	reqData := OnlineStatusRequest{ // Re-use OnlineStatusRequest as it only needs UUID
		UUID: playerUUID.String(),
	}
	return c.sendRequest(http.MethodPost, "/game/unban", reqData)
}

// Helper to send HTTP requests
func (c *Client) sendRequest(method, endpoint string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON for %s: %w", endpoint, err)
	}

	url := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Attempt to read error message from body if available
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
