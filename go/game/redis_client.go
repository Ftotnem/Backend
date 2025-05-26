package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8" // Import the go-redis library
)

// RedisClient wraps the go-redis client and provides methods for game-service operations.
type RedisClient struct {
	client    *redis.Client
	onlineTTL time.Duration // TTL for online status keys
}

// Key constants for Redis
const (
	OnlineKeyPrefix         = "online:"              // Key for player online status: online:<uuid>
	PlaytimeKeyPrefix       = "playtime:"            // Key for total playtime: playtime:<uuid>
	DeltaPlaytimeKeyPrefix  = "deltatime:"           // Key for delta playtime since last persist: deltatime:<uuid>
	BannedKeyPrefix         = "banned:"              // Key for player ban status: banned:<uuid>
	PlayerTeamKeyPrefix     = "team:"                // Key for player's assigned team: team:<uuid>
	TeamTotalPlaytimePrefix = "team_total_playtime:" // Key for total playtime of a team: team_total_playtime:<teamID>
)

// NewRedisClient initializes a new Redis client.
// It takes the Redis address and the online status TTL from the configuration.
func NewRedisClient(addr string, onlineTTL time.Duration) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   0, // Use default DB
	})

	// Ping the Redis server to ensure the connection is successful
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{
		client:    rdb,
		onlineTTL: onlineTTL,
	}, nil
}

// Close closes the Redis client connection.
func (rc *RedisClient) Close() error {
	return rc.client.Close()
}

// SetOnlineStatus sets a player's online status in Redis with a TTL.
// The client should call this periodically (e.g., every 10 seconds) to keep the player marked as online.
func (rc *RedisClient) SetOnlineStatus(ctx context.Context, uuid string) error {
	key := OnlineKeyPrefix + uuid
	// Set the key to "true" with the configured online TTL
	return rc.client.Set(ctx, key, "true", rc.onlineTTL).Err()
}

// SetOfflineStatus removes a player's online status from Redis.
func (rc *RedisClient) SetOfflineStatus(ctx context.Context, uuid string) error {
	key := OnlineKeyPrefix + uuid
	// Delete the online status key
	return rc.client.Del(ctx, key).Err()
}

// IsOnline checks if a player is currently marked as online in Redis.
func (rc *RedisClient) IsOnline(ctx context.Context, uuid string) (bool, error) {
	key := OnlineKeyPrefix + uuid
	// Check if the key exists. If it does, the player is online.
	exists, err := rc.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check online status for %s: %w", uuid, err)
	}
	return exists == 1, nil
}

// IsBanned checks if a player is currently marked as banned in Redis.
// This assumes the 'banned:<uuid>' key is set to "true" when a player is banned.
func (rc *RedisClient) IsBanned(ctx context.Context, uuid string) (bool, error) {
	key := BannedKeyPrefix + uuid
	status, err := rc.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil // Key does not exist, so not banned
	}
	if err != nil {
		return false, fmt.Errorf("failed to get ban status for %s: %w", uuid, err)
	}
	return status == "true", nil
}

// GetPlayerTeam retrieves the team ID for a given player UUID from Redis.
func (rc *RedisClient) GetPlayerTeam(ctx context.Context, uuid string) (string, error) {
	key := PlayerTeamKeyPrefix + uuid
	teamID, err := rc.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("team not found for player %s", uuid)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get team for player %s: %w", uuid, err)
	}
	return teamID, nil
}

// IncrementPlayerPlaytime increments a player's total playtime in Redis.
func (rc *RedisClient) IncrementPlayerPlaytime(ctx context.Context, uuid string, ticks float64) error {
	key := PlaytimeKeyPrefix + uuid
	return rc.client.IncrByFloat(ctx, key, ticks).Err()
}

// IncrementPlayerDeltaPlaytime increments a player's delta playtime in Redis.
func (rc *RedisClient) IncrementPlayerDeltaPlaytime(ctx context.Context, uuid string, ticks float64) error {
	key := DeltaPlaytimeKeyPrefix + uuid
	return rc.client.IncrByFloat(ctx, key, ticks).Err()
}

// IncrementTeamTotalPlaytime increments a team's total playtime in Redis.
func (rc *RedisClient) IncrementTeamTotalPlaytime(ctx context.Context, teamID string, ticks float64) error {
	key := TeamTotalPlaytimePrefix + teamID
	return rc.client.IncrByFloat(ctx, key, ticks).Err()
}

