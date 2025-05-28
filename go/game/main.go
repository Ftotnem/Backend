package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ftotnem/Backend/go/shared/api"
	"github.com/Ftotnem/Backend/go/shared/service"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	redisClient, err := NewRedisClient(cfg.RedisAddrs, cfg.RedisOnlineTTL)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("Error closing Redis client: %v", err)
		} else {
			log.Println("Redis client closed.")
		}
	}()

	playerServiceClient := service.NewPlayerClient(cfg.PlayerServiceURL)

	gameService := NewGameService(redisClient, playerServiceClient, cfg)

	log.Printf("DEBUG: Configured TickInterval before updater start: %v", cfg.TickInterval)
	// --- NEW: Initialize and Start GameUpdater ---
	gameUpdater := NewGameUpdater(redisClient, cfg)
	go gameUpdater.Start()   // Run the updater in a separate goroutine
	defer gameUpdater.Stop() // Ensure it stops on shutdown

	playtimeSyncer := NewPlaytimeSyncer(redisClient, playerServiceClient, cfg.PersistenceInterval)
	go playtimeSyncer.Start()
	defer playtimeSyncer.Stop()

	baseServer := api.NewBaseServer(cfg.ListenAddr)

	// Register your handlers on the BaseServer's router
	baseServer.Router.HandleFunc("/game/online", gameService.HandleOnline).Methods("POST")
	baseServer.Router.HandleFunc("/game/offline", gameService.HandleOffline).Methods("POST")
	baseServer.Router.HandleFunc("/game/total/{team}", gameService.GetTeamTotal).Methods("GET")
	baseServer.Router.HandleFunc("/game/player/{uuid}/online", gameService.GetPlayerOnlineStatus).Methods("GET")
	baseServer.Router.HandleFunc("/game/ban", gameService.HandleBanPlayer).Methods("POST")
	baseServer.Router.HandleFunc("/game/unban", gameService.HandleUnbanPlayer).Methods("POST")

	// --- NEW: Register playtime and deltatime endpoints ---
	baseServer.Router.HandleFunc("/playtime/{uuid}", gameService.handleGetPlaytime).Methods("GET")
	baseServer.Router.HandleFunc("/deltatime/{uuid}", gameService.handleGetDeltaPlaytime).Methods("GET")
	// --- END NEW ---

	go func() {
		log.Printf("Game Service listening on %s", cfg.ListenAddr)
		if err := baseServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on %s: %v", cfg.ListenAddr, err)
		}
	}()

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
