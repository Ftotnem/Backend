// go/player-data-service/datastore.go
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/Ftotnem/Backend/go/shared/models"
)

// PlayerStore represents the MongoDB data store for player profiles.
type PlayerStore struct {
	collection   *mongo.Collection
	mojangClient *MojangClient // NEW: Reference to Mojang API client
}

// NewPlayerStore creates a new PlayerStore instance.
func NewPlayerStore(client *mongo.Client, databaseName, collectionName string, mojangClient *MojangClient) *PlayerStore { // NEW: mojangClient param
	collection := client.Database(databaseName).Collection(collectionName)
	return &PlayerStore{
		collection:   collection,
		mojangClient: mojangClient, // Assign the client
	}
}

// ConnectMongoDB establishes a connection to the MongoDB server.
func ConnectMongoDB(connStr string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	log.Println("Successfully connected to MongoDB!")
	return client, nil
}

// CreatePlayer inserts a new player document into the collection.
// It will error if a player with the same UUID already exists.
func (ps *PlayerStore) CreatePlayer(ctx context.Context, player *models.Player) error {
	if player.CreatedAt == nil || player.CreatedAt.IsZero() {
		now := time.Now()
		player.CreatedAt = &now
	}
	_, err := ps.collection.InsertOne(ctx, player)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("player %s already exists", player.UUID)
		}
		return fmt.Errorf("failed to create player %s: %w", player.UUID, err)
	}

	log.Printf("Player %s created successfully.", player.UUID)
	return nil
}

// GetPlayerByUUID retrieves a player by their UUID.
// Returns mongo.ErrNoDocuments if the player is not found.
func (ps *PlayerStore) GetPlayerByUUID(ctx context.Context, uuid string) (*models.Player, error) {
	var player models.Player
	filter := bson.M{"_id": uuid}

	err := ps.collection.FindOne(ctx, filter).Decode(&player)
	if err != nil {
		return nil, err
	}
	return &player, nil
}

// GetOrCreatePlayer attempts to get a player by UUID. If not found, it creates a new player.
// It tries to fetch the username from Mojang's API for new players.
// It returns the player and a boolean indicating if the player was newly created.
func (ps *PlayerStore) GetOrCreatePlayer(ctx context.Context, uuid string) (*models.Player, bool, error) {
	now := time.Now()        // Current time for timestamps
	var player models.Player // Will hold the returned player document

	filter := bson.M{"_id": uuid} // Filter by UUID (which is MongoDB's _id)

	// $set: Fields to update/set on every call (for both existing and new documents)
	setUpdates := bson.M{
		"last_login_at": &now, // Always update last login
	}

	// $setOnInsert: Fields to set ONLY if a new document is inserted (upsert: true)
	// These fields will be ignored if the document already exists.
	teams := []string{"AQUA_CREEPERS", "PURPLE_SWORDERS"} // Ensure 'teams' is defined/accessible
	assignedTeam := teams[rand.Intn(len(teams))]

	setOnInsertFields := bson.M{
		"username":             "", // Placeholder; will be fetched async
		"team":                 assignedTeam,
		"total_playtime_ticks": 0.0,
		"delta_playtime_ticks": 0.0,
		"banned":               false,
		// ban_expires_at will be nil/zero value by default if not set here
		"created_at": &now, // Set creation time only if new
	}

	// Combine $set and $setOnInsert operations
	update := bson.M{
		"$set":         setUpdates,
		"$setOnInsert": setOnInsertFields,
	}

	// First, use UpdateOne with upsert to determine if document was created
	updateOpts := options.Update().SetUpsert(true)
	updateResult, err := ps.collection.UpdateOne(ctx, filter, update, updateOpts)
	if err != nil {
		return nil, false, fmt.Errorf("failed to upsert player %s: %w", uuid, err)
	}

	// Check if this was an upsert (new document created)
	wasCreated := updateResult.UpsertedID != nil

	// Now fetch the document to return it
	err = ps.collection.FindOne(ctx, filter).Decode(&player)
	if err != nil {
		return nil, false, fmt.Errorf("failed to fetch player %s after upsert: %w", uuid, err)
	}

	log.Printf("Player %s (%s) processed. Created: %t. Team: %s, LastLogin: %v",
		player.UUID, player.Username, wasCreated, player.Team, player.LastLoginAt)

	// Asynchronously fetch username for newly created players
	// This part should only run if 'wasCreated' is true AND username is still empty.
	if wasCreated {
		log.Printf("Initiating async Mojang fetch for new player %s.", player.UUID)
		go func(playerUUID string) { // Pass UUID to goroutine to avoid closure issues
			mojangCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			username, mojangErr := ps.mojangClient.GetUsernameByUUID(mojangCtx, playerUUID)
			if mojangErr != nil {
				log.Printf("WARN: Failed to fetch username from Mojang for UUID %s: %v", playerUUID, mojangErr)
				return
			}

			// Update only if username is different from what was initially stored (empty)
			if player.Username != username {
				updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer updateCancel()

				if updateErr := ps.UpdatePlayerUsername(updateCtx, playerUUID, username); updateErr != nil {
					log.Printf("WARN: Failed to update username for player %s in DB: %v", playerUUID, updateErr)
				} else {
					log.Printf("INFO: Successfully updated username for player %s to %s.", playerUUID, username)
					// If you want the in-memory 'player' object to reflect the update immediately,
					// you'd also do player.Username = username here, but the primary response was already sent.
				}
			}
		}(uuid) // Pass 'uuid' to the goroutine
	}

	return &player, wasCreated, nil
}

