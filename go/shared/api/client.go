// go/shared/api/client.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io" // Import for io.ReadAll
	"net/http"
	"time"
)

// HTTPError is a custom error type for HTTP responses with non-OK status codes.
type HTTPError struct {
	StatusCode int
	Message    string
	URL        string
	Method     string
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("HTTP error %d %s from %s %s: %s", e.StatusCode, http.StatusText(e.StatusCode), e.Method, e.URL, e.Message)
	}
	return fmt.Sprintf("HTTP error %d %s from %s %s", e.StatusCode, http.StatusText(e.StatusCode), e.Method, e.URL)
}

// Common errors for client usage
var (
	ErrNotFound = fmt.Errorf("resource not found")
	ErrConflict = fmt.Errorf("resource conflict")
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    baseURL,
	}
}

// doRequest is a helper for common request logic
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	url := fmt.Sprintf("%s%s", c.baseURL, path)

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body for %s %s: %w", method, url, err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create %s request for %s: %w", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send %s request to %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errorResponse struct {
			Message string `json:"message"`
		}
		// Try to read error message from body
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr == nil && len(bodyBytes) > 0 {
			if jsonErr := json.Unmarshal(bodyBytes, &errorResponse); jsonErr == nil && errorResponse.Message != "" {
				return &HTTPError{StatusCode: resp.StatusCode, Message: errorResponse.Message, URL: url, Method: method}
			}
			// If JSON decoding fails or message is empty, just include the raw body if it's small
			if len(bodyBytes) < 200 { // Limit size to avoid logging huge bodies
				return &HTTPError{StatusCode: resp.StatusCode, Message: string(bodyBytes), URL: url, Method: method}
			}
		}
		return &HTTPError{StatusCode: resp.StatusCode, URL: url, Method: method}
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode %s response from %s: %w", method, url, err)
		}
	}
	return nil
}

func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	return c.doRequest(ctx, "GET", path, nil, result)
}

func (c *Client) Post(ctx context.Context, path string, body interface{}, result interface{}) error {
	return c.doRequest(ctx, "POST", path, body, result)
}

func (c *Client) Put(ctx context.Context, path string, body interface{}, result interface{}) error {
	return c.doRequest(ctx, "PUT", path, body, result)
}
