package node_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// setupTestRedis creates a miniredis instance and returns a client and cleanup function.
func setupTestRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return client, func() {
		client.Close()
		mr.Close()
	}
}

func testConfig(nodeID string) *configuration.NodeHealthConfig {
	return &configuration.NodeHealthConfig{
		SyncInterval: 100 * time.Millisecond,
		NodeID:       nodeID,
	}
}

func testCacheConfig(t *testing.T) *configuration.CacheConfiguration {
	t.Helper()
	return &configuration.CacheConfiguration{
		Disk: configuration.DiskCacheConfiguration{
			Tiers: []configuration.DiskTierConfiguration{
				{Tier: 0, Path: t.TempDir()},
			},
		},
	}
}

func TestNewRegistry(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	t.Run("creates registry with explicit node ID", func(t *testing.T) {
		config := testConfig("test-node-1")
		cacheConfig := testCacheConfig(t)
		logger := zap.NewNop()

		reg, err := node.NewRegistry(config, cacheConfig, logger, redisClient)
		require.NoError(t, err)
		assert.NotNil(t, reg)
		assert.Equal(t, "test-node-1", reg.NodeID())
	})

	t.Run("creates registry with hostname fallback", func(t *testing.T) {
		config := testConfig("")
		cacheConfig := testCacheConfig(t)
		logger := zap.NewNop()

		reg, err := node.NewRegistry(config, cacheConfig, logger, redisClient)
		require.NoError(t, err)
		assert.NotNil(t, reg)
		assert.NotEmpty(t, reg.NodeID())
	})
}

func TestRegistry_GetNodeInfo(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	config := testConfig("test-node")
	cacheConfig := testCacheConfig(t)
	logger := zap.NewNop()

	reg, err := node.NewRegistry(config, cacheConfig, logger, redisClient)
	require.NoError(t, err)

	info := reg.GetNodeInfo()

	assert.Equal(t, "test-node", info.NodeID)
	assert.Equal(t, node.StatusStarting, info.Status)
	assert.WithinDuration(t, time.Now(), info.LastUpdated, time.Second)
	// Verify disk stats are collected
	assert.Len(t, info.Stats.TierDiskUsage, 1)
	assert.NotZero(t, info.Stats.TierDiskUsage[0].TotalBytes)
}

func TestRegistry_SetStatus(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	config := testConfig("test-node")
	cacheConfig := testCacheConfig(t)
	logger := zap.NewNop()

	reg, err := node.NewRegistry(config, cacheConfig, logger, redisClient)
	require.NoError(t, err)

	// Initially starting
	info := reg.GetNodeInfo()
	assert.Equal(t, node.StatusStarting, info.Status)

	// Set to healthy
	reg.SetStatus(node.StatusHealthy)
	info = reg.GetNodeInfo()
	assert.Equal(t, node.StatusHealthy, info.Status)

	// Set to degraded
	reg.SetStatus(node.StatusDegraded)
	info = reg.GetNodeInfo()
	assert.Equal(t, node.StatusDegraded, info.Status)
}

func TestRegistry_SyncAndGet(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	config := testConfig("node-1")
	cacheConfig := testCacheConfig(t)
	logger := zap.NewNop()

	reg1, err := node.NewRegistry(config, cacheConfig, logger, redisClient)
	require.NoError(t, err)

	// Create a second registry to fetch the first node's info
	config2 := testConfig("node-2")
	cacheConfig2 := testCacheConfig(t)
	reg2, err := node.NewRegistry(config2, cacheConfig2, logger, redisClient)
	require.NoError(t, err)

	// Initially, node-1 is not in Redis
	_, err = reg2.GetNode(context.Background(), "node-1")
	require.ErrorIs(t, err, node.ErrNodeNotFound)

	// Run the sync task once using MakeTask
	task := reg1.MakeTask()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run task briefly - it will sync immediately then wait for interval
	go func() {
		_ = task.Run(ctx, zap.NewNop())
	}()

	// Wait for sync to complete
	time.Sleep(20 * time.Millisecond)

	// Now node-1 should be in Redis
	info, err := reg2.GetNode(context.Background(), "node-1")
	require.NoError(t, err)
	assert.Equal(t, "node-1", info.NodeID)
	assert.Equal(t, node.StatusHealthy, info.Status) // Task sets to healthy after first sync
}

