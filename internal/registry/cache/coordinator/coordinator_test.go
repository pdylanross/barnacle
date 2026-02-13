package coordinator_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/internal/registry/cache"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	"github.com/pdylanross/barnacle/test/mocks"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test constants for upstream and repo.
const (
	testUpstream = "docker.io"
	testRepo     = "library/nginx"
)

// testDigest creates a valid sha256 digest string for testing.
func testDigest(hex string) string {
	if len(hex) < 64 {
		hex += strings.Repeat("a", 64-len(hex))
	}
	return "sha256:" + hex[:64]
}

// testDescriptor creates a test descriptor.
func testDescriptor(digest string, size int64) *v1.Descriptor {
	h, _ := v1.NewHash(digest)
	return &v1.Descriptor{
		Digest:    h,
		Size:      size,
		MediaType: types.DockerLayer,
	}
}

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

// testNodeRegistry creates a mock NodeRegistry for testing.
func testNodeRegistry(nodeID string) *mocks.NodeRegistry {
	return mocks.NewNodeRegistry(nodeID, &node.Info{NodeID: nodeID})
}

func TestNewBlobCache(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	t.Run("creates cache with valid options", func(t *testing.T) {
		c, createErr := coordinator.NewBlobCache(&coordinator.Options{
			Redis:        redisClient,
			NodeID:       "test-node",
			NodeRegistry: testNodeRegistry("test-node"),
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: mocks.NewBlobCache()},
			},
		})
		require.NoError(t, createErr)
		assert.NotNil(t, c)
	})

	t.Run("fails without redis client", func(t *testing.T) {
		_, createErr := coordinator.NewBlobCache(&coordinator.Options{
			NodeID: "test-node",
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: mocks.NewBlobCache()},
			},
		})
		require.Error(t, createErr)
		assert.Contains(t, createErr.Error(), "redis client is required")
	})

	t.Run("fails without tiers", func(t *testing.T) {
		_, createErr := coordinator.NewBlobCache(&coordinator.Options{
			Redis:  redisClient,
			NodeID: "test-node",
			Tiers:  []coordinator.TierCache{},
		})
		require.Error(t, createErr)
		assert.Contains(t, createErr.Error(), "at least one cache tier is required")
	})

	t.Run("fails without node registry", func(t *testing.T) {
		_, createErr := coordinator.NewBlobCache(&coordinator.Options{
			Redis:  redisClient,
			NodeID: "test-node",
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: mocks.NewBlobCache()},
			},
		})
		require.Error(t, createErr)
		assert.Contains(t, createErr.Error(), "node registry is required")
	})

	t.Run("uses hostname as default node ID", func(t *testing.T) {
		c, createErr := coordinator.NewBlobCache(&coordinator.Options{
			Redis:        redisClient,
			NodeRegistry: testNodeRegistry("default"),
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: mocks.NewBlobCache()},
			},
		})
		require.NoError(t, createErr)
		assert.NotEmpty(t, c.NodeID())
	})
}

func TestCoordinatorBlobCache_PutAndGet(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	tier0 := mocks.NewBlobCache()
	c, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:        redisClient,
		NodeID:       "node-1",
		NodeRegistry: testNodeRegistry("node-1"),
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0},
		},
	})
	require.NoError(t, createErr)

	digest := testDigest("abc123")
	content := []byte("test blob content")
	desc := testDescriptor(digest, int64(len(content)))

	t.Run("stores blob in tier 0", func(t *testing.T) {
		putErr := c.Put(
			context.Background(),
			testUpstream,
			testRepo,
			digest,
			desc,
			bytes.NewReader(content),
			&coordinator.CacheLocationDecision{Local: true, Tier: 0},
		)
		require.NoError(t, putErr)

		// Verify stored in tier 0
		reader, getErr := tier0.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, getErr)
		data, _ := io.ReadAll(reader)
		assert.Equal(t, content, data)
	})

	t.Run("retrieves blob from cache", func(t *testing.T) {
		reader, getErr := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, getErr)
		data, _ := io.ReadAll(reader)
		assert.Equal(t, content, data)
	})

	t.Run("returns not found for missing blob", func(t *testing.T) {
		_, getErr := c.Get(context.Background(), testUpstream, testRepo, testDigest("nonexistent"))
		assert.ErrorIs(t, getErr, cache.ErrBlobNotFound)
	})
}

