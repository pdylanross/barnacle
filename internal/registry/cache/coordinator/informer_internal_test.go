package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/test/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// makeNodeInfo creates a node.Info with the given parameters.
func makeNodeInfo(nodeID string, status node.Status, tierUsages []node.DiskUsageStats) *node.Info {
	return &node.Info{
		NodeID: nodeID,
		Status: status,
		Stats: node.Stats{
			TierDiskUsage: tierUsages,
		},
	}
}

// makeDiskUsage creates a DiskUsageStats with the given total and free bytes.
func makeDiskUsage(totalBytes, freeBytes uint64) node.DiskUsageStats {
	return node.DiskUsageStats{
		TotalBytes: totalBytes,
		FreeBytes:  freeBytes,
		UsedBytes:  totalBytes - freeBytes,
	}
}

// testSpaceReserver creates a SpaceReserver for testing with the default TTL.
func testSpaceReserver() *SpaceReserver {
	return NewSpaceReserver(0, zap.NewNop())
}

func TestCacheInformer_FindCacheLocation(t *testing.T) {
	logger := zap.NewNop()

	t.Run("local node has capacity in tier 0", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 500),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes(nil)

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		decision, reservation, err := ci.FindCacheLocation(context.Background(), 100)
		require.NoError(t, err)
		assert.True(t, decision.Local)
		assert.Equal(t, 0, decision.Tier)
		assert.NotNil(t, reservation)
		reservation.Release()
	})

	t.Run("local full remote has capacity in tier 0", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 50),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes([]*node.Info{
			makeNodeInfo("remote-1", node.StatusHealthy, []node.DiskUsageStats{
				makeDiskUsage(1000, 500),
			}),
		})

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		decision, reservation, err := ci.FindCacheLocation(context.Background(), 100)
		require.NoError(t, err)
		assert.False(t, decision.Local)
		assert.Equal(t, "remote-1", decision.NodeID)
		assert.Equal(t, 0, decision.Tier)
		assert.Nil(t, reservation)
	})

	t.Run("no capacity anywhere", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 10),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes([]*node.Info{
			makeNodeInfo("remote-1", node.StatusHealthy, []node.DiskUsageStats{
				makeDiskUsage(1000, 10),
			}),
		})

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		_, _, err := ci.FindCacheLocation(context.Background(), 100)
		assert.ErrorIs(t, err, ErrNoCapacity)
	})

	t.Run("tier 0 full everywhere tier 1 has local capacity", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 10),  // tier 0 full
			makeDiskUsage(5000, 500), // tier 1 has space
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes([]*node.Info{
			makeNodeInfo("remote-1", node.StatusHealthy, []node.DiskUsageStats{
				makeDiskUsage(1000, 10), // tier 0 full
				makeDiskUsage(5000, 10), // tier 1 also full
			}),
		})

		tiers := []TierCache{{Tier: 0, Cache: nil}, {Tier: 1, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		decision, reservation, err := ci.FindCacheLocation(context.Background(), 100)
		require.NoError(t, err)
		assert.True(t, decision.Local)
		assert.Equal(t, 1, decision.Tier)
		assert.NotNil(t, reservation)
		reservation.Release()
	})

	t.Run("tier 0 full everywhere tier 1 full locally remote has tier 1", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 10), // tier 0 full
			makeDiskUsage(5000, 10), // tier 1 full
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes([]*node.Info{
			makeNodeInfo("remote-1", node.StatusHealthy, []node.DiskUsageStats{
				makeDiskUsage(1000, 10),  // tier 0 full
				makeDiskUsage(5000, 500), // tier 1 has space
			}),
		})

		tiers := []TierCache{{Tier: 0, Cache: nil}, {Tier: 1, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		decision, reservation, err := ci.FindCacheLocation(context.Background(), 100)
		require.NoError(t, err)
		assert.False(t, decision.Local)
		assert.Equal(t, "remote-1", decision.NodeID)
		assert.Equal(t, 1, decision.Tier)
		assert.Nil(t, reservation)
	})

	t.Run("both local and remote have tier 0 capacity prefers local", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 500),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes([]*node.Info{
			makeNodeInfo("remote-1", node.StatusHealthy, []node.DiskUsageStats{
				makeDiskUsage(1000, 500),
			}),
		})

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		decision, reservation, err := ci.FindCacheLocation(context.Background(), 100)
		require.NoError(t, err)
		assert.True(t, decision.Local)
		assert.Equal(t, 0, decision.Tier)
		assert.NotNil(t, reservation)
		reservation.Release()
	})

	t.Run("skip unhealthy remote nodes", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 10),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes([]*node.Info{
			makeNodeInfo("remote-degraded", node.StatusDegraded, []node.DiskUsageStats{
				makeDiskUsage(1000, 500),
			}),
		})

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		_, _, err := ci.FindCacheLocation(context.Background(), 100)
		assert.ErrorIs(t, err, ErrNoCapacity)
	})

	t.Run("skip starting remote nodes", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 10),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes([]*node.Info{
			makeNodeInfo("remote-starting", node.StatusStarting, []node.DiskUsageStats{
				makeDiskUsage(1000, 500),
			}),
		})

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		_, _, err := ci.FindCacheLocation(context.Background(), 100)
		assert.ErrorIs(t, err, ErrNoCapacity)
	})

	t.Run("ListOtherNodes fails but local has capacity", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 500),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodesError(errors.New("redis connection failed"))

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		decision, reservation, err := ci.FindCacheLocation(context.Background(), 100)
		require.NoError(t, err)
		assert.True(t, decision.Local)
		assert.Equal(t, 0, decision.Tier)
		assert.NotNil(t, reservation)
		reservation.Release()
	})

	t.Run("ListOtherNodes fails and local is full", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 10),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodesError(errors.New("redis connection failed"))

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		_, _, err := ci.FindCacheLocation(context.Background(), 100)
		assert.ErrorIs(t, err, ErrNoCapacity)
	})

	t.Run("TotalBytes zero treated as no capacity", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			{TotalBytes: 0, FreeBytes: 500}, // stats collection failed
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes(nil)

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		_, _, err := ci.FindCacheLocation(context.Background(), 100)
		assert.ErrorIs(t, err, ErrNoCapacity)
	})

	t.Run("TierDiskUsage shorter than tiers list", func(t *testing.T) {
		// Only one tier of disk usage stats but two tiers configured
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 10), // tier 0 full
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes(nil)

		tiers := []TierCache{{Tier: 0, Cache: nil}, {Tier: 1, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		_, _, err := ci.FindCacheLocation(context.Background(), 100)
		assert.ErrorIs(t, err, ErrNoCapacity)
	})

	t.Run("context cancellation", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 500),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := ci.FindCacheLocation(ctx, 100)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("exact size fit succeeds", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 100), // exactly 100 free
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes(nil)

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		decision, reservation, err := ci.FindCacheLocation(context.Background(), 100)
		require.NoError(t, err)
		assert.True(t, decision.Local)
		assert.Equal(t, 0, decision.Tier)
		assert.NotNil(t, reservation)
		reservation.Release()
	})

	t.Run("blob one byte too large fails", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 99),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes(nil)

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		_, _, err := ci.FindCacheLocation(context.Background(), 100)
		assert.ErrorIs(t, err, ErrNoCapacity)
	})
}

