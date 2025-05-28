package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/Ftotnem/Backend/go/shared/api"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive" // Make sure this is imported
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TeamService holds dependencies for HTTP handlers related to teams.
type TeamService struct {
	teamStore   *TeamStore
	playerStore *PlayerStore
}

// NewTeamService creates a new TeamService instance.
func NewTeamService(teamStore *TeamStore, playerStore *PlayerStore) *TeamService {
	return &TeamService{
		teamStore:   teamStore,
		playerStore: playerStore,
	}
}

// SyncTeamTotalsResponse defines the response body for SyncTeamTotalsHandler.
type SyncTeamTotalsResponse struct {
	TeamTotals map[string]float64 `json:"teamTotals"` // Map of teamID to calculated total playtime
	Message    string             `json:"message"`
}

// SyncTeamTotalsHandler aggregates player playtimes from MongoDB and updates team totals.
// POST /teams/sync-totals (or whatever endpoint your Player Service's SyncPlayerPlaytime calls)
// Assuming this is the handler for `/player/sync-playtime` in the Player Service
func (ts *TeamService) SyncTeamTotalsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second) // Longer timeout for aggregation
	defer cancel()

	log.Println("Starting team total playtime aggregation job...")

	// Explicit MongoDB Aggregation Pipeline using primitive.E for each element
	pipeline := mongo.Pipeline{
		primitive.D{ // Corresponds to the first stage: {$group: {...}}
			primitive.E{Key: "$group", Value: primitive.D{
				primitive.E{Key: "_id", Value: "$team"}, // Group by the "Team" field in Player model
				primitive.E{Key: "calculatedTotal", Value: primitive.D{
					primitive.E{Key: "$sum", Value: "$total_playtime_ticks"},
				}},
			}},
		},
	}

	cursor, err := ts.playerStore.collection.Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("Error running aggregation for team totals: %v", err)
		api.WriteError(w, http.StatusInternalServerError, "Failed to aggregate team totals: "+err.Error())
		return
	}
	defer cursor.Close(ctx)

	teamTotalsMap := make(map[string]float64)

	// Iterate through aggregation results and update MongoDB Team collection
	for cursor.Next(ctx) {
		var result struct {
			TeamID          string  `bson:"_id"` // Matches the _id from $group
			CalculatedTotal float64 `bson:"calculatedTotal"`
		}
		if err := cursor.Decode(&result); err != nil {
			log.Printf("Error decoding aggregation result: %v", err)
			continue // Log and continue for other teams
		}

		// Update the team's total_playtime_ticks in MongoDB
		filter := bson.M{"_id": result.TeamID}
		update := bson.M{
			"$set": bson.M{"total_playtime_ticks": result.CalculatedTotal, "last_updated": time.Now()},
		}
		opts := options.Update().SetUpsert(true) // Ensure the team document exists

		_, err := ts.teamStore.collection.UpdateOne(ctx, filter, update, opts)
		if err != nil {
			log.Printf("ERROR: Failed to update total playtime for team %s in MongoDB: %v", result.TeamID, err)
			// Decide if you want to stop or continue. For an aggregation job, often continue.
		} else {
			teamTotalsMap[result.TeamID] = result.CalculatedTotal
			log.Printf("INFO: Successfully updated MongoDB total playtime for team '%s' to %.2f ticks.", result.TeamID, result.CalculatedTotal)
		}
	}

	if err := cursor.Err(); err != nil {
		log.Printf("Error after aggregation cursor iteration: %v", err)
		// This might be an error during cursor iteration, not necessarily the aggregation itself
	}

	log.Println("Team total playtime aggregation job finished.")
	api.WriteJSON(w, http.StatusOK, SyncTeamTotalsResponse{
		TeamTotals: teamTotalsMap,
		Message:    "Team totals aggregated and updated in MongoDB successfully. Redis will be updated by the Game Service.",
	})
}
