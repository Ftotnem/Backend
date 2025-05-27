package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9" // Import the go-redis library
)

// Define a custom error for when a Redis key is not found
var ErrRedisKeyNotFound = fmt.Errorf("redis key not found")

// RedisClient wraps the go-redis client and provides methods for game-service operations.
type RedisClient struct {
	client    *redis.ClusterClient // Use redis.ClusterClient
	onlineTTL time.Duration        // TTL for online status keys
}

// Key constants for Redis
const (
	// CHANGE: Use hash tags around the UUID to ensure keys related to the same UUID
	// hash to the same slot in a Redis Cluster.
	OnlineKeyPrefix         = "online:{%s}:"         // Key for player online status: online:{uuid}
	PlaytimeKeyPrefix       = "playtime:{%s}:"       // Key for total playtime: playtime:{uuid}
	DeltaPlaytimeKeyPrefix  = "deltatime:{%s}:"      // Key for delta playtime since last persist: deltatime:{uuid}
	BannedKeyPrefix         = "banned:{%s}:"         // Key for player ban status: banned:{uuid}
	PlayerTeamKeyPrefix     = "team:{%s}:"           // Key for player's assigned team: team:{uuid}
	TeamTotalPlaytimePrefix = "team_total_playtime:" // Key for total playtime of a team: team_total_playtime:<teamID>
)

// NewRedisClient initializes a new Redis client.
// It takes the Redis address(es) and the online status TTL from the configuration.
func NewRedisClient(addrs []string, onlineTTL time.Duration) (*RedisClient, error) {
	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: addrs,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis Cluster: %w", err)
	}

	log.Printf("Successfully connected to Redis Cluster at %v", addrs)

	return &RedisClient{
		client:    rdb,
		onlineTTL: onlineTTL,
	}, nil
}

// Close closes the Redis client connection.
func (rc *RedisClient) Close() error {
	return rc.client.Close()
}

// Helper function to format keys with the UUID hash tag
func playerKey(prefix, uuid string) string {
	return fmt.Sprintf(prefix, uuid)
}

// SetOnlineStatus sets a player's online status in Redis with a TTL.
func (rc *RedisClient) SetOnlineStatus(ctx context.Context, uuid string) error {
	key := playerKey(OnlineKeyPrefix, uuid)
	return rc.client.Set(ctx, key, "true", rc.onlineTTL).Err()
}

// SetOfflineStatus removes a player's online status from Redis.
func (rc *RedisClient) SetOfflineStatus(ctx context.Context, uuid string) error {
	key := playerKey(OnlineKeyPrefix, uuid)
	return rc.client.Del(ctx, key).Err()
}

// IsOnline checks if a player is currently marked as online in Redis.
func (rc *RedisClient) IsOnline(ctx context.Context, uuid string) (bool, error) {
	key := playerKey(OnlineKeyPrefix, uuid)
	exists, err := rc.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check online status for %s: %w", uuid, err)
	}
	return exists == 1, nil
}

// SetBanStatus sets or removes a player's ban status in Redis with a TTL.
func (rc *RedisClient) SetBanStatus(ctx context.Context, uuid string, banned bool, banExpiresAt int64) error {
	key := playerKey(BannedKeyPrefix, uuid)

	if banned {
		if banExpiresAt > 0 { // Temporary ban
			duration := time.Until(time.Unix(banExpiresAt, 0))
			if duration < 0 {
				duration = 1 * time.Millisecond
			}
			return rc.client.Set(ctx, key, banExpiresAt, duration).Err()
		} else { // Permanent ban (banExpiresAt == 0)
			return rc.client.Set(ctx, key, banExpiresAt, 0).Err()
		}
	} else {
		return rc.client.Del(ctx, key).Err()
	}
}

