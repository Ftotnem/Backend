// go/player-data-service/datastore.go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/Ftotnem/Backend/go/shared/models"
)

// PlayerStore ... (rest of PlayerStore code)
type PlayerStore struct {
	collection *mongo.Collection
}

// NewPlayerStore ... (rest of NewPlayerStore code)
func NewPlayerStore(client *mongo.Client, databaseName, collectionName string) *PlayerStore {
	collection := client.Database(databaseName).Collection(collectionName)
	return &PlayerStore{collection: collection}
}

// ConnectMongoDB ... (rest of ConnectMongoDB code)
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

// CreatePlayer ... (rest of CreatePlayer code)
func (ps *PlayerStore) CreatePlayer(ctx context.Context, player *models.Player) error {
	if player.CreatedAt == nil || player.CreatedAt.IsZero() {
		now := time.Now()
		player.CreatedAt = &now
	}

	opts := options.Update().SetUpsert(true)
	filter := bson.M{"_id": player.UUID}
	update := bson.M{"$set": player}

	_, err := ps.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to create/upsert player %s: %w", player.UUID, err)
	}

	player.IsNewPlayer = true

	log.Printf("Player %s created/upserted successfully.", player.UUID)
	return nil
}

// GetPlayerByUUID ... (rest of GetPlayerByUUID code)
func (ps *PlayerStore) GetPlayerByUUID(ctx context.Context, uuid string) (*models.Player, error) {
	var player models.Player
	filter := bson.M{"_id": uuid}

	err := ps.collection.FindOne(ctx, filter).Decode(&player)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("player %s not found", uuid)
		}
		return nil, fmt.Errorf("failed to get player %s: %w", uuid, err)
	}
	return &player, nil
}

// UpdatePlayerPlaytime sets a player's total playtime ticks to a new absolute value.
// This is called by the Game State Service periodically to persist accumulated ticks from Redis.
func (ps *PlayerStore) UpdatePlayerPlaytime(ctx context.Context, uuid string, newTotalPlaytime float64) error { // CHANGED PARAM TO float64
	filter := bson.M{"_id": uuid}
	update := bson.M{"$set": bson.M{"total_playtime_ticks": newTotalPlaytime}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to set playtime for player %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player %s not found for playtime update", uuid)
	}
	// Use %f for float64, and you might want to specify precision (e.g., %.2f for 2 decimal places)
	log.Printf("Set total playtime for player %s to %.2f ticks. Matched %d, Modified %d.", uuid, newTotalPlaytime, result.MatchedCount, result.ModifiedCount)
	return nil
}

// UpdatePlayerDeltaPlaytime sets a player's delta playtime ticks to a new absolute value. Delta playtime is ticks per tick.
// This is called by the Game State Service periodically to persist the current delta ticks from Redis.

func (ps *PlayerStore) UpdatePlayerDeltaPlaytime(ctx context.Context, uuid string, newDeltaPlaytime float64) error {
	filter := bson.M{"_id": uuid}
	update := bson.M{"$set": bson.M{"delta_playtime_ticks": newDeltaPlaytime}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to set delta playtime for player %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player %s not found for playtime update", uuid)
	}
	// Use %f for float64, and you might want to specify precision (e.g., %.2f for 2 decimal places)
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
