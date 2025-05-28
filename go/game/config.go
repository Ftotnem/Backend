package main

import (
	"fmt"
	"net" // New import for net.SplitHostPort
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all the necessary configuration for the game-service.
type Config struct {
	ListenAddr                string        // Address for the HTTP server (e.g., ":8082" or "0.0.0.0:8082")
	ServiceRegistrationPort   int           // The numeric port to register with the cluster (extracted from ListenAddr)
	RedisAddrs                []string      // Redis server address (e.g., "127.0.0.1:7000")
	TickInterval              time.Duration // Duration for the game tick (e.g., 50ms)
	PersistenceInterval       time.Duration // Duration for periodic MongoDB persistence (e.g., 1m)
	RedisOnlineTTL            time.Duration // TTL for 'online:<uuid>' keys in Redis (e.g., 15s)
	GameServiceInstanceID     int           // Unique identifier for this game service instance (e.g., 0, 1, 2)
	TotalGameServiceInstances int           // Total number of active game service instances (e.g., 1, 3)
	PlayerServiceURL          string        // The url to the used player-service
}

// LoadConfig loads configuration from environment variables, applying defaults if not set.
// It returns a Config struct or an error if any required variable is missing or invalid.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		ListenAddr:       os.Getenv("GAME_SERVICE_LISTEN_ADDR"),
		PlayerServiceURL: os.Getenv("PLAYERS_SERVICE_URL"),
	}

	var err error

	// --- Load Duration fields ---
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

	cfg.PersistenceInterval, err = getDuration("GAME_SERVICE_PERSISTENCE_INTERVAL", 30*time.Second)
	if err != nil {
		return nil, err
	}

	cfg.RedisOnlineTTL, err = getDuration("REDIS_ONLINE_TTL", 15*time.Second)
	if err != nil {
		return nil, err
	}

	// --- Load Int fields ---
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

	// --- Load Redis Cluster Addresses ---
	redisAddrsStr := os.Getenv("REDIS_ADDRS") // New environment variable name, plural for clarity
	if redisAddrsStr == "" {
		// Default for a common local Redis Cluster setup (e.g., 6 nodes: 7000-7005)
		// It's sufficient to list a few seed nodes, the client will discover the rest.
		cfg.RedisAddrs = []string{
			"127.0.0.1:7000",
			"127.0.0.1:7001",
			"127.0.0.1:7002",
			"127.0.0.1:7003",
			"127.0.0.1:7004",
			"127.0.0.1:7005",
		}
	} else {
		// Split the comma-separated string into a slice of addresses
		cfg.RedisAddrs = strings.Split(redisAddrsStr, ",")
		// Optionally, trim spaces from each address
		for i, addr := range cfg.RedisAddrs {
			cfg.RedisAddrs[i] = strings.TrimSpace(addr)
		}
	}

	// --- Apply string defaults and extract ServiceRegistrationPort ---
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8082" // Default HTTP listen address, including port
	}

	// Parse the port from ListenAddr for service registration
	_, portStr, err := net.SplitHostPort(cfg.ListenAddr)
	if err != nil {
		// If SplitHostPort fails, check if ListenAddr is just a host (e.g., "0.0.0.0")
		// In that case, we need to apply a default port.
		if !strings.Contains(cfg.ListenAddr, ":") {
			defaultPort := "8082" // Use a common default port if only host is specified
			cfg.ListenAddr = fmt.Sprintf("%s:%s", cfg.ListenAddr, defaultPort)
			portStr = defaultPort
		} else {
			// If it contains a colon but is still invalid (e.g., "::"), return an error
			return nil, fmt.Errorf("invalid ListenAddr format for port extraction: %w", err)
		}
	}

	cfg.ServiceRegistrationPort, err = strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port number extracted from ListenAddr '%s': %w", portStr, err)
	}

	if cfg.PlayerServiceURL == "" {
		cfg.PlayerServiceURL = "http://localhost:8081" // Corrected URL scheme
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