func TestCacheInformer_Reservation(t *testing.T) {
	logger := zap.NewNop()

	t.Run("reservation reduces effective capacity", func(t *testing.T) {
		// 500 bytes free, place a 400-byte blob (succeeds), then a second 200-byte blob (fails)
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 500),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes(nil)

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		sr := NewSpaceReserver(5*time.Minute, zap.NewNop())
		ci := newCacheInformer(nr, tiers, logger, sr)

		// First placement succeeds
		decision, reservation, err := ci.FindCacheLocation(context.Background(), 400)
		require.NoError(t, err)
		assert.True(t, decision.Local)
		assert.NotNil(t, reservation)

		// Second placement fails (500 free - 400 claimed = 100 effective, 200 needed)
		_, _, err = ci.FindCacheLocation(context.Background(), 200)
		require.ErrorIs(t, err, ErrNoCapacity)

		reservation.Release()
	})

	t.Run("released reservation restores capacity", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 500),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes(nil)

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		sr := NewSpaceReserver(5*time.Minute, zap.NewNop())
		ci := newCacheInformer(nr, tiers, logger, sr)

		// Place and hold a reservation
		_, reservation, err := ci.FindCacheLocation(context.Background(), 400)
		require.NoError(t, err)

		// Should fail
		_, _, err = ci.FindCacheLocation(context.Background(), 200)
		require.ErrorIs(t, err, ErrNoCapacity)

		// Release the reservation
		reservation.Release()

		// Now should succeed
		decision, reservation2, err := ci.FindCacheLocation(context.Background(), 200)
		require.NoError(t, err)
		assert.True(t, decision.Local)
		assert.NotNil(t, reservation2)
		reservation2.Release()
	})

	t.Run("local decision returns non-nil reservation", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 500),
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes(nil)

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		decision, reservation, err := ci.FindCacheLocation(context.Background(), 100)
		require.NoError(t, err)
		assert.True(t, decision.Local)
		assert.NotNil(t, reservation)
		reservation.Release()
	})

	t.Run("remote decision returns nil reservation", func(t *testing.T) {
		localInfo := makeNodeInfo("local-node", node.StatusHealthy, []node.DiskUsageStats{
			makeDiskUsage(1000, 10), // local full
		})
		nr := mocks.NewNodeRegistry("local-node", localInfo)
		nr.SetOtherNodes([]*node.Info{
			makeNodeInfo("remote-1", node.StatusHealthy, []node.DiskUsageStats{
				makeDiskUsage(1000, 500),
			}),
		})

		tiers := []TierCache{{Tier: 0, Cache: nil}}
		ci := newCacheInformer(nr, tiers, logger, testSpaceReserver())

		decision, reservation, err := ci.FindCacheLocation(context.Background(), 100)
		require.NoError(t, err)
		assert.False(t, decision.Local)
		assert.Nil(t, reservation)
	})
}
