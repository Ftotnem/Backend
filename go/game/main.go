package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ftotnem/Backend/go/shared/api" // Import your shared API module
)

func main() {
	// Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	log.Printf("Configuration loaded: ListenAddr=%s, RedisAddr=%s, TickInterval=%v, PersistenceInterval=%v, RedisOnlineTTL=%v, InstanceID=%d, TotalInstances=%d",
		cfg.ListenAddr, cfg.RedisAddr, cfg.TickInterval, cfg.PersistenceInterval, cfg.RedisOnlineTTL, cfg.GameServiceInstanceID, cfg.TotalGameServiceInstances)

	// Initialize Redis Client
	redisClient, err := NewRedisClient(cfg.RedisAddr, cfg.RedisOnlineTTL)
	if err != nil {
		log.Fatalf("Failed to initialize Redis client: %v", err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("Error closing Redis client: %v", err)
		} else {
			log.Println("Redis client closed.")
		}
	}()
	log.Println("Successfully connected to Redis.")

	// Initialize GameService (HTTP handlers)
	gameService := NewGameService(redisClient, cfg)

	// Use your new BaseServer from the shared API module
	baseServer := api.NewBaseServer(cfg.ListenAddr)

	// Register game-service specific handlers on the BaseServer's router
	baseServer.Router.HandleFunc("/game/online", gameService.HandleOnline).Methods("POST")
	baseServer.Router.HandleFunc("/game/offline", gameService.HandleOffline).Methods("POST")
	baseServer.Router.HandleFunc("/game/teams/total", gameService.GetTeamTotals).Methods("GET")
	baseServer.Router.HandleFunc("/game/players/{uuid}/online", gameService.GetPlayerOnlineStatus).Methods("GET") // Optional: for checking individual player status

	// TODO: Initialize and start the tick-based updater goroutine here
	// go StartUpdater(context.Background(), redisClient, cfg)
	// log.Println("Tick-based updater started.")

	// TODO: Initialize and start the periodic persister goroutine here
	// go StartPersister(context.Background(), redisClient, mongoClient, cfg)
	// log.Println("Periodic persister started.")

	// Start HTTP Server in a goroutine
	go func() {
		log.Printf("Game Service listening on %s", cfg.ListenAddr)
		if err := baseServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on %s: %v", cfg.ListenAddr, err)
		}
	}()

	// Graceful Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("Received signal %s, shutting down...", sig)

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()

	if err := baseServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Game Service gracefully stopped.")
}