// IsBanned checks if a player is currently marked as banned in Redis and if the ban is still active.
func (rc *RedisClient) IsBanned(ctx context.Context, uuid string) (bool, error) {
	key := playerKey(BannedKeyPrefix, uuid)
	val, err := rc.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get ban status for %s: %w", uuid, err)
	}

	expiresAt, parseErr := strconv.ParseInt(val, 10, 64)
	if parseErr != nil {
		log.Printf("WARNING: Ban status for %s has non-timestamp value: %s. Treating as not banned.", uuid, val)
		return false, nil
	}

	if expiresAt > 0 && time.Now().Unix() >= expiresAt {
		go func() {
			delCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := rc.client.Del(delCtx, key).Err(); err != nil {
				log.Printf("Error deleting expired ban key %s: %v", key, err)
			}
		}()
		return false, nil
	}

	return true, nil
}

// SetPlayerTeam sets a player's assigned team in Redis.
func (rc *RedisClient) SetPlayerTeam(ctx context.Context, uuid string, teamID string) error {
	key := playerKey(PlayerTeamKeyPrefix, uuid)
	return rc.client.Set(ctx, key, teamID, 0).Err()
}

// GetPlayerTeam retrieves the team ID for a given player UUID from Redis.
func (rc *RedisClient) GetPlayerTeam(ctx context.Context, uuid string) (string, error) {
	key := playerKey(PlayerTeamKeyPrefix, uuid)
	teamID, err := rc.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("team not found for player %s", uuid)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get team for player %s: %w", uuid, err)
	}
	return teamID, nil
}

// IncrementPlayerPlaytime increments a player's total playtime in Redis
// by the value stored in their delta playtime key.
// It assumes the delta playtime key holds a constant value to be added.
func (rc *RedisClient) IncrementPlayerPlaytime(ctx context.Context, uuid string) error {
	deltaKey := playerKey(DeltaPlaytimeKeyPrefix, uuid)
	totalPlaytimeKey := playerKey(PlaytimeKeyPrefix, uuid)

	// 1. Get the delta value as a string from Redis.
	deltaStr, err := rc.client.Get(ctx, deltaKey).Result()
	if err == redis.Nil {
		// If the delta key doesn't exist, we can assume the delta is 0
		// or decide to return an error, depending on your application's logic.
		// For now, we'll log and return, meaning no increment occurs.
		log.Printf("INFO: Delta playtime key %s not found for %s. Assuming delta is 0.", deltaKey, uuid)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get delta playtime for %s: %w", uuid, err)
	}

	// 2. Convert the string delta value to a float64.
	deltaFloat, err := strconv.ParseFloat(deltaStr, 64)
	if err != nil {
		// This handles cases where the value stored under DeltaPlaytimeKeyPrefix
		// is not a valid float (e.g., it's "abc").
		return fmt.Errorf("failed to parse delta playtime '%s' for %s as float: %w", deltaStr, uuid, err)
	}

	// 3. Atomically increment the total playtime by the float value.
	// IncrByFloat is an atomic operation for the increment itself.
	return rc.client.IncrByFloat(ctx, totalPlaytimeKey, deltaFloat).Err()
}

// --- NEW RedisClient GETTER METHODS START ---

// GetPlayerPlaytime retrieves a player's total playtime from Redis.
func (rc *RedisClient) GetPlayerPlaytime(ctx context.Context, uuid string) (float64, error) {
	key := playerKey(PlaytimeKeyPrefix, uuid)
	val, err := rc.client.Get(ctx, key).Float64()
	if err == redis.Nil {
		return 0, ErrRedisKeyNotFound // Return specific error for not found
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get total playtime for %s: %w", uuid, err)
	}
	return val, nil
}

// GetDeltaPlaytime retrieves a player's delta playtime from Redis.
func (rc *RedisClient) GetDeltaPlaytime(ctx context.Context, uuid string) (float64, error) {
	key := playerKey(DeltaPlaytimeKeyPrefix, uuid)
	val, err := rc.client.Get(ctx, key).Float64()
	if err == redis.Nil {
		return 0, ErrRedisKeyNotFound // Return specific error for not found
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get delta playtime for %s: %w", uuid, err)
	}
	return val, nil
}

// --- NEW RedisClient GETTER METHODS END ---

