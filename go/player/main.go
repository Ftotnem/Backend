package main

import (
	"context"
	"log"
	"net/http" // Keep http for http.ErrServerClosed
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ftotnem/Backend/go/shared/api"
	"go.mongodb.org/mongo-driver/bson"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	mongoClient, err := ConnectMongoDB(cfg.MongoDBConnStr)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err = mongoClient.Disconnect(context.Background()); err != nil {
			log.Printf("Error disconnecting from MongoDB: %v", err)
		} else {
			log.Println("Disconnected from MongoDB.")
		}
	}()

	mojangClient := NewMojangClient()

	playerStore := NewPlayerStore(mongoClient, cfg.MongoDBDatabase, cfg.MongoDBPlayersCollection, mojangClient)
	playerService := NewPlayerService(playerStore)

	go startUsernameFiller(playerStore, mojangClient, 1*time.Minute)

	// Use your new BaseServer from the shared API module
	baseServer := api.NewBaseServer(cfg.ListenAddr)

	// Register your handlers on the BaseServer's router
	baseServer.Router.HandleFunc("/players/{uuid}", playerService.GetPlayerHandler).Methods("GET")
	baseServer.Router.HandleFunc("/players/{uuid}/playtime", playerService.UpdatePlayerPlaytimeHandler).Methods("PUT")
	baseServer.Router.HandleFunc("/players/{uuid}/deltaplaytime", playerService.UpdatePlayerDeltaPlaytimeHandler).Methods("PUT")
	baseServer.Router.HandleFunc("/players/{uuid}/ban", playerService.UpdatePlayerBanStatusHandler).Methods("PUT")
	baseServer.Router.HandleFunc("/players/{uuid}/lastlogin", playerService.UpdatePlayerLastLoginHandler).Methods("PUT")

	go func() {
		log.Printf("Player Data Service listening on %s", cfg.ListenAddr)
		if err := baseServer.Start(); err != nil && err != http.ErrServerClosed { // Use baseServer.Start()
			log.Fatalf("Could not listen on %s: %v", cfg.ListenAddr, err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("Received signal %s, shutting down...", sig)

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()

	if err := baseServer.Shutdown(shutdownCtx); err != nil { // Use baseServer.Shutdown()
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Player Data Service gracefully stopped.")
}

func startUsernameFiller(store *PlayerStore, mojangClient *MojangClient, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Starting background username filler, checking every %v", interval)

	for range ticker.C {
		log.Println("Running username filler job...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		filter := bson.M{"username": ""}
		cursor, err := store.collection.Find(ctx, filter)
		if err != nil {
			log.Printf("Error finding players with empty usernames: %v", err)
			cancel()
			continue
		}

		var playersToUpdate []struct {
			UUID string `bson:"_id"`
		}
		if err := cursor.All(ctx, &playersToUpdate); err != nil {
			log.Printf("Error decoding players with empty usernames: %v", err)
			cursor.Close(ctx)
			cancel()
			continue
		}
		cursor.Close(ctx)

		if len(playersToUpdate) == 0 {
			log.Println("No players with empty usernames found.")
			cancel()
			continue
		}

		log.Printf("Found %d players with empty usernames to process.", len(playersToUpdate))

		for _, p := range playersToUpdate {
			time.Sleep(100 * time.Millisecond)

			username, mojangErr := mojangClient.GetUsernameByUUID(ctx, p.UUID)
			if mojangErr != nil {
				log.Printf("WARN: Username filler failed to fetch username for UUID %s: %v", p.UUID, mojangErr)
				continue
			}

			if updateErr := store.UpdatePlayerUsername(ctx, p.UUID, username); updateErr != nil {
				log.Printf("WARN: Username filler failed to update username for player %s in DB: %v", p.UUID, updateErr)
			} else {
				log.Printf("INFO: Username filler successfully updated username for player %s to %s.", p.UUID, username)
			}
		}
		cancel()
		log.Println("Username filler job finished.")
	}
}
