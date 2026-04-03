// Package node provides node registration and health tracking for distributed barnacle clusters.
// It manages the current node's status in Redis and provides access to other nodes' information.
package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pdylanross/barnacle/internal/tasks"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Redis key prefix for node information.
const keyPrefixNode = "barnacle:node:"

// Status represents the current state of a node.
type Status string

const (
	// StatusHealthy indicates the node is operating normally.
	StatusHealthy Status = "healthy"
	// StatusStarting indicates the node is still initializing.
	StatusStarting Status = "starting"
	// StatusDegraded indicates the node is experiencing issues but still operational.
	StatusDegraded Status = "degraded"
)

// Stats contains high-level statistics about a node.
type Stats struct {
	// TierDiskUsage contains disk usage statistics for each cache tier.
	// The index corresponds to the tier number (0 = highest priority tier).
	TierDiskUsage []DiskUsageStats `json:"tierDiskUsage,omitempty"`
}

// Info contains information about a node in the cluster.
type Info struct {
	// NodeID is the unique identifier for this node.
	NodeID string `json:"nodeId"`
	// Status indicates the current operational state of the node.
	Status Status `json:"status"`
	// LastUpdated is the timestamp when this information was last updated.
	LastUpdated time.Time `json:"lastUpdated"`
	// Stats contains high-level statistics about the node.
	Stats Stats `json:"stats"`
}

// Registry manages node registration and health tracking.
type Registry struct {
	config         *configuration.NodeHealthConfig
	cacheConfig    *configuration.CacheConfiguration
	tierSizeLimits []uint64 // Pre-parsed size limits per tier (0 = no limit)
	logger         *zap.Logger
	redisClient    *redis.Client

	nodeID string
	status Status
}

// NewRegistry creates a new node registry.
func NewRegistry(
	config *configuration.NodeHealthConfig,
	cacheConfig *configuration.CacheConfiguration,
	logger *zap.Logger,
	redisClient *redis.Client,
) (*Registry, error) {
	nodeID := config.NodeID
	if nodeID == "" {
		nodeID = os.Getenv("BARNACLE_NODE_ID")
	}
	if nodeID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("failed to get hostname for node ID: %w", err)
		}
		nodeID = hostname
	}

	// Pre-parse tier size limits to avoid repeated string parsing
	var tierSizeLimits []uint64
	if cacheConfig != nil {
		tierSizeLimits = make([]uint64, len(cacheConfig.Disk.Tiers))
		for i, tier := range cacheConfig.Disk.Tiers {
			limit, err := tier.GetSizeLimitBytes()
			if err != nil {
				return nil, fmt.Errorf("tier %d: %w", tier.Tier, err)
			}
			tierSizeLimits[i] = limit
		}
	}

	r := &Registry{
		config:         config,
		cacheConfig:    cacheConfig,
		tierSizeLimits: tierSizeLimits,
		logger:         logger.Named("node-registry"),
		redisClient:    redisClient,
		nodeID:         nodeID,
		status:         StatusStarting,
	}

	r.logger.Info("node registry initialized",
		zap.String("nodeID", nodeID),
		zap.Duration("syncInterval", config.GetSyncInterval()),
		zap.Duration("ttl", config.GetTTL()))

	return r, nil
}

// NodeID returns the unique identifier for this node.
func (r *Registry) NodeID() string {
	return r.nodeID
}

// SetStatus updates the node's operational status.
func (r *Registry) SetStatus(status Status) {
	r.status = status
}

// GetNodeInfo returns the current node information for this node.
func (r *Registry) GetNodeInfo() *Info {
	return &Info{
		NodeID:      r.nodeID,
		Status:      r.status,
		LastUpdated: time.Now(),
		Stats:       r.collectStats(),
	}
}