// GetPlayerPlaytimeAndDelta fetches a player's total playtime and delta playtime from Redis.
func (rc *RedisClient) GetPlayerPlaytimeAndDelta(ctx context.Context, uuid string) (float64, float64, error) {
	totalPlaytimeKey := playerKey(PlaytimeKeyPrefix, uuid)
	deltaPlaytimeKey := playerKey(DeltaPlaytimeKeyPrefix, uuid)

	pipe := rc.client.Pipeline()
	totalCmd := pipe.Get(ctx, totalPlaytimeKey)
	deltaCmd := pipe.Get(ctx, deltaPlaytimeKey)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get player playtime and delta in pipeline: %w", err)
	}

	totalPlaytime, err := totalCmd.Float64()
	if err == redis.Nil {
		totalPlaytime = 0
	} else if err != nil {
		return 0, 0, fmt.Errorf("failed to parse total playtime for %s: %w", uuid, err)
	}

	deltaPlaytime, err := deltaCmd.Float64()
	if err == redis.Nil {
		deltaPlaytime = 0
	} else if err != nil {
		return 0, 0, fmt.Errorf("failed to parse delta playtime for %s: %w", uuid, err)
	}

	return totalPlaytime, deltaPlaytime, nil
}

// SetPlayerPlaytime sets a player's total playtime in Redis.
func (rc *RedisClient) SetPlayerPlaytime(ctx context.Context, uuid string, playtime float64) error {
	key := playerKey(PlaytimeKeyPrefix, uuid)
	return rc.client.Set(ctx, key, playtime, 0).Err()
}

// SetDeltaPlaytime sets a player's delta playtime in Redis.
func (rc *RedisClient) SetDeltaPlaytime(ctx context.Context, uuid string, deltaPlaytime float64) error {
	key := playerKey(DeltaPlaytimeKeyPrefix, uuid)
	return rc.client.Set(ctx, key, deltaPlaytime, 0).Err()
}

// CheckPlaytimeKeysExist checks if both total playtime and delta playtime keys exist for a player.
func (rc *RedisClient) CheckPlaytimeKeysExist(ctx context.Context, uuid string) (bool, bool, error) {
	totalPlaytimeKey := playerKey(PlaytimeKeyPrefix, uuid)
	deltaPlaytimeKey := playerKey(DeltaPlaytimeKeyPrefix, uuid)

	pipe := rc.client.Pipeline()
	totalExistsCmd := pipe.Exists(ctx, totalPlaytimeKey)
	deltaExistsCmd := pipe.Exists(ctx, deltaPlaytimeKey)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, false, fmt.Errorf("failed to check playtime keys existence in pipeline: %w", err)
	}

	return totalExistsCmd.Val() == 1, deltaExistsCmd.Val() == 1, nil
}

// RemovePlayerSessionData removes all player-specific keys that are only needed while online.
func (rc *RedisClient) RemovePlayerSessionData(ctx context.Context, uuid string) error {
	keysToDelete := []string{
		playerKey(OnlineKeyPrefix, uuid),
		playerKey(PlaytimeKeyPrefix, uuid),
		playerKey(DeltaPlaytimeKeyPrefix, uuid),
		playerKey(PlayerTeamKeyPrefix, uuid),
	}

	deletedCount, err := rc.client.Del(ctx, keysToDelete...).Result()
	if err != nil {
		return fmt.Errorf("failed to delete player session keys for %s: %w", uuid, err)
	}
	log.Printf("Deleted %d Redis session keys for player %s.", deletedCount, uuid)
	return nil
}

// IncrementTeamTotalPlaytime increments a team's total playtime in Redis.
func (rc *RedisClient) IncrementTeamTotalPlaytime(ctx context.Context, teamID string, ticks float64) error {
	key := TeamTotalPlaytimePrefix + teamID // No hash tag here, as teamID might not be consistent across other team-related keys
	return rc.client.IncrByFloat(ctx, key, ticks).Err()
}

