// go/player-data-service/mojang.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// mojangProfile represents the structure of the JSON response from Mojang's Session Server.
type mojangProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// We only care about ID and Name for this purpose
}

// MojangClient is a client for interacting with Mojang's Session Server API.
type MojangClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewMojangClient creates a new MojangClient instance.
func NewMojangClient() *MojangClient {
	return &MojangClient{
		httpClient: &http.Client{Timeout: 5 * time.Second}, // Short timeout for external API
		baseURL:    "https://sessionserver.mojang.com/session/minecraft/profile",
	}
}

// GetUsernameByUUID fetches a Minecraft username from Mojang's API using the player's UUID.
// Returns the username and nil error on success.
// Returns an empty string and an error if lookup fails (e.g., network, Mojang API down, UUID not found).
func (mc *MojangClient) GetUsernameByUUID(ctx context.Context, uuid string) (string, error) {
	url := fmt.Sprintf("%s/%s", mc.baseURL, uuid)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create Mojang API request: %w", err)
	}

	resp, err := mc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make Mojang API request for UUID %s: %w", uuid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// If 404, it might mean the UUID doesn't exist on Mojang (e.g., cracked client UUID)
		// or if Mojang is having issues. Treat as "username not found" for now.
		if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("mojang profile not found for UUID %s (Status: %d)", uuid, resp.StatusCode)
		}
		return "", fmt.Errorf("unexpected status from Mojang API for UUID %s: %d", uuid, resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Mojang API response body for UUID %s: %w", uuid, err)
	}

	var profile mojangProfile
	if err := json.Unmarshal(bodyBytes, &profile); err != nil {
		return "", fmt.Errorf("failed to unmarshal Mojang API response for UUID %s: %w", uuid, err)
	}

	if profile.Name == "" {
		return "", fmt.Errorf("mojang API returned empty username for UUID %s", uuid)
	}

	return profile.Name, nil
}
