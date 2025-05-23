// go/player-data-service/main.go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

func main() {
	// 1. Load Configuration
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 2. Connect to MongoDB
	mongoClient, err := ConnectMongoDB(cfg.MongoDBConnStr)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		// Disconnect from MongoDB when the main function exits
		if err = mongoClient.Disconnect(context.Background()); err != nil {
			log.Printf("Error disconnecting from MongoDB: %v", err)
		} else {
			log.Println("Disconnected from MongoDB.")
		}
	}()

	// 3. Initialize PlayerStore
	playerStore := NewPlayerStore(mongoClient, cfg.MongoDBDatabase, cfg.MongoDBPlayersCollection)
	playerService := NewPlayerService(playerStore) // Pass store to handlers

	// 4. Set up HTTP Router
	router := mux.NewRouter()

	// Player Management Routes
	router.HandleFunc("/players", playerService.CreatePlayerHandler).Methods("POST")
	router.HandleFunc("/players/{uuid}", playerService.GetPlayerHandler).Methods("GET")
	router.HandleFunc("/players/{uuid}/playtime", playerService.UpdatePlayerPlaytimeHandler).Methods("PUT")
	router.HandleFunc("/players/{uuid}/deltaplaytime", playerService.UpdatePlayerDeltaPlaytimeHandler).Methods("PUT")
	router.HandleFunc("/players/{uuid}/ban", playerService.UpdatePlayerBanStatusHandler).Methods("PUT")

	// 5. Start HTTP Server
	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: router,
		// Good practice to set timeouts to prevent slowloris attacks and dead connections
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Goroutine to start the HTTP server
	go func() {
		log.Printf("Player Data Service listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on %s: %v", cfg.ListenAddr, err)
		}
	}()

	// 6. Graceful Shutdown
	// Listen for OS signals to gracefully shut down (e.g., Ctrl+C, kill command)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM) // Capture Ctrl+C and kill

	// Block until we receive a signal
	sig := <-sigChan
	log.Printf("Received signal %s, shutting down...", sig)

	// Create a context with a timeout for the shutdown
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()

	// Attempt to gracefully shut down the HTTP server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Player Data Service gracefully stopped.")
}
