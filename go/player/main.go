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

	// Initialize TeamStore
	teamStore := NewTeamStore(mongoClient, cfg.MongoDBDatabase, cfg.MongoDBTeamCollection)

	// Ensure default teams exist in the database
	defaultTeams := []string{"AQUA_CREEPERS", "PURPLE_SWORDERS"}
	if err := teamStore.EnsureTeamsExist(context.Background(), defaultTeams); err != nil {
		log.Fatalf("Failed to ensure default teams exist: %v", err)
	}
	playerStore := NewPlayerStore(mongoClient, cfg.MongoDBDatabase, cfg.MongoDBPlayersCollection, mojangClient, teamStore)
	playerService := NewPlayerService(playerStore)

	teamService := NewTeamService(teamStore, playerStore) // Pass playerStore to TeamService for aggregation

	go startUsernameFiller(playerStore, mojangClient, 1*time.Minute)

	baseServer := api.NewBaseServer(cfg.ListenAddr)

	// Register your handlers on the BaseServer's router
	baseServer.Router.HandleFunc("/profiles", playerService.CreateProfileHandler).Methods("POST")
	baseServer.Router.HandleFunc("/profiles/{uuid}", playerService.GetProfileHandler).Methods("GET")
	baseServer.Router.HandleFunc("/profiles/{uuid}/playtime", playerService.UpdateProfilePlaytimeHandler).Methods("PUT")
	baseServer.Router.HandleFunc("/profiles/{uuid}/deltaplaytime", playerService.UpdateProfileDeltaPlaytimeHandler).Methods("PUT")
	baseServer.Router.HandleFunc("/profiles/{uuid}/ban", playerService.UpdateProfileBanStatusHandler).Methods("PUT")
	baseServer.Router.HandleFunc("/profiles/{uuid}/lastlogin", playerService.UpdateProfileLastLoginHandler).Methods("PUT")

	baseServer.Router.HandleFunc("/teams/sync-totals", teamService.SyncTeamTotalsHandler).Methods("POST")

	go func() {
		log.Printf("Player Data Service listening on %s", cfg.ListenAddr)
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
			log.Printf("Error finding profiles with empty usernames: %v", err)
			cancel()
			continue
		}

		var profilesToUpdate []struct {
			UUID string `bson:"_id"`
		}
		if err := cursor.All(ctx, &profilesToUpdate); err != nil {
			log.Printf("Error decoding profiles with empty usernames: %v", err)
			cursor.Close(ctx)
			cancel()
			continue
		}
		cursor.Close(ctx)

		if len(profilesToUpdate) == 0 {
			log.Println("No profiles with empty usernames found.")
			cancel()
			continue
		}

		log.Printf("Found %d profiles with empty usernames to process.", len(profilesToUpdate))

		for _, p := range profilesToUpdate {
			time.Sleep(100 * time.Millisecond) // Be nice to Mojang API

			username, mojangErr := mojangClient.GetUsernameByUUID(ctx, p.UUID)
			if mojangErr != nil {
				log.Printf("WARN: Username filler failed to fetch username for UUID %s: %v", p.UUID, mojangErr)
				continue
			}

			if updateErr := store.UpdateProfileUsername(ctx, p.UUID, username); updateErr != nil {
				log.Printf("WARN: Username filler failed to update username for profile %s in DB: %v", p.UUID, updateErr)
			} else {
				log.Printf("INFO: Username filler successfully updated username for profile %s to %s.", p.UUID, username)
			}
		}
		cancel()
		log.Println("Username filler job finished.")
	}
}