func TestCoordinatorBlobCache_Head(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	tier0 := mocks.NewBlobCache()
	c, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:        redisClient,
		NodeID:       "node-1",
		NodeRegistry: testNodeRegistry("node-1"),
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0},
		},
	})
	require.NoError(t, createErr)

	digest := testDigest("def456")
	content := []byte("head test content")
	desc := testDescriptor(digest, int64(len(content)))

	t.Run("returns not found for missing blob", func(t *testing.T) {
		_, headErr := c.Head(context.Background(), testUpstream, testRepo, testDigest("missing"))
		assert.ErrorIs(t, headErr, cache.ErrBlobNotFound)
	})

	t.Run("returns descriptor for existing blob", func(t *testing.T) {
		putErr := c.Put(
			context.Background(),
			testUpstream,
			testRepo,
			digest,
			desc,
			bytes.NewReader(content),
			&coordinator.CacheLocationDecision{Local: true, Tier: 0},
		)
		require.NoError(t, putErr)

		gotDesc, headErr := c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, headErr)
		assert.Equal(t, desc.Size, gotDesc.Size)
	})
}

func TestCoordinatorBlobCache_Redirect(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	// Create two coordinator caches with different node IDs
	tier0Node1 := mocks.NewBlobCache()
	node1, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:        redisClient,
		NodeID:       "node-1",
		NodeRegistry: testNodeRegistry("node-1"),
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0Node1},
		},
	})
	require.NoError(t, createErr)

	tier0Node2 := mocks.NewBlobCache()
	node2, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:        redisClient,
		NodeID:       "node-2",
		NodeRegistry: testNodeRegistry("node-2"),
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0Node2},
		},
	})
	require.NoError(t, createErr)

	digest := testDigest("redirect123")
	content := []byte("redirect test content")
	desc := testDescriptor(digest, int64(len(content)))

	// Store on node 1
	putErr := node1.Put(
		context.Background(),
		testUpstream,
		testRepo,
		digest,
		desc,
		bytes.NewReader(content),
		&coordinator.CacheLocationDecision{Local: true, Tier: 0},
	)
	require.NoError(t, putErr)

	expectedURL := httptk.NewBlobRedirectError("node-1", testUpstream, testRepo, digest).URL

	t.Run("Head returns redirect when blob on another node", func(t *testing.T) {
		_, headErr := node2.Head(context.Background(), testUpstream, testRepo, digest)
		require.Error(t, headErr)

		var redirectErr *httptk.RedirectError
		require.ErrorAs(t, headErr, &redirectErr, "expected RedirectError, got %T: %v", headErr, headErr)
		assert.Equal(t, expectedURL, redirectErr.URL)
	})

	t.Run("Get returns redirect when blob on another node", func(t *testing.T) {
		_, getErr := node2.Get(context.Background(), testUpstream, testRepo, digest)
		require.Error(t, getErr)

		var redirectErr *httptk.RedirectError
		require.ErrorAs(t, getErr, &redirectErr, "expected RedirectError, got %T: %v", getErr, getErr)
		assert.Equal(t, expectedURL, redirectErr.URL)
	})
}

