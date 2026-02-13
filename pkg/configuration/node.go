package configuration

import (
	"fmt"
	"time"
)

// Default node health configuration values.
const (
	DefaultNodeHealthSyncInterval = 1 * time.Second
)

// NodeHealthConfig contains settings for node health reporting.
type NodeHealthConfig struct {
	// SyncInterval is how often the node reports its health to Redis.
	// The Redis key TTL will be set to 2x this value.
	// Defaults to 1 second.
	SyncInterval time.Duration `koanf:"syncInterval"`

	// NodeID is the unique identifier for this node in the cluster.
	// If not specified, falls back to BARNACLE_NODE_ID environment variable,
	// then to the system hostname.
	NodeID string `koanf:"nodeId"`
}

// Validate checks that the node health configuration is valid.
func (n *NodeHealthConfig) Validate() error {
	if n.SyncInterval < 0 {
		return fmt.Errorf("%w: node sync interval cannot be negative", ErrInvalidConfiguration)
	}
	return nil
}

// GetSyncInterval returns the sync interval, using the default if not set.
func (n *NodeHealthConfig) GetSyncInterval() time.Duration {
	if n.SyncInterval == 0 {
		return DefaultNodeHealthSyncInterval
	}
	return n.SyncInterval
}

// ttlMultiplier is the factor applied to sync interval to calculate TTL.
const ttlMultiplier = 2

// GetTTL returns the Redis key TTL, which is 2x the sync interval.
func (n *NodeHealthConfig) GetTTL() time.Duration {
	return n.GetSyncInterval() * ttlMultiplier
}
