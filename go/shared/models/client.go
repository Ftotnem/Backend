package models

import (
	"net/http"
	"time"
	// Assuming you use google/uuid for UUIDs
)

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