// UpdatePlayerUsername updates only the Username field for a player.
func (ps *PlayerStore) UpdatePlayerUsername(ctx context.Context, uuid, username string) error {
	filter := bson.M{"_id": uuid}
	update := bson.M{"$set": bson.M{"username": username}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update username for player %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player %s not found for username update", uuid)
	}
	log.Printf("Updated username for player %s to %s. Matched %d, Modified %d.", uuid, username, result.MatchedCount, result.ModifiedCount)
	return nil
}

// UpdatePlayerPlaytime ... (rest of UpdatePlayerPlaytime code)
func (ps *PlayerStore) UpdatePlayerPlaytime(ctx context.Context, uuid string, newTotalPlaytime float64) error {
	filter := bson.M{"_id": uuid}
	update := bson.M{"$set": bson.M{"total_playtime_ticks": newTotalPlaytime}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to set playtime for player %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player %s not found for playtime update", uuid)
	}
	log.Printf("Set total playtime for player %s to %.2f ticks. Matched %d, Modified %d.", uuid, newTotalPlaytime, result.MatchedCount, result.ModifiedCount)
	return nil
}

// UpdatePlayerDeltaPlaytime ... (rest of UpdatePlayerDeltaPlaytime code)
func (ps *PlayerStore) UpdatePlayerDeltaPlaytime(ctx context.Context, uuid string, newDeltaPlaytime float64) error {
	filter := bson.M{"_id": uuid}
	update := bson.M{"$set": bson.M{"delta_playtime_ticks": newDeltaPlaytime}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to set delta playtime for player %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player %s not found for delta playtime update", uuid)
	}
	log.Printf("Set delta playtime for player %s to %.2f ticks. Matched %d, Modified %d.", uuid, newDeltaPlaytime, result.MatchedCount, result.ModifiedCount)
	return nil
}

// UpdatePlayerBanStatus ... (rest of UpdatePlayerBanStatus code)
func (ps *PlayerStore) UpdatePlayerBanStatus(ctx context.Context, uuid string, banned bool, expiresAt *time.Time) error {
	filter := bson.M{"_id": uuid}
	update := bson.M{"$set": bson.M{"banned": banned, "ban_expires_at": expiresAt}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update ban status for player %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player %s not found for ban status update", uuid)
	}
	log.Printf("Updated ban status for player %s. Banned: %t, Expires: %v", uuid, banned, expiresAt)
	return nil
}

// UpdatePlayerLastLogin updates only the LastLoginAt timestamp for a player.
func (ps *PlayerStore) UpdatePlayerLastLogin(ctx context.Context, uuid string) error {
	filter := bson.M{"_id": uuid}
	now := time.Now()
	update := bson.M{"$set": bson.M{"last_login_at": &now}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update last login for player %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player %s not found for last login update", uuid)
	}
	log.Printf("Updated last login for player %s to %v. Matched %d, Modified %d.", uuid, now, result.MatchedCount, result.ModifiedCount)
	return nil
}
