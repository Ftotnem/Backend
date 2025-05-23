// go/player-data-service/config.go
package main

import (
	"os"
)

// Config holds the configuration for the Player Data Service
type Config struct {
	ListenAddr               string // Address for the HTTP server to listen on (e.g., ":8080")
	MongoDBConnStr           string // MongoDB connection string
	MongoDBDatabase          string // MongoDB database name (e.g., "minecraft_events")
	MongoDBPlayersCollection string // MongoDB collection for players (e.g., "players")
}

// LoadConfig loads configuration from environment variables.
// In a real application, you might use a dedicated config library (e.g., github.com/spf13/viper)
func LoadConfig() (*Config, error) {
	cfg := &Config{
		ListenAddr:               os.Getenv("LISTEN_ADDR"),
		MongoDBConnStr:           os.Getenv("MONGODB_CONN_STR"),
		MongoDBDatabase:          os.Getenv("MONGODB_DATABASE"),
		MongoDBPlayersCollection: os.Getenv("MONGODB_PLAYERS_COLLECTION"),
	}

	// Set defaults if environment variables are not provided
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8081"
	}
	if cfg.MongoDBConnStr == "" {
		cfg.MongoDBConnStr = "mongodb://localhost:27017"
	}
	if cfg.MongoDBDatabase == "" {
		cfg.MongoDBDatabase = "minestom" // Default database name
	}
	if cfg.MongoDBPlayersCollection == "" {
		cfg.MongoDBPlayersCollection = "players" // Default collection name
	}

	return cfg, nil
}