// GetTeamTotalPlaytime retrieves a team's total playtime from Redis.
func (rc *RedisClient) GetTeamTotalPlaytime(ctx context.Context, teamID string) (float64, error) {
	key := TeamTotalPlaytimePrefix + teamID
	val, err := rc.client.Get(ctx, key).Float64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get team total playtime for %s: %w", teamID, err)
	}
	return val, nil
}

// GetAllOnlineUUIDs retrieves all UUIDs currently marked as online.
// This uses SCAN to iterate keys, which is suitable for production.
// In redisclient.go (GetAllOnlineUUIDs)
func (rc *RedisClient) GetAllOnlineUUIDs(ctx context.Context) ([]string, error) {
	var allOnlineUUIDs []string

	// The pattern needs to be without a hash tag for a cluster-wide scan
	// So it should be `online:*` to match `online:{uuid}:`
	scanPattern := strings.Replace(OnlineKeyPrefix, "{%s}:", "*", 1) // Converts "online:{%s}:" to "online:*:*" or "online:*" based on final form
	if !strings.HasSuffix(scanPattern, "*") {
		scanPattern += "*" // Ensure it ends with a wildcard to match anything after 'online:'
	}
	// Corrected to handle the specific format `online:{uuid}:`
	// We want to scan for "online:*" and then filter
	scanPattern = "online:*" // This will match online:{uuid}:

	// Iterate over each master node in the cluster
	// go-redis ClusterClient.ForEachMaster is the way to do this
	err := rc.client.ForEachMaster(ctx, func(ctx context.Context, client *redis.Client) error {
		iter := client.Scan(ctx, 0, scanPattern, 0).Iterator() // Run SCAN on this specific master node

		for iter.Next(ctx) {
			key := iter.Val()

			// Extract UUID from the key "online:{uuid}:"
			start := strings.Index(key, "{")
			end := strings.Index(key, "}")
			if start != -1 && end != -1 && end > start {
				uuid := key[start+1 : end]
				allOnlineUUIDs = append(allOnlineUUIDs, uuid)
			} else {
				log.Printf("WARN in GetAllOnlineUUIDs (Node %s): Could not parse UUID from key: %s", client.Options().Addr, key)
			}
		}
		if err := iter.Err(); err != nil {
			return fmt.Errorf("failed to scan on master node %s: %w", client.Options().Addr, err)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error during cluster-wide scan for online UUIDs: %w", err)
	}

	return allOnlineUUIDs, nil
}

// GetAllPlaytimeAndDeltaPlaytime fetches all playtime and delta playtime values from Redis.
func (rc *RedisClient) GetAllPlaytimeAndDeltaPlaytime(ctx context.Context) (map[string]float64, map[string]float64, error) {
	playtimes := make(map[string]float64)
	deltaPlaytimes := make(map[string]float64)

	// Fetch all playtime keys
	iterPlaytime := rc.client.Scan(ctx, 0, "playtime:{*}", 0).Iterator() // Adjusted pattern
	for iterPlaytime.Next(ctx) {
		key := iterPlaytime.Val()
		start := strings.Index(key, "{")
		end := strings.Index(key, "}")
		uuid := ""
		if start != -1 && end != -1 && end > start {
			uuid = key[start+1 : end]
		} else {
			log.Printf("WARN: Could not parse UUID from playtime key: %s", key)
			continue
		}

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
	iterDeltaPlaytime := rc.client.Scan(ctx, 0, "deltatime:{*}", 0).Iterator() // Adjusted pattern
	for iterDeltaPlaytime.Next(ctx) {
		key := iterDeltaPlaytime.Val()
		start := strings.Index(key, "{")
		end := strings.Index(key, "}")
		uuid := ""
		if start != -1 && end != -1 && end > start {
			uuid = key[start+1 : end]
		} else {
			log.Printf("WARN: Could not parse UUID from delta playtime key: %s", key)
			continue
		}

		val, err := rc.client.Get(ctx, key).Float64()
		if err != redis.Nil && err != nil { // Allow redis.Nil for keys that might not exist
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