func TestRegistry_ListNodes(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	logger := zap.NewNop()

	// Create multiple registries
	reg1, err := node.NewRegistry(testConfig("node-1"), testCacheConfig(t), logger, redisClient)
	require.NoError(t, err)
	reg2, err := node.NewRegistry(testConfig("node-2"), testCacheConfig(t), logger, redisClient)
	require.NoError(t, err)
	reg3, err := node.NewRegistry(testConfig("node-3"), testCacheConfig(t), logger, redisClient)
	require.NoError(t, err)

	// Run sync tasks briefly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() { _ = reg1.MakeTask().Run(ctx, zap.NewNop()) }()
	go func() { _ = reg2.MakeTask().Run(ctx, zap.NewNop()) }()
	go func() { _ = reg3.MakeTask().Run(ctx, zap.NewNop()) }()

	// Wait for syncs
	time.Sleep(30 * time.Millisecond)

	// List all nodes
	nodes, err := reg1.ListNodes(context.Background())
	require.NoError(t, err)
	assert.Len(t, nodes, 3)

	// Verify all nodes are present
	nodeIDs := make(map[string]bool)
	for _, n := range nodes {
		nodeIDs[n.NodeID] = true
	}
	assert.True(t, nodeIDs["node-1"])
	assert.True(t, nodeIDs["node-2"])
	assert.True(t, nodeIDs["node-3"])
}

func TestRegistry_ListOtherNodes(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	logger := zap.NewNop()

	reg1, err := node.NewRegistry(testConfig("node-1"), testCacheConfig(t), logger, redisClient)
	require.NoError(t, err)
	reg2, err := node.NewRegistry(testConfig("node-2"), testCacheConfig(t), logger, redisClient)
	require.NoError(t, err)

	// Run sync tasks briefly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() { _ = reg1.MakeTask().Run(ctx, zap.NewNop()) }()
	go func() { _ = reg2.MakeTask().Run(ctx, zap.NewNop()) }()

	time.Sleep(30 * time.Millisecond)

	// List other nodes from reg1's perspective
	others, err := reg1.ListOtherNodes(context.Background())
	require.NoError(t, err)
	assert.Len(t, others, 1)
	assert.Equal(t, "node-2", others[0].NodeID)

	// List other nodes from reg2's perspective
	others, err = reg2.ListOtherNodes(context.Background())
	require.NoError(t, err)
	assert.Len(t, others, 1)
	assert.Equal(t, "node-1", others[0].NodeID)
}

func TestRegistry_ContextCancellation(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	config := testConfig("test-node")
	cacheConfig := testCacheConfig(t)
	logger := zap.NewNop()

	reg, createErr := node.NewRegistry(config, cacheConfig, logger, redisClient)
	require.NoError(t, createErr)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("GetNode returns error with cancelled context", func(t *testing.T) {
		_, err := reg.GetNode(ctx, "any-node")
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("ListNodes returns error with cancelled context", func(t *testing.T) {
		_, err := reg.ListNodes(ctx)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("ListOtherNodes returns error with cancelled context", func(t *testing.T) {
		_, err := reg.ListOtherNodes(ctx)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestNodeHealthConfig(t *testing.T) {
	t.Run("GetSyncInterval returns default when not set", func(t *testing.T) {
		config := &configuration.NodeHealthConfig{}
		assert.Equal(t, configuration.DefaultNodeHealthSyncInterval, config.GetSyncInterval())
	})

	t.Run("GetSyncInterval returns configured value", func(t *testing.T) {
		config := &configuration.NodeHealthConfig{
			SyncInterval: 30 * time.Second,
		}
		assert.Equal(t, 30*time.Second, config.GetSyncInterval())
	})

	t.Run("GetTTL returns 2x sync interval", func(t *testing.T) {
		config := &configuration.NodeHealthConfig{
			SyncInterval: 15 * time.Second,
		}
		assert.Equal(t, 30*time.Second, config.GetTTL())
	})

	t.Run("Validate rejects negative sync interval", func(t *testing.T) {
		config := &configuration.NodeHealthConfig{
			SyncInterval: -1 * time.Second,
		}
		err := config.Validate()
		require.Error(t, err)
		require.ErrorIs(t, err, configuration.ErrInvalidConfiguration)
	})

	t.Run("Validate accepts zero sync interval", func(t *testing.T) {
		config := &configuration.NodeHealthConfig{}
		err := config.Validate()
		assert.NoError(t, err)
	})
}

func TestIDFromKey(t *testing.T) {
	assert.Equal(t, "my-node", node.IDFromKey("barnacle:node:my-node"))
	assert.Empty(t, node.IDFromKey("barnacle:node:"))
}
