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
	cluster "github.com/Ftotnem/Backend/go/shared/cluster"
	"github.com/Ftotnem/Backend/go/shared/service"
	"go.minekube.com/gate/pkg/util/uuid"
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

	// --- NEW: Initialize Service Registrar ---
	instanceID := uuid.New().String() // Generate a unique ID for this instance

	serviceConfig := cluster.ServiceConfig{
		ServiceID:         instanceID,
		ServiceType:       "game-service",
		IP:                "0.0.0.0",                   // Or extract host from ListenAddr if you need specific IP
		Port:              cfg.ServiceRegistrationPort, // Now an int!
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTTL:      15 * time.Second,
		CleanupInterval:   30 * time.Second,
		InitialMetadata: map[string]string{
			"version": "1.0.0",
		},
	}

	registrar, err := cluster.NewServiceRegistrar(redisClient.client, serviceConfig)
	if err != nil {
		log.Fatalf("Failed to create service registrar: %v", err)
	}

	// Register the service and start heartbeating
	if err := registrar.Register(); err != nil {
		log.Fatalf("Failed to register service with cluster membership: %v", err)
	}
	defer func() {
		// Use a background context for graceful deregistration during shutdown
		// This ensures the deregister call has enough time even if the main context is cancelled
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		registrar.Stop(shutdownCtx)
	}()
	// --- END NEW: Initialize Service Registrar ---

	gameService := NewGameService(redisClient, playerServiceClient, cfg)

	log.Printf("DEBUG: Configured TickInterval before updater start: %v", cfg.TickInterval)
	// --- Update: Initialize and Start GameUpdater with registrar ---
	gameUpdater := NewGameUpdater(redisClient, cfg, registrar) // Pass registrar
	go gameUpdater.Start()
	defer gameUpdater.Stop()

	// --- Update: Initialize and Start PlaytimeSyncer with registrar ---
	playtimeSyncer := NewPlaytimeSyncer(redisClient, playerServiceClient, cfg.PersistenceInterval, registrar) // Pass registrar
	go playtimeSyncer.Start()
	defer playtimeSyncer.Stop()

	baseServer := api.NewBaseServer(cfg.ListenAddr + cfg.Port)

	// Register your handlers on the BaseServer's router
	baseServer.Router.HandleFunc("/game/online", gameService.HandleOnline).Methods("POST")
	baseServer.Router.HandleFunc("/game/offline", gameService.HandleOffline).Methods("POST")
	baseServer.Router.HandleFunc("/game/total/{team}", gameService.GetTeamTotal).Methods("GET")
	baseServer.Router.HandleFunc("/game/player/{uuid}/online", gameService.GetPlayerOnlineStatus).Methods("GET")
	baseServer.Router.HandleFunc("/game/ban", gameService.HandleBanPlayer).Methods("POST")
	baseServer.Router.HandleFunc("/game/unban", gameService.HandleUnbanPlayer).Methods("POST")

	// Register playtime and deltatime endpoints
	baseServer.Router.HandleFunc("/playtime/{uuid}", gameService.handleGetPlaytime).Methods("GET")
	baseServer.Router.HandleFunc("/deltatime/{uuid}", gameService.handleGetDeltaPlaytime).Methods("GET")

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
