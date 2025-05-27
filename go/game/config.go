package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all the necessary configuration for the game-service.
type Config struct {
	ListenAddr                string        // Address for the HTTP server (e.g., ":8082")
	RedisAddr                 string        // Redis server address (e.g., "127.0.0.1:7000")
	TickInterval              time.Duration // Duration for the game tick (e.g., 50ms)
	PersistenceInterval       time.Duration // Duration for periodic MongoDB persistence (e.g., 1m)
	RedisOnlineTTL            time.Duration // TTL for 'online:<uuid>' keys in Redis (e.g., 15s)
	GameServiceInstanceID     int           // Unique identifier for this game service instance (e.g., 0, 1, 2)
	TotalGameServiceInstances int           // Total number of active game service instances (e.g., 1, 3)
}

// LoadConfig loads configuration from environment variables, applying defaults if not set.
// It returns a Config struct or an error if any required variable is missing or invalid.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		ListenAddr: os.Getenv("GAME_SERVICE_LISTEN_ADDR"),
		RedisAddr:  os.Getenv("REDIS_ADDR"),
	}

	var err error

	// --- Load Duration fields ---
	// Using a helper for durations, similar to your original design, to handle parsing errors.
	// Defaults will be applied if the env var is not set OR if parsing fails.
	getDuration := func(envKey string, defaultVal time.Duration) (time.Duration, error) {
		valStr := os.Getenv(envKey)
		if valStr == "" {
			return defaultVal, nil
		}
		d, err := time.ParseDuration(valStr)
		if err != nil {
			return 0, fmt.Errorf("invalid duration format for %s: %w", envKey, err)
		}
		return d, nil
	}

	cfg.TickInterval, err = getDuration("GAME_SERVICE_TICK_INTERVAL", 50*time.Millisecond)
	if err != nil {
		return nil, err
	}

	cfg.PersistenceInterval, err = getDuration("GAME_SERVICE_PERSISTENCE_INTERVAL", 1*time.Minute)
	if err != nil {
		return nil, err
	}

	cfg.RedisOnlineTTL, err = getDuration("REDIS_ONLINE_TTL", 15*time.Second)
	if err != nil {
		return nil, err
	}

	// --- Load Int fields ---
	// Using a helper for integers, similar to your original design, to handle parsing errors.
	// Defaults will be applied if the env var is not set OR if parsing fails.
	getInt := func(envKey string, defaultVal int) (int, error) {
		valStr := os.Getenv(envKey)
		if valStr == "" {
			return defaultVal, nil
		}
		i, err := strconv.Atoi(valStr)
		if err != nil {
			return 0, fmt.Errorf("invalid integer format for %s: %w", envKey, err)
		}
		return i, nil
	}

	cfg.GameServiceInstanceID, err = getInt("GAME_SERVICE_INSTANCE_ID", 0) // Default to 0 for single instance
	if err != nil {
		return nil, err
	}

	cfg.TotalGameServiceInstances, err = getInt("TOTAL_GAME_SERVICE_INSTANCES", 1) // Default to 1 for single instance
	if err != nil {
		return nil, err
	}

	// --- Apply string defaults directly ---
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8082" // Default HTTP listen address
	}
	if cfg.RedisAddr == "" {
		// For a local Redis cluster, you'd typically connect to one of the masters (e.g., 7000)
		// and the client library (like go-redis) would handle cluster redirection.
		cfg.RedisAddr = "127.0.0.1:7000" // Default Redis address for local cluster
	}

	// --- Final validation for instance IDs (important even with defaults) ---
	if cfg.TotalGameServiceInstances <= 0 {
		return nil, fmt.Errorf("TOTAL_GAME_SERVICE_INSTANCES must be a positive integer (got %d)", cfg.TotalGameServiceInstances)
	}
	if cfg.GameServiceInstanceID < 0 {
		return nil, fmt.Errorf("GAME_SERVICE_INSTANCE_ID must be a non-negative integer (got %d)", cfg.GameServiceInstanceID)
	}
	if cfg.GameServiceInstanceID >= cfg.TotalGameServiceInstances {
		return nil, fmt.Errorf("GAME_SERVICE_INSTANCE_ID (%d) must be less than TOTAL_GAME_SERVICE_INSTANCES (%d)", cfg.GameServiceInstanceID, cfg.TotalGameServiceInstances)
	}

	return cfg, nil
}
