package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9" // Ensure you're using v9
)

// Constants for Redis keys and default intervals.
const (
	// Redis key format for the hash storing service instances: service_registry:{service_type}
	redisRegistryHashPrefix = "service_registry:"
	// Default interval for sending heartbeats
	DefaultHeartbeatInterval = 5 * time.Second
	// Default threshold for considering a service dead (e.g., 3x heartbeat interval)
	DefaultServiceTimeout = 15 * time.Second
	// Default interval for background cleanup of truly stale entries
	DefaultCleanupInterval = 30 * time.Second
)

// ServiceInfo holds metadata about a registered service instance.
type ServiceInfo struct {
	ID        string            `json:"id"`                 // Unique ID for this instance (UUID)
	Type      string            `json:"type"`               // Type of service (e.g., "game", "player", "proxy")
	IP        string            `json:"ip,omitempty"`       // IP address of the service instance
	Port      int               `json:"port,omitempty"`     // Port of the service instance (NOW int)
	LastSeen  time.Time         `json:"last_seen"`          // Last time this instance sent a heartbeat
	CreatedAt time.Time         `json:"created_at"`         // When this instance started
	Metadata  map[string]string `json:"metadata,omitempty"` // Generic additional data
}

// ServiceConfig holds the configuration for a service registering itself.
type ServiceConfig struct {
	// ServiceID: Unique ID for this specific service instance (e.g., a UUID). If empty, a new UUID will be generated.
	ServiceID string
	// ServiceType: Type of service (e.g., "game-service", "player-service", "proxy"). Required.
	ServiceType string
	// IP: IP address of this service instance. Optional.
	IP string
	// Port: Port of this service instance. Optional. (NOW int)
	Port int
	// InitialMetadata: Any additional key-value pairs to store with this service instance. Optional.
	InitialMetadata map[string]string

	// HeartbeatInterval: How often to send a heartbeat. Defaults to DefaultHeartbeatInterval.
	HeartbeatInterval time.Duration
	// HeartbeatTTL: How long an instance is considered alive without a heartbeat. Defaults to DefaultServiceTimeout.
	// This should be greater than HeartbeatInterval.
	HeartbeatTTL time.Duration
	// CleanupInterval: How often the background goroutine actively removes stale entries. Defaults to DefaultCleanupInterval.
	CleanupInterval time.Duration
}

// ServiceRegistrar manages the registration and heartbeat for a service instance in Redis using a Hash.
type ServiceRegistrar struct {
	config      ServiceConfig
	redisClient *redis.ClusterClient // Changed to redis.ClusterClient
	currentInfo ServiceInfo          // Holds the current state of this instance's info

	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc
	cleanupCtx      context.Context
	cleanupCancel   context.CancelFunc
	wg              sync.WaitGroup
	isStopped       bool
}

// NewServiceRegistrar creates a new ServiceRegistrar instance.
// `redisClient` is the Redis Cluster client to use.
// `config` provides the details for this service instance.
func NewServiceRegistrar(redisClient *redis.ClusterClient, config ServiceConfig) (*ServiceRegistrar, error) { // Changed signature
	if redisClient == nil {
		return nil, fmt.Errorf("redis client cannot be nil")
	}
	if config.ServiceType == "" {
		return nil, fmt.Errorf("service type cannot be empty")
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if config.HeartbeatTTL == 0 {
		config.HeartbeatTTL = DefaultServiceTimeout
	}
	if config.HeartbeatTTL <= config.HeartbeatInterval {
		return nil, fmt.Errorf("HeartbeatTTL (%s) must be greater than HeartbeatInterval (%s)", config.HeartbeatTTL, config.HeartbeatInterval)
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = DefaultCleanupInterval
	}

	if config.ServiceID == "" {
		config.ServiceID = uuid.New().String()
	}

	sr := &ServiceRegistrar{
		config:      config,
		redisClient: redisClient, // Assign redis.ClusterClient
		currentInfo: ServiceInfo{
			ID:        config.ServiceID,
			Type:      config.ServiceType,
			IP:        config.IP,
			Port:      config.Port, // Port is now int
			CreatedAt: time.Now(),
			Metadata:  config.InitialMetadata,
		},
	}

	sr.heartbeatCtx, sr.heartbeatCancel = context.WithCancel(context.Background())
	sr.cleanupCtx, sr.cleanupCancel = context.WithCancel(context.Background())

	return sr, nil
}

// Register registers the current service instance with Redis and starts its periodic heartbeats and cleanup.
func (sr *ServiceRegistrar) Register() error {
	// Initial registration/heartbeat
	if err := sr.sendHeartbeat(sr.heartbeatCtx); err != nil {
		return fmt.Errorf("failed initial registration/heartbeat: %w", err)
	}

	sr.wg.Add(2) // One for heartbeatLoop, one for cleanupLoop
	go sr.heartbeatLoop()
	go sr.cleanupLoop()

	log.Printf("ServiceRegistrar: Registered '%s' as ID '%s' (IP: %s, Port: %d). Heartbeat every %s, TTL %s, Cleanup %s.",
		sr.config.ServiceType, sr.config.ServiceID, sr.config.IP, sr.config.Port,
		sr.config.HeartbeatInterval, sr.config.HeartbeatTTL, sr.config.CleanupInterval)
	return nil
}

// sendHeartbeat updates the service's info in Redis.
func (sr *ServiceRegistrar) sendHeartbeat(ctx context.Context) error {
	sr.currentInfo.LastSeen = time.Now() // Update LastSeen timestamp

	infoJSON, err := json.Marshal(sr.currentInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal service info: %w", err)
	}

	key := sr.getRedisKey(sr.config.ServiceType)
	cmd := sr.redisClient.HSet(ctx, key, sr.config.ServiceID, infoJSON)
	if cmd.Err() != nil {
		return fmt.Errorf("failed to send heartbeat for %s (%s): %w", sr.config.ServiceType, sr.config.ServiceID, cmd.Err())
	}
	// log.Printf("ServiceRegistrar: Sent heartbeat for %s:%s", sr.config.ServiceType, sr.config.ServiceID) // Too noisy for production
	return nil
}

// heartbeatLoop runs in a goroutine to send periodic heartbeats.
func (sr *ServiceRegistrar) heartbeatLoop() {
	defer sr.wg.Done()
	ticker := time.NewTicker(sr.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sr.heartbeatCtx.Done():
			return
		case <-ticker.C:
			if err := sr.sendHeartbeat(sr.heartbeatCtx); err != nil {
				log.Printf("WARNING: ServiceRegistrar: Error sending heartbeat for %s (%s): %v",
					sr.config.ServiceType, sr.config.ServiceID, err)
			}
		}
	}
}