func TestCoordinatorBlobCache_MultipleTiers(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	tier0 := mocks.NewBlobCache()
	tier1 := mocks.NewBlobCache()
	c, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:        redisClient,
		NodeID:       "node-1",
		NodeRegistry: testNodeRegistry("node-1"),
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0},
			{Tier: 1, Cache: tier1},
		},
	})
	require.NoError(t, createErr)

	digest := testDigest("tiered123")
	content := []byte("tiered content")
	desc := testDescriptor(digest, int64(len(content)))

	t.Run("Put with tier 0 decision stores in tier 0", func(t *testing.T) {
		putErr := c.Put(
			context.Background(),
			testUpstream,
			testRepo,
			digest,
			desc,
			bytes.NewReader(content),
			&coordinator.CacheLocationDecision{Local: true, Tier: 0},
		)
		require.NoError(t, putErr)

		// Should be in tier 0
		_, headErr := tier0.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, headErr)

		// Should not be in tier 1
		_, headErr = tier1.Head(context.Background(), testUpstream, testRepo, digest)
		assert.ErrorIs(t, headErr, cache.ErrBlobNotFound)
	})

	t.Run("Put with tier 1 decision stores in tier 1", func(t *testing.T) {
		tier1Digest := testDigest("tiered456")
		tier1Content := []byte("tier 1 content")
		tier1Desc := testDescriptor(tier1Digest, int64(len(tier1Content)))

		putErr := c.Put(
			context.Background(),
			testUpstream,
			testRepo,
			tier1Digest,
			tier1Desc,
			bytes.NewReader(tier1Content),
			&coordinator.CacheLocationDecision{Local: true, Tier: 1},
		)
		require.NoError(t, putErr)

		// Should not be in tier 0
		_, headErr := tier0.Head(context.Background(), testUpstream, testRepo, tier1Digest)
		require.ErrorIs(t, headErr, cache.ErrBlobNotFound)

		// Should be in tier 1
		_, headErr = tier1.Head(context.Background(), testUpstream, testRepo, tier1Digest)
		require.NoError(t, headErr)

		// Verify location in Redis shows tier 1
		loc, locErr := c.GetLocation(context.Background(), tier1Digest)
		require.NoError(t, locErr)
		assert.Equal(t, 1, loc.Tier)
	})
}

func TestCoordinatorBlobCache_Delete(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	tier0 := mocks.NewBlobCache()
	c, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:        redisClient,
		NodeID:       "node-1",
		NodeRegistry: testNodeRegistry("node-1"),
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0},
		},
	})
	require.NoError(t, createErr)

	digest := testDigest("delete123")
	content := []byte("delete test content")
	desc := testDescriptor(digest, int64(len(content)))

	// Put and then delete
	putErr := c.Put(
		context.Background(),
		testUpstream,
		testRepo,
		digest,
		desc,
		bytes.NewReader(content),
		&coordinator.CacheLocationDecision{Local: true, Tier: 0},
	)
	require.NoError(t, putErr)

	deleteErr := c.Delete(context.Background(), testUpstream, testRepo, digest)
	require.NoError(t, deleteErr)

	// Should no longer be found
	_, headErr := c.Head(context.Background(), testUpstream, testRepo, digest)
	assert.ErrorIs(t, headErr, cache.ErrBlobNotFound)
}

func TestCoordinatorBlobCache_AccessCounting(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	tier0 := mocks.NewBlobCache()
	c, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:                redisClient,
		NodeID:               "node-1",
		NodeRegistry:         testNodeRegistry("node-1"),
		AccessWindowDuration: time.Hour,
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0},
		},
	})
	require.NoError(t, createErr)

	digest := testDigest("access123")
	content := []byte("access test content")
	desc := testDescriptor(digest, int64(len(content)))

	putErr := c.Put(
		context.Background(),
		testUpstream,
		testRepo,
		digest,
		desc,
		bytes.NewReader(content),
		&coordinator.CacheLocationDecision{Local: true, Tier: 0},
	)
	require.NoError(t, putErr)

	t.Run("records access on Get", func(t *testing.T) {
		// Get the blob multiple times
		for range 5 {
			reader, getErr := c.Get(context.Background(), testUpstream, testRepo, digest)
			require.NoError(t, getErr)
			_ = reader.Close()
		}

		// Check access count
		count, accessErr := c.GetAccessCount(context.Background(), digest)
		require.NoError(t, accessErr)
		assert.Equal(t, int64(5), count)
	})
}

func TestCoordinatorBlobCache_DiscoveryFromDisk(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	tier0 := mocks.NewBlobCache()

	// Pre-populate tier0 directly (simulating data on disk before Redis knew about it)
	digest := testDigest("discovery123")
	content := []byte("discovery test content")
	desc := testDescriptor(digest, int64(len(content)))
	_ = tier0.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))

	// Create coordinator - it doesn't know about this blob in Redis yet
	c, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:        redisClient,
		NodeID:       "node-1",
		NodeRegistry: testNodeRegistry("node-1"),
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0},
		},
	})
	require.NoError(t, createErr)

	t.Run("discovers blob from local tier and registers in Redis", func(t *testing.T) {
		// First access should find it locally and register it
		gotDesc, headErr := c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, headErr)
		assert.Equal(t, desc.Size, gotDesc.Size)

		// Verify it's now registered in Redis by checking that a second coordinator
		// on a different node would get a redirect
		node2, node2Err := coordinator.NewBlobCache(&coordinator.Options{
			Redis:        redisClient,
			NodeID:       "node-2",
			NodeRegistry: testNodeRegistry("node-2"),
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: mocks.NewBlobCache()},
			},
		})
		require.NoError(t, node2Err)

		_, node2HeadErr := node2.Head(context.Background(), testUpstream, testRepo, digest)
		var redirectErr *httptk.RedirectError
		require.ErrorAs(
			t,
			node2HeadErr,
			&redirectErr,
			"expected redirect after discovery, got %T: %v",
			node2HeadErr,
			node2HeadErr,
		)
		assert.Contains(t, redirectErr.URL, "node-1")
	})
}

