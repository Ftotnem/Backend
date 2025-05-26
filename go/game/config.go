package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all the necessary configuration for the game-service.
type Config struct {
	ListenAddr                string        // Address for the HTTP server (e.g., ":8081")
	RedisAddr                 string        // Redis server address (e.g., "localhost:6379")
	TickInterval              time.Duration // Duration for the game tick (e.g., 50ms)
	PersistenceInterval       time.Duration // Duration for periodic MongoDB persistence (e.g., 1m)
	RedisOnlineTTL            time.Duration // TTL for 'online:<uuid>' keys in Redis (e.g., 15s)
	GameServiceInstanceID     int           // Unique identifier for this game service instance (e.g., 1, 2)
	TotalGameServiceInstances int           // Total number of active game service instances
}

// LoadConfig loads configuration from environment variables.
// It returns a Config struct or an error if any required variable is missing or invalid.
func LoadConfig() (*Config, error) {
	// Helper function to get environment variable or return an error if missing
	getEnv := func(key string) (string, error) {
		val := os.Getenv(key)
		if val == "" {
			return "", fmt.Errorf("environment variable %s not set", key)
		}
		return val, nil
	}

	// Helper function to get environment variable as duration
	getEnvDuration := func(key string) (time.Duration, error) {
		valStr, err := getEnv(key)
		if err != nil {
			return 0, err
		}
		duration, err := time.ParseDuration(valStr)
		if err != nil {
			return 0, fmt.Errorf("invalid duration format for %s: %w", key, err)
		}
		return duration, nil
	}

	// Helper function to get environment variable as int
	getEnvInt := func(key string) (int, error) {
		valStr, err := getEnv(key)
		if err != nil {
			return 0, err
		}
		valInt, err := strconv.Atoi(valStr)
		if err != nil {
			return 0, fmt.Errorf("invalid integer format for %s: %w", key, err)
		}
		return valInt, nil
	}

	listenAddr, err := getEnv("GAME_SERVICE_LISTEN_ADDR")
	if err != nil {
		return nil, err
	}

	redisAddr, err := getEnv("REDIS_ADDR")
	if err != nil {
		return nil, err
	}

	tickInterval, err := getEnvDuration("GAME_SERVICE_TICK_INTERVAL")
	if err != nil {
		return nil, err
	}

	persistenceInterval, err := getEnvDuration("GAME_SERVICE_PERSISTENCE_INTERVAL")
	if err != nil {
		return nil, err
	}

	redisOnlineTTL, err := getEnvDuration("REDIS_ONLINE_TTL")
	if err != nil {
		return nil, err
	}

	gameServiceInstanceID, err := getEnvInt("GAME_SERVICE_INSTANCE_ID")
	if err != nil {
		return nil, err
	}
	if gameServiceInstanceID < 0 {
		return nil, fmt.Errorf("GAME_SERVICE_INSTANCE_ID must be a non-negative integer")
	}

	totalGameServiceInstances, err := getEnvInt("TOTAL_GAME_SERVICE_INSTANCES")
	if err != nil {
		return nil, err
	}
	if totalGameServiceInstances <= 0 {
		return nil, fmt.Errorf("TOTAL_GAME_SERVICE_INSTANCES must be a positive integer")
	}
	if gameServiceInstanceID >= totalGameServiceInstances {
		return nil, fmt.Errorf("GAME_SERVICE_INSTANCE_ID (%d) must be less than TOTAL_GAME_SERVICE_INSTANCES (%d)", gameServiceInstanceID, totalGameServiceInstances)
	}

	return &Config{
		ListenAddr:                listenAddr,
		RedisAddr:                 redisAddr,
		TickInterval:              tickInterval,
		PersistenceInterval:       persistenceInterval,
		RedisOnlineTTL:            redisOnlineTTL,
		GameServiceInstanceID:     gameServiceInstanceID,
		TotalGameServiceInstances: totalGameServiceInstances,
	}, nil
}
