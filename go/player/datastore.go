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
	mojangClient *MojangClient
	teamStore    *TeamStore
}

// NewPlayerStore creates a new PlayerStore instance.
func NewPlayerStore(client *mongo.Client, databaseName, collectionName string, mojangClient *MojangClient, teamStore *TeamStore) *PlayerStore {
	collection := client.Database(databaseName).Collection(collectionName)
	return &PlayerStore{
		collection:   collection,
		mojangClient: mojangClient,
		teamStore:    teamStore,
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

// CreateProfile inserts a new player document (profile) into the collection.
// It will error if a profile with the same UUID already exists.
// This function also initializes default fields and attempts to fetch the username.
func (ps *PlayerStore) CreateProfile(ctx context.Context, playerUUID string) (*models.Player, error) {
	now := time.Now()
	allTeams := []string{"AQUA_CREEPERS", "PURPLE_SWORDERS"}

	// Determine the team with the least players
	var assignedTeam string
	minPlayers := int64(-1) // Initialize with a value that any real count will be less than

	// Fetch player counts for all teams
	teamCounts := make(map[string]int64)
	for _, teamName := range allTeams {
		count, err := ps.teamStore.GetTeamPlayerCount(ctx, teamName)
		if err != nil {
			// Log warning but proceed, default to random if counts can't be fetched
			log.Printf("WARN: Could not retrieve player count for team %s: %v. Falling back to random assignment if other teams also fail.", teamName, err)
			teamCounts[teamName] = -1 // Indicate an error, effectively making it undesirable
		} else {
			teamCounts[teamName] = count
		}
	}

	// Find the team with the minimum number of players
	leastPopulatedTeams := []string{}
	for _, teamName := range allTeams {
		count := teamCounts[teamName]
		if count == -1 { // Skip teams that failed to fetch
			continue
		}

		if minPlayers == -1 || count < minPlayers {
			minPlayers = count
			leastPopulatedTeams = []string{teamName} // Start new list if a new minimum is found
		} else if count == minPlayers {
			leastPopulatedTeams = append(leastPopulatedTeams, teamName) // Add to list if tied
		}
	}

	if len(leastPopulatedTeams) > 0 {
		// If there are teams to choose from, pick one randomly from the least populated
		assignedTeam = leastPopulatedTeams[rand.Intn(len(leastPopulatedTeams))]
		log.Printf("INFO: Assigned player %s to team %s (least populated).", playerUUID, assignedTeam)
	} else {
		// Fallback: if no team counts could be fetched (e.g., all failed), assign randomly
		assignedTeam = allTeams[rand.Intn(len(allTeams))]
		log.Printf("WARN: Could not determine least populated team. Assigned player %s to team %s randomly.", playerUUID, assignedTeam)
	}

	newProfile := &models.Player{
		UUID:               playerUUID,
		Username:           "", // Placeholder, will be filled by Mojang API
		Team:               assignedTeam,
		TotalPlaytimeTicks: 0.0,
		DeltaPlaytimeTicks: 1.0,
		Banned:             false,
		CreatedAt:          &now,
		LastLoginAt:        &now,
	}

	_, err := ps.collection.InsertOne(ctx, newProfile)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("player profile %s already exists", playerUUID)
		}
		return nil, fmt.Errorf("failed to create player profile %s: %w", playerUUID, err)
	}

	log.Printf("Player profile %s created successfully with default values.", playerUUID)

	// Increment the player count for the assigned team
	go func(team string) {
		teamCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ps.teamStore.IncrementTeamPlayerCount(teamCtx, team); err != nil {
			log.Printf("ERROR: Failed to increment player count for team %s after creating profile %s: %v", team, playerUUID, err)
		}
	}(assignedTeam)

	// Asynchronously fetch username for the newly created profile
	go func(uuid string) {
		mojangCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		username, mojangErr := ps.mojangClient.GetUsernameByUUID(mojangCtx, uuid)
		if mojangErr != nil {
			log.Printf("WARN: Failed to fetch username from Mojang for UUID %s: %v", uuid, mojangErr)
			return
		}

		updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer updateCancel()

		if updateErr := ps.UpdateProfileUsername(updateCtx, uuid, username); updateErr != nil {
			log.Printf("WARN: Failed to update username for player profile %s in DB: %v", uuid, updateErr)
		} else {
			log.Printf("INFO: Successfully updated username for player profile %s to %s.", uuid, username)
			newProfile.Username = username // Update in-memory struct for immediate return (though not strictly necessary as response is already sent)
		}
	}(playerUUID)

	return newProfile, nil
}