// GetTeamTotalPlaytime retrieves a team's total playtime from Redis.
func (rc *RedisClient) GetTeamTotalPlaytime(ctx context.Context, teamID string) (float64, error) {
	key := TeamTotalPlaytimePrefix + teamID
	val, err := rc.client.Get(ctx, key).Float64()
	if err == redis.Nil {
		return 0, nil // Team playtime not set yet, treat as 0
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get team total playtime for %s: %w", teamID, err)
	}
	return val, nil
}

// GetAllOnlineUUIDs retrieves all UUIDs currently marked as online.
// This uses SCAN to iterate keys, which is suitable for production.
func (rc *RedisClient) GetAllOnlineUUIDs(ctx context.Context) ([]string, error) {
	var uuids []string
	iter := rc.client.Scan(ctx, 0, OnlineKeyPrefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		// Extract UUID by removing the prefix
		if len(key) > len(OnlineKeyPrefix) {
			uuids = append(uuids, key[len(OnlineKeyPrefix):])
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan online UUIDs: %w", err)
	}
	return uuids, nil
}

// GetAllPlaytimeAndDeltaPlaytime fetches all playtime and delta playtime values from Redis.
// It returns two maps: uuid -> playtime and uuid -> deltatime.
func (rc *RedisClient) GetAllPlaytimeAndDeltaPlaytime(ctx context.Context) (map[string]float64, map[string]float64, error) {
	playtimes := make(map[string]float64)
	deltaPlaytimes := make(map[string]float64)

	// Fetch all playtime keys
	iterPlaytime := rc.client.Scan(ctx, 0, PlaytimeKeyPrefix+"*", 0).Iterator()
	for iterPlaytime.Next(ctx) {
		key := iterPlaytime.Val()
		uuid := key[len(PlaytimeKeyPrefix):]
		val, err := rc.client.Get(ctx, key).Float64()
		if err != nil {
			log.Printf("WARN: Failed to get playtime for %s: %v", uuid, err)
			continue
		}
		playtimes[uuid] = val
	}
	if err := iterPlaytime.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to scan playtime keys: %w", err)
	}

	// Fetch all delta playtime keys
	iterDeltaPlaytime := rc.client.Scan(ctx, 0, DeltaPlaytimeKeyPrefix+"*", 0).Iterator()
	for iterDeltaPlaytime.Next(ctx) {
		key := iterDeltaPlaytime.Val()
		uuid := key[len(DeltaPlaytimeKeyPrefix):]
		val, err := rc.client.Get(ctx, key).Float64()
		if err != nil {
			log.Printf("WARN: Failed to get delta playtime for %s: %v", uuid, err)
			continue
		}
		deltaPlaytimes[uuid] = val
	}
	if err := iterDeltaPlaytime.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to scan delta playtime keys: %w", err)
	}

	return playtimes, deltaPlaytimes, nil
}

// GetAllTeamTotalPlaytimes fetches all team total playtime values from Redis.
func (rc *RedisClient) GetAllTeamTotalPlaytimes(ctx context.Context) (map[string]float64, error) {
	teamPlaytimes := make(map[string]float64)
	iter := rc.client.Scan(ctx, 0, TeamTotalPlaytimePrefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		teamID := key[len(TeamTotalPlaytimePrefix):]
		val, err := rc.client.Get(ctx, key).Float64()
		if err != nil {
			log.Printf("WARN: Failed to get team total playtime for %s: %v", teamID, err)
			continue
		}
		teamPlaytimes[teamID] = val
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan team total playtime keys: %w", err)
	}
	return teamPlaytimes, nil
}

// ResetDeltaPlaytime sets a player's delta playtime to 0 after persistence.
func (rc *RedisClient) ResetDeltaPlaytime(ctx context.Context, uuid string) error {
	key := DeltaPlaytimeKeyPrefix + uuid
	return rc.client.Set(ctx, key, 0, 0).Err() // Set to 0 with no expiration
}

// SetPlayerPlaytime sets a player's total playtime in Redis.
// This is primarily for the persister to update the total playtime after fetching from Redis.
func (rc *RedisClient) SetPlayerPlaytime(ctx context.Context, uuid string, playtime float64) error {
	key := PlaytimeKeyPrefix + uuid
	return rc.client.Set(ctx, key, playtime, 0).Err() // Set with no expiration
}

// SetTeamTotalPlaytime sets a team's total playtime in Redis.
// This is primarily for the persister to update the total playtime after fetching from Redis.
func (rc *RedisClient) SetTeamTotalPlaytime(ctx context.Context, teamID string, totalTicks float64) error {
	key := TeamTotalPlaytimePrefix + teamID
	return rc.client.Set(ctx, key, totalTicks, 0).Err() // Set with no expiration
}
