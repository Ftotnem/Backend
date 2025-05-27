// team_store.go (or within main package)
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/Ftotnem/Backend/go/shared/models"
)

// TeamStore represents the MongoDB data store for team profiles.
type TeamStore struct {
	collection *mongo.Collection
}

// NewTeamStore creates a new TeamStore instance.
func NewTeamStore(client *mongo.Client, databaseName, collectionName string) *TeamStore {
	collection := client.Database(databaseName).Collection(collectionName)
	return &TeamStore{
		collection: collection,
	}
}

// EnsureTeamsExist initializes default team documents if they don't exist.
func (ts *TeamStore) EnsureTeamsExist(ctx context.Context, teams []string) error {
	for _, teamName := range teams {
		filter := bson.M{"_id": teamName}
		update := bson.M{
			"$setOnInsert": bson.M{
				"player_count":         0,
				"total_playtime_ticks": 0.0,
				"created_at":           time.Now(),
				"last_updated":         time.Now(),
			},
		}
		opts := options.Update().SetUpsert(true) // Upsert will insert if not found

		result, err := ts.collection.UpdateOne(ctx, filter, update, opts)
		if err != nil {
			return fmt.Errorf("failed to upsert team %s: %w", teamName, err)
		}
		if result.UpsertedID != nil {
			log.Printf("INFO: Initialized team '%s' in database.", teamName)
		}
	}
	return nil
}

// GetTeamPlayerCount retrieves the current player count for a given team.
func (ts *TeamStore) GetTeamPlayerCount(ctx context.Context, teamName string) (int64, error) {
	var team models.Team
	filter := bson.M{"_id": teamName}

	err := ts.collection.FindOne(ctx, filter).Decode(&team)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, nil // Team not found, assume 0 players (though EnsureTeamsExist should prevent this)
		}
		return 0, fmt.Errorf("failed to get player count for team %s: %w", teamName, err)
	}
	return team.PlayerCount, nil
}

// IncrementTeamPlayerCount atomically increments the player count for a team.
func (ts *TeamStore) IncrementTeamPlayerCount(ctx context.Context, teamName string) error {
	filter := bson.M{"_id": teamName}
	update := bson.M{
		"$inc": bson.M{"player_count": 1},
		"$set": bson.M{"last_updated": time.Now()},
	}

	result, err := ts.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to increment player count for team %s: %w", teamName, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("team %s not found for player count increment", teamName)
	}
	log.Printf("INFO: Incremented player count for team %s.", teamName)
	return nil
}

// DecrementTeamPlayerCount atomically decrements the player count for a team.
// (You might need this if players leave a team or are deleted)
func (ts *TeamStore) DecrementTeamPlayerCount(ctx context.Context, teamName string) error {
	filter := bson.M{"_id": teamName}
	update := bson.M{
		"$inc": bson.M{"player_count": -1},
		"$set": bson.M{"last_updated": time.Now()},
	}

	result, err := ts.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to decrement player count for team %s: %w", teamName, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("team %s not found for player count decrement", teamName)
	}
	log.Printf("INFO: Decremented player count for team %s.", teamName)
	return nil
}

// UpdateTeamTotalPlaytime atomically increments the total playtime for a team.
func (ts *TeamStore) UpdateTeamTotalPlaytime(ctx context.Context, teamName string, playtimeIncrement float64) error {
	filter := bson.M{"_id": teamName}
	update := bson.M{
		"$inc": bson.M{"total_playtime_ticks": playtimeIncrement},
		"$set": bson.M{"last_updated": time.Now()},
	}

	result, err := ts.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update total playtime for team %s: %w", teamName, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("team %s not found for total playtime update", teamName)
	}
	log.Printf("INFO: Updated total playtime for team %s by %.2f ticks.", teamName, playtimeIncrement)
	return nil
}

// GetAllTeams retrieves all team documents.
func (ts *TeamStore) GetAllTeams(ctx context.Context) ([]models.Team, error) {
	var teams []models.Team
	cursor, err := ts.collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to find all teams: %w", err)
	}
	defer cursor.Close(ctx)

	if err = cursor.All(ctx, &teams); err != nil {
		return nil, fmt.Errorf("failed to decode all teams: %w", err)
	}
	return teams, nil
}