func TestCoordinatorBlobCache_FindCacheLocation(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	tier0 := mocks.NewBlobCache()
	c, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:        redisClient,
		NodeID:       "node-1",
		NodeRegistry: testNodeRegistry("node-1"),
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0},
		},
	})
	require.NoError(t, createErr)

	t.Run("FindCacheLocation accessible via Cache interface", func(t *testing.T) {
		// Local node info has no TierDiskUsage stats, so this should return ErrNoCapacity
		decision, reservation, findErr := c.FindCacheLocation(context.Background(), 100)
		require.ErrorIs(t, findErr, coordinator.ErrNoCapacity)
		assert.Nil(t, decision)
		assert.Nil(t, reservation)
	})
}

func TestCoordinatorBlobCache_ListLocal(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	tier0 := mocks.NewBlobCache()
	tier1 := mocks.NewBlobCache()
	c, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:        redisClient,
		NodeID:       "node-1",
		NodeRegistry: testNodeRegistry("node-1"),
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0},
			{Tier: 1, Cache: tier1},
		},
	})
	require.NoError(t, createErr)

	digest0 := testDigest("list0000")
	digest1 := testDigest("list1111")
	content := []byte("list test content")
	desc0 := testDescriptor(digest0, int64(len(content)))
	desc1 := testDescriptor(digest1, int64(len(content)))

	// Store in different tiers
	putErr := c.Put(context.Background(), testUpstream, testRepo, digest0, desc0,
		bytes.NewReader(content), &coordinator.CacheLocationDecision{Local: true, Tier: 0})
	require.NoError(t, putErr)

	putErr = c.Put(context.Background(), testUpstream, testRepo, digest1, desc1,
		bytes.NewReader(content), &coordinator.CacheLocationDecision{Local: true, Tier: 1})
	require.NoError(t, putErr)

	t.Run("lists blobs from all tiers", func(t *testing.T) {
		blobs, listErr := c.ListLocal(context.Background())
		require.NoError(t, listErr)
		assert.Len(t, blobs, 2)

		foundDigests := make(map[string]int)
		for _, b := range blobs {
			foundDigests[b.Digest] = b.Tier
		}

		assert.Equal(t, 0, foundDigests[digest0])
		assert.Equal(t, 1, foundDigests[digest1])
	})

	t.Run("returns empty list when no blobs", func(t *testing.T) {
		emptyTier := mocks.NewBlobCache()
		emptyCache, emptyErr := coordinator.NewBlobCache(&coordinator.Options{
			Redis:        redisClient,
			NodeID:       "node-empty",
			NodeRegistry: testNodeRegistry("node-empty"),
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: emptyTier},
			},
		})
		require.NoError(t, emptyErr)

		blobs, listErr := emptyCache.ListLocal(context.Background())
		require.NoError(t, listErr)
		assert.Empty(t, blobs)
	})

	t.Run("returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, listErr := c.ListLocal(ctx)
		assert.ErrorIs(t, listErr, context.Canceled)
	})
}