// cleanupLoop runs in a goroutine to periodically remove truly stale entries from Redis.
func (sr *ServiceRegistrar) cleanupLoop() {
	defer sr.wg.Done()
	ticker := time.NewTicker(sr.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sr.cleanupCtx.Done():
			log.Printf("ServiceRegistrar: Cleanup loop for '%s' (ID: %s) stopped.", sr.config.ServiceType, sr.config.ServiceID)
			return
		case <-ticker.C:
			key := sr.getRedisKey(sr.config.ServiceType)
			results, err := sr.redisClient.HGetAll(sr.cleanupCtx, key).Result()
			if err != nil {
				log.Printf("WARNING: ServiceRegistrar: Error getting services for cleanup of type %s: %v", sr.config.ServiceType, err)
				continue
			}

			staleIDs := []string{}
			currentTime := time.Now()
			for instanceID, infoJSON := range results {
				var info ServiceInfo
				if err := json.Unmarshal([]byte(infoJSON), &info); err != nil {
					log.Printf("WARNING: ServiceRegistrar: Failed to unmarshal ServiceInfo for ID %s during cleanup: %v", instanceID, err)
					staleIDs = append(staleIDs, instanceID) // Malformed entry, mark for deletion
					continue
				}
				if currentTime.Sub(info.LastSeen) > sr.config.HeartbeatTTL {
					staleIDs = append(staleIDs, instanceID)
				}
			}

			if len(staleIDs) > 0 {
				log.Printf("ServiceRegistrar: Cleaning up %d stale services of type '%s'.", len(staleIDs), sr.config.ServiceType)
				cmd := sr.redisClient.HDel(sr.cleanupCtx, key, staleIDs...)
				if cmd.Err() != nil {
					log.Printf("WARNING: ServiceRegistrar: Error during cleanup of stale services of type %s: %v", sr.config.ServiceType, cmd.Err())
				}
			}
		}
	}
}

// GetServiceID returns the ID of this service instance.
func (sr *ServiceRegistrar) GetServiceID() string {
	return sr.config.ServiceID
}

// GetActiveServices retrieves a map of active service instances for a given service type.
// The map key is the instance ID, and the value is the ServiceInfo.
// This function filters out services whose LastSeen timestamp is older than the HeartbeatTTL.
func (sr *ServiceRegistrar) GetActiveServices(ctx context.Context, serviceType string) (map[string]ServiceInfo, error) {
	key := sr.getRedisKey(serviceType)
	results, err := sr.redisClient.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get all services of type %s from Redis: %w", serviceType, err)
	}

	activeServices := make(map[string]ServiceInfo)
	currentTime := time.Now()

	for instanceID, infoJSON := range results {
		var info ServiceInfo
		if err := json.Unmarshal([]byte(infoJSON), &info); err != nil {
			log.Printf("WARNING: ServiceRegistrar: Failed to unmarshal ServiceInfo for ID %s (type %s): %v", instanceID, serviceType, err)
			continue // Skip malformed entries, they'll be cleaned up by cleanupLoop
		}

		// Consider service active if its last heartbeat was within the HeartbeatTTL
		if currentTime.Sub(info.LastSeen) <= sr.config.HeartbeatTTL {
			activeServices[instanceID] = info
		}
	}
	return activeServices, nil
}

// Stop gracefully stops the heartbeat and cleanup loops, and removes this service instance from Redis.
func (sr *ServiceRegistrar) Stop(ctx context.Context) {
	if sr.isStopped {
		return
	}
	sr.isStopped = true
	log.Printf("ServiceRegistrar: Stopping '%s' (ID: %s)...", sr.config.ServiceType, sr.config.ServiceID)

	sr.heartbeatCancel() // Signal heartbeat loop to stop
	sr.cleanupCancel()   // Signal cleanup loop to stop
	sr.wg.Wait()         // Wait for both goroutines to finish

	// Deregister this service instance from Redis hash
	key := sr.getRedisKey(sr.config.ServiceType)
	cmd := sr.redisClient.HDel(ctx, key, sr.config.ServiceID)
	if cmd.Err() != nil {
		log.Printf("WARNING: ServiceRegistrar: Failed to deregister service %s (%s): %v", sr.config.ServiceType, sr.config.ServiceID, cmd.Err())
	} else {
		log.Printf("ServiceRegistrar: Successfully deregistered '%s' (ID: %s).", sr.config.ServiceType, sr.config.ServiceID)
	}
}

// getRedisKey constructs the Redis hash key for a given service type.
func (sr *ServiceRegistrar) getRedisKey(serviceType string) string {
	return fmt.Sprintf("%s%s", redisRegistryHashPrefix, serviceType)
}