// collectStats gathers statistics about this node.
func (r *Registry) collectStats() Stats {
	stats := Stats{}

	if r.cacheConfig == nil {
		return stats
	}

	tiers := r.cacheConfig.Disk.Tiers
	stats.TierDiskUsage = make([]DiskUsageStats, len(tiers))

	for i, tier := range tiers {
		diskUsage, err := GetDiskUsage(tier.Path)
		if err != nil {
			r.logger.Warn("failed to get disk usage for tier",
				zap.Int("tier", tier.Tier),
				zap.String("path", tier.Path),
				zap.Error(err))
			// Set empty stats with just the path for this tier
			stats.TierDiskUsage[i] = DiskUsageStats{Path: tier.Path}
			continue
		}

		// Apply pre-parsed size limit if set
		if i < len(r.tierSizeLimits) && r.tierSizeLimits[i] > 0 {
			diskUsage = ApplySizeLimit(diskUsage, r.tierSizeLimits[i])
		}

		stats.TierDiskUsage[i] = *diskUsage
	}

	return stats
}

// redisKey returns the Redis key for a node.
func redisKey(nodeID string) string {
	return keyPrefixNode + nodeID
}

// sync saves the current node's information to Redis.
func (r *Registry) sync(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	info := r.GetNodeInfo()
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal node info: %w", err)
	}

	key := redisKey(r.nodeID)
	ttl := r.config.GetTTL()

	if setErr := r.redisClient.Set(ctx, key, data, ttl).Err(); setErr != nil {
		return fmt.Errorf("failed to save node info to Redis: %w", setErr)
	}

	return nil
}

// MakeTask creates a repeating task that syncs node info to Redis.
func (r *Registry) MakeTask() tasks.Task {
	return tasks.NewRepeating(r.config.GetSyncInterval(), func(ctx context.Context) error {
		// After first successful sync, mark as healthy
		if r.status == StatusStarting {
			r.status = StatusHealthy
		}
		return r.sync(ctx)
	})
}

// ErrNodeNotFound is returned when a node is not found in Redis.
var ErrNodeNotFound = errors.New("node not found")

// GetNode retrieves the node information for a specific node ID.
func (r *Registry) GetNode(ctx context.Context, nodeID string) (*Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := redisKey(nodeID)
	data, err := r.redisClient.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get node info from Redis: %w", err)
	}

	var info Info
	if unmarshalErr := json.Unmarshal([]byte(data), &info); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal node info: %w", unmarshalErr)
	}

	return &info, nil
}

// ListNodes returns information about all nodes currently registered in Redis.
func (r *Registry) ListNodes(ctx context.Context) ([]*Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Find all node keys
	pattern := keyPrefixNode + "*"
	keys, err := r.redisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list node keys from Redis: %w", err)
	}

	if len(keys) == 0 {
		return []*Info{}, nil
	}

	// Get all node values
	values, err := r.redisClient.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get node info from Redis: %w", err)
	}

	nodes := make([]*Info, 0, len(values))
	for i, val := range values {
		if val == nil {
			continue
		}

		data, ok := val.(string)
		if !ok {
			r.logger.Warn("unexpected value type for node key",
				zap.String("key", keys[i]))
			continue
		}

		var info Info
		if unmarshalErr := json.Unmarshal([]byte(data), &info); unmarshalErr != nil {
			r.logger.Warn("failed to unmarshal node info",
				zap.String("key", keys[i]),
				zap.Error(unmarshalErr))
			continue
		}

		nodes = append(nodes, &info)
	}

	return nodes, nil
}

// ListOtherNodes returns information about all nodes except this one.
func (r *Registry) ListOtherNodes(ctx context.Context) ([]*Info, error) {
	nodes, err := r.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	otherNodes := make([]*Info, 0, len(nodes))
	for _, n := range nodes {
		if n.NodeID != r.nodeID {
			otherNodes = append(otherNodes, n)
		}
	}

	return otherNodes, nil
}

// IDFromKey extracts the node ID from a Redis key.
func IDFromKey(key string) string {
	return strings.TrimPrefix(key, keyPrefixNode)
}