func TestCoordinatorBlobCache_GetAccessCountWindow(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	tier0 := mocks.NewBlobCache()
	c, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:                redisClient,
		NodeID:               "node-1",
		NodeRegistry:         testNodeRegistry("node-1"),
		AccessWindowDuration: time.Hour,
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0},
		},
	})
	require.NoError(t, createErr)

	digest := testDigest("accesswindow123")
	content := []byte("access window test")
	desc := testDescriptor(digest, int64(len(content)))

	putErr := c.Put(context.Background(), testUpstream, testRepo, digest, desc,
		bytes.NewReader(content), &coordinator.CacheLocationDecision{Local: true, Tier: 0})
	require.NoError(t, putErr)

	// Generate accesses
	for range 3 {
		reader, getErr := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, getErr)
		_ = reader.Close()
	}

	t.Run("counts accesses within window", func(t *testing.T) {
		count, err := c.GetAccessCountWindow(context.Background(), digest, 5*time.Minute)
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})

	t.Run("returns zero for unknown digest", func(t *testing.T) {
		count, err := c.GetAccessCountWindow(context.Background(), testDigest("unknown999"), 5*time.Minute)
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	t.Run("returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := c.GetAccessCountWindow(ctx, digest, 5*time.Minute)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestCoordinatorBlobCache_DeleteLocalOnly(t *testing.T) {
	t.Run("deletes blob and cleans Redis when registered to this node", func(t *testing.T) {
		redisClient, cleanup := setupTestRedis(t)
		defer cleanup()

		tier0 := mocks.NewBlobCache()
		c, createErr := coordinator.NewBlobCache(&coordinator.Options{
			Redis:        redisClient,
			NodeID:       "node-1",
			NodeRegistry: testNodeRegistry("node-1"),
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: tier0},
			},
		})
		require.NoError(t, createErr)

		digest := testDigest("dellocal111")
		content := []byte("delete local test")
		desc := testDescriptor(digest, int64(len(content)))

		putErr := c.Put(context.Background(), testUpstream, testRepo, digest, desc,
			bytes.NewReader(content), &coordinator.CacheLocationDecision{Local: true, Tier: 0})
		require.NoError(t, putErr)

		// Verify blob exists
		_, headErr := c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, headErr)

		// Delete locally
		deleteErr := c.DeleteLocalOnly(context.Background(), digest)
		require.NoError(t, deleteErr)

		// Blob should be gone from local cache
		_, headErr = tier0.Head(context.Background(), "", "", digest)
		require.ErrorIs(t, headErr, cache.ErrBlobNotFound)

		// Location should be gone from Redis
		_, locErr := c.GetLocation(context.Background(), digest)
		require.ErrorIs(t, locErr, coordinator.ErrLocationNotFound)
	})

	t.Run("deletes blob locally but keeps Redis when registered to another node", func(t *testing.T) {
		redisClient, cleanup := setupTestRedis(t)
		defer cleanup()

		tier0Node1 := mocks.NewBlobCache()
		node1, createErr := coordinator.NewBlobCache(&coordinator.Options{
			Redis:        redisClient,
			NodeID:       "node-1",
			NodeRegistry: testNodeRegistry("node-1"),
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: tier0Node1},
			},
		})
		require.NoError(t, createErr)

		tier0Node2 := mocks.NewBlobCache()
		node2, createErr := coordinator.NewBlobCache(&coordinator.Options{
			Redis:        redisClient,
			NodeID:       "node-2",
			NodeRegistry: testNodeRegistry("node-2"),
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: tier0Node2},
			},
		})
		require.NoError(t, createErr)

		digest := testDigest("dellocal222")
		content := []byte("cross node test")
		desc := testDescriptor(digest, int64(len(content)))

		// Store blob on node-1 (registers in Redis as node-1)
		putErr := node1.Put(context.Background(), testUpstream, testRepo, digest, desc,
			bytes.NewReader(content), &coordinator.CacheLocationDecision{Local: true, Tier: 0})
		require.NoError(t, putErr)

		// Also put it directly in node-2's tier cache (simulating a copy)
		_ = tier0Node2.Put(context.Background(), "", "", digest, desc, bytes.NewReader(content))

		// DeleteLocalOnly on node-2 should NOT remove Redis location (owned by node-1)
		deleteErr := node2.DeleteLocalOnly(context.Background(), digest)
		require.NoError(t, deleteErr)

		// node-2's local cache should be empty
		_, headErr := tier0Node2.Head(context.Background(), "", "", digest)
		require.ErrorIs(t, headErr, cache.ErrBlobNotFound)

		// Redis location should still point to node-1
		loc, locErr := node1.GetLocation(context.Background(), digest)
		require.NoError(t, locErr)
		assert.Equal(t, "node-1", loc.NodeID)
	})

	t.Run("succeeds for blob not in Redis", func(t *testing.T) {
		redisClient, cleanup := setupTestRedis(t)
		defer cleanup()

		tier0 := mocks.NewBlobCache()
		c, createErr := coordinator.NewBlobCache(&coordinator.Options{
			Redis:        redisClient,
			NodeID:       "node-1",
			NodeRegistry: testNodeRegistry("node-1"),
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: tier0},
			},
		})
		require.NoError(t, createErr)

		deleteErr := c.DeleteLocalOnly(context.Background(), testDigest("notinredis"))
		require.NoError(t, deleteErr)
	})

	t.Run("returns error with cancelled context", func(t *testing.T) {
		redisClient, cleanup := setupTestRedis(t)
		defer cleanup()

		tier0 := mocks.NewBlobCache()
		c, createErr := coordinator.NewBlobCache(&coordinator.Options{
			Redis:        redisClient,
			NodeID:       "node-1",
			NodeRegistry: testNodeRegistry("node-1"),
			Tiers: []coordinator.TierCache{
				{Tier: 0, Cache: tier0},
			},
		})
		require.NoError(t, createErr)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		deleteErr := c.DeleteLocalOnly(ctx, testDigest("cancelled"))
		assert.ErrorIs(t, deleteErr, context.Canceled)
	})
}

