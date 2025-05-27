// services/gameservice/client.go
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.minekube.com/gate/pkg/util/uuid"
)

// PlayerStatusRequest represents the payload for online/offline updates.
type PlayerStatusRequest struct {
	UUID     uuid.UUID `json:"uuid"`
	Username string    `json:"username"`
	// Add any other relevant player data you might need, e.g., "server_name"
	// ServerName string `json:"server_name,omitempty"`
}

// Client is a client for the Game Service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Game Service client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second, // Configure a reasonable timeout
		},
	}
}

// SendPlayerOnline sends a POST request to the /game/online endpoint.
func (c *Client) SendPlayerOnline(playerUUID uuid.UUID, username string) error {
	reqData := PlayerStatusRequest{
		UUID:     playerUUID,
		Username: username,
	}
	return c.sendRequest("/game/online", reqData)
}

// SendPlayerOffline sends a POST request to the /game/offline endpoint.
func (c *Client) SendPlayerOffline(playerUUID uuid.UUID, username string) error {
	reqData := PlayerStatusRequest{
		UUID:     playerUUID,
		Username: username,
	}
	return c.sendRequest("/game/offline", reqData)
}

func (c *Client) sendRequest(endpoint string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON for %s: %w", endpoint, err)
	}

	url := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
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
		return fmt.Errorf("received non-OK status code from %s: %d", url, resp.StatusCode)
	}

	return nil
}