// GetProfileByUUID retrieves a player profile by their UUID.
// Returns mongo.ErrNoDocuments if the player profile is not found.
func (ps *PlayerStore) GetProfileByUUID(ctx context.Context, uuid string) (*models.Player, error) {
	var profile models.Player
	filter := bson.M{"_id": uuid}

	err := ps.collection.FindOne(ctx, filter).Decode(&profile)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// UpdateProfileUsername updates only the Username field for a player profile.
func (ps *PlayerStore) UpdateProfileUsername(ctx context.Context, uuid, username string) error {
	filter := bson.M{"_id": uuid}
	update := bson.M{"$set": bson.M{"username": username}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update username for player profile %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player profile %s not found for username update", uuid)
	}
	log.Printf("Updated username for player profile %s to %s. Matched %d, Modified %d.", uuid, username, result.MatchedCount, result.ModifiedCount)
	return nil
}

// UpdateProfilePlaytime updates a player profile's total playtime.
func (ps *PlayerStore) UpdateProfilePlaytime(ctx context.Context, uuid string, newTotalPlaytime float64) error {
	filter := bson.M{"_id": uuid}
	update := bson.M{"$set": bson.M{"total_playtime_ticks": newTotalPlaytime}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to set playtime for player profile %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player profile %s not found for playtime update", uuid)
	}
	log.Printf("Set total playtime for player profile %s to %.2f ticks. Matched %d, Modified %d.", uuid, newTotalPlaytime, result.MatchedCount, result.ModifiedCount)
	return nil
}

// UpdateProfileDeltaPlaytime updates a player profile's delta playtime.
func (ps *PlayerStore) UpdateProfileDeltaPlaytime(ctx context.Context, uuid string, newDeltaPlaytime float64) error {
	filter := bson.M{"_id": uuid}
	update := bson.M{"$set": bson.M{"delta_playtime_ticks": newDeltaPlaytime}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to set delta playtime for player profile %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player profile %s not found for delta playtime update", uuid)
	}
	log.Printf("Set delta playtime for player profile %s to %.2f ticks. Matched %d, Modified %d.", uuid, newDeltaPlaytime, result.MatchedCount, result.ModifiedCount)
	return nil
}

// UpdateProfileBanStatus updates a player profile's ban status.
func (ps *PlayerStore) UpdateProfileBanStatus(ctx context.Context, uuid string, banned bool, expiresAt *time.Time) error {
	filter := bson.M{"_id": uuid}
	update := bson.M{"$set": bson.M{"banned": banned, "ban_expires_at": expiresAt}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update ban status for player profile %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player profile %s not found for ban status update", uuid)
	}
	log.Printf("Updated ban status for player profile %s. Banned: %t, Expires: %v", uuid, banned, expiresAt)
	return nil
}

// UpdateProfileLastLogin updates only the LastLoginAt timestamp for a player profile.
func (ps *PlayerStore) UpdateProfileLastLogin(ctx context.Context, uuid string) error {
	filter := bson.M{"_id": uuid}
	now := time.Now()
	update := bson.M{"$set": bson.M{"last_login_at": &now}}

	result, err := ps.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update last login for player profile %s: %w", uuid, err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("player profile %s not found for last login update", uuid)
	}
	log.Printf("Updated last login for player profile %s to %v. Matched %d, Modified %d.", uuid, now, result.MatchedCount, result.ModifiedCount)
	return nil
}