func TestCoordinatorBlobCache_ContextCancellation(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	tier0 := mocks.NewBlobCache()

	c, createErr := coordinator.NewBlobCache(&coordinator.Options{
		Redis:        redisClient,
		NodeID:       "node-1",
		NodeRegistry: testNodeRegistry("node-1"),
		Tiers: []coordinator.TierCache{
			{Tier: 0, Cache: tier0},
		},
	})
	require.NoError(t, createErr)

	digest := testDigest("context123")
	content := []byte("context test content")
	desc := testDescriptor(digest, int64(len(content)))

	// First store a blob so we can test Get and Head with cancelled context
	putErr := c.Put(
		context.Background(),
		testUpstream,
		testRepo,
		digest,
		desc,
		bytes.NewReader(content),
		&coordinator.CacheLocationDecision{Local: true, Tier: 0},
	)
	require.NoError(t, putErr)

	t.Run("Head returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, headErr := c.Head(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, headErr, context.Canceled)
	})

	t.Run("Get returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, getErr := c.Get(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, getErr, context.Canceled)
	})

	t.Run("Put returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		newDigest := testDigest("ctxcancel456")
		cancelledPutErr := c.Put(
			ctx,
			testUpstream,
			testRepo,
			newDigest,
			desc,
			bytes.NewReader(content),
			&coordinator.CacheLocationDecision{Local: true, Tier: 0},
		)
		assert.ErrorIs(t, cancelledPutErr, context.Canceled)
	})

	t.Run("Delete returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		deleteErr := c.Delete(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, deleteErr, context.Canceled)
	})

	t.Run("GetAccessCount returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, accessErr := c.GetAccessCount(ctx, digest)
		assert.ErrorIs(t, accessErr, context.Canceled)
	})

	t.Run("GetLocation returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, locErr := c.GetLocation(ctx, digest)
		assert.ErrorIs(t, locErr, context.Canceled)
	})

	t.Run("SetLocation returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		setErr := c.SetLocation(ctx, digest, "node-1", 0)
		assert.ErrorIs(t, setErr, context.Canceled)
	})

	t.Run("Head returns error with deadline exceeded context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()

		_, headErr := c.Head(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, headErr, context.DeadlineExceeded)
	})

	t.Run("Get returns error with deadline exceeded context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()

		_, getErr := c.Get(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, getErr, context.DeadlineExceeded)
	})

	t.Run("Put returns error with deadline exceeded context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()

		newDigest := testDigest("ctxdeadline789")
		deadlinePutErr := c.Put(
			ctx,
			testUpstream,
			testRepo,
			newDigest,
			desc,
			bytes.NewReader(content),
			&coordinator.CacheLocationDecision{Local: true, Tier: 0},
		)
		assert.ErrorIs(t, deadlinePutErr, context.DeadlineExceeded)
	})

	t.Run("Delete returns error with deadline exceeded context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()

		deleteErr := c.Delete(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, deleteErr, context.DeadlineExceeded)
	})
}
