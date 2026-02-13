// Package coordinator provides a distributed blob cache coordinator that uses Redis
// to track blob locations across multiple nodes and cache tiers.
//
// The coordinator manages a tiered cache system where:
//   - New blobs are inserted into tier 0
//   - Redis tracks which node and tier each blob lives in
//   - Requests for blobs on other nodes return redirect information
//   - Access counts are tracked using Redis sorted sets for future promotion/demotion
package coordinator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pdylanross/barnacle/internal/registry/cache"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Redis key prefixes for blob metadata.
const (
	// keyPrefixLocation is the prefix for blob location hash keys.
	// Format: barnacle:blob:loc:{digest} with fields: node (string), tier (int).
	keyPrefixLocation = "barnacle:blob:loc:"

	// keyPrefixAccess is the prefix for blob access count sorted sets.
	// Format: barnacle:blob:access:{digest} with members and scores as timestamps.
	keyPrefixAccess = "barnacle:blob:access:"

	// locationFieldNode is the hash field name for the node ID.
	locationFieldNode = "node"

	// locationFieldTier is the hash field name for the cache tier.
	locationFieldTier = "tier"

	// defaultAccessWindowDuration is the default sliding window for access counting.
	defaultAccessWindowDuration = 24 * time.Hour
)

// TierCache represents a cache tier with its backing storage.
type TierCache struct {
	// Tier is the tier number (0 is the insertion tier, higher numbers are for less frequently accessed items).
	Tier int
	// Cache is the blob cache implementation for this tier.
	Cache cache.BlobCache
}

// InFlightTracker is an interface for tracking in-flight blob requests.
// This is used by the rebalancing system to know when it's safe to delete a blob.
type InFlightTracker interface {
	// Increment increments the in-flight counter for a blob and returns a release function.
	Increment(ctx context.Context, digest string) (func(), error)
}

// Options configures the coordinator blob cache.
type Options struct {
	// Redis is the Redis client used for coordination.
	Redis *redis.Client

	// NodeID identifies this node in the cluster.
	// Defaults to the hostname if not specified.
	// Can be set via BARNACLE_NODE_ID environment variable.
	NodeID string

	// Tiers is the list of cache tiers, ordered from tier 0 (insertion tier) upward.
	// At least one tier must be provided.
	Tiers []TierCache

	// Logger is the logger for debug and error logging.
	Logger *zap.Logger

	// AccessWindowDuration is the duration of the sliding window for access counting.
	// Defaults to 24 hours if not specified.
	AccessWindowDuration time.Duration

	// NodeRegistry provides access to node information for cache placement decisions.
	NodeRegistry NodeRegistryProvider

	// InFlightTracker tracks in-flight blob requests for rebalancing.
	// If nil, in-flight tracking is disabled.
	InFlightTracker InFlightTracker
}

// Cache provides distributed blob cache management with coordination via Redis.
type Cache interface {
	// Head returns the descriptor for a cached blob without reading its content.
	Head(ctx context.Context, upstream, repo, digest string) (*v1.Descriptor, error)

	// Get retrieves a cached blob by digest.
	Get(ctx context.Context, upstream, repo, digest string) (io.ReadCloser, error)

	// Put stores a blob in the tier specified by the decision and registers its location in Redis.
	Put(
		ctx context.Context,
		upstream, repo, digest string,
		descriptor *v1.Descriptor,
		content io.Reader,
		decision *CacheLocationDecision,
	) error

	// Delete removes a blob from all local tiers and Redis.
	Delete(ctx context.Context, upstream, repo, digest string) error

	// GetAccessCount returns the number of accesses for a blob within the sliding window.
	// This is useful for promotion/demotion logic.
	GetAccessCount(ctx context.Context, digest string) (int64, error)

	// GetLocation returns the current location of a blob.
	// This is useful for promotion/demotion logic.
	GetLocation(ctx context.Context, digest string) (*BlobLocation, error)

	// SetLocation updates the location of a blob in Redis.
	// This is useful for promotion/demotion logic when moving blobs between tiers.
	SetLocation(ctx context.Context, digest string, nodeID string, tier int) error

	// NodeID returns the node ID of this coordinator.
	NodeID() string

	// FindCacheLocation determines where a blob of the given size should be cached.
	// It examines disk capacity across all nodes and tiers, preferring the local node
	// and lower-numbered tiers. Returns ErrNoCapacity if no node has space.
	//
	// When the decision is local, a non-nil Reservation is returned that the caller
	// must Release after the blob write completes (success or failure).
	// When the decision is remote, the returned Reservation is nil.
	FindCacheLocation(ctx context.Context, blobSize int64) (*CacheLocationDecision, *Reservation, error)

	// ListLocal returns information about all blobs cached locally across all tiers,
	// annotated with their tier number.
	ListLocal(ctx context.Context) ([]LocalBlobInfo, error)

	// GetAccessCountWindow returns the number of accesses for a blob within the given duration window.
	GetAccessCountWindow(ctx context.Context, digest string, window time.Duration) (int64, error)

	// DeleteLocalOnly removes a blob from all local disk tiers and conditionally cleans up Redis.
	// Redis location and access history are only deleted if Redis indicates the blob is on this node.
	DeleteLocalOnly(ctx context.Context, digest string) error
}

// coordinatorBlobCache implements CoordinatorCache with distributed coordination.
type coordinatorBlobCache struct {
	redis                *redis.Client
	nodeID               string
	tiers                []TierCache
	logger               *zap.Logger
	accessWindowDuration time.Duration
	informer             *cacheInformer
	inFlightTracker      InFlightTracker
}

// NewBlobCache creates a new coordinator blob cache.
//
// The coordinator uses Redis to track blob locations across nodes and cache tiers.
// New blobs are inserted into tier 0. When a blob is requested that exists on
// another node, an [httptk.RedirectError] is returned with the redirect URL.
//
// Access counts are tracked using Redis sorted sets with timestamps, creating
// a sliding window count that can be used for promotion/demotion decisions.
func NewBlobCache(opts *Options) (Cache, error) {
	if opts.Redis == nil {
		return nil, errors.New("redis client is required")
	}

	if len(opts.Tiers) == 0 {
		return nil, errors.New("at least one cache tier is required")
	}

	if opts.NodeRegistry == nil {
		return nil, errors.New("node registry is required")
	}

	nodeID := opts.NodeID
	if nodeID == "" {
		// Check environment variable
		nodeID = os.Getenv("BARNACLE_NODE_ID")
	}
	if nodeID == "" {
		// Fall back to hostname
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("failed to get hostname for node ID: %w", err)
		}
		nodeID = hostname
	}

	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	logger = logger.Named("coordinatorBlobCache")

	accessWindow := opts.AccessWindowDuration
	if accessWindow == 0 {
		accessWindow = defaultAccessWindowDuration
	}

	spaceReserver := NewSpaceReserver(0, logger)

	c := &coordinatorBlobCache{
		redis:                opts.Redis,
		nodeID:               nodeID,
		tiers:                opts.Tiers,
		logger:               logger,
		accessWindowDuration: accessWindow,
		informer:             newCacheInformer(opts.NodeRegistry, opts.Tiers, logger, spaceReserver),
		inFlightTracker:      opts.InFlightTracker,
	}

	logger.Info("coordinator blob cache initialized",
		zap.String("nodeID", nodeID),
		zap.Int("tierCount", len(opts.Tiers)),
		zap.Duration("accessWindow", accessWindow))

	return c, nil
}

// locationKey returns the Redis key for a blob's location.
func locationKey(digest string) string {
	return keyPrefixLocation + digest
}

// accessKey returns the Redis key for a blob's access counts.
func accessKey(digest string) string {
	return keyPrefixAccess + digest
}

// BlobLocation represents where a blob is stored.
type BlobLocation struct {
	NodeID string
	Tier   int
}

// ErrLocationNotFound is returned when a blob's location is not tracked in Redis.
var ErrLocationNotFound = errors.New("blob location not found in Redis")

// getLocation retrieves the location of a blob from Redis.
// Returns ErrLocationNotFound if the blob location is not tracked.
func (c *coordinatorBlobCache) getLocation(ctx context.Context, digest string) (*BlobLocation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := locationKey(digest)

	result, err := c.redis.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get blob location: %w", err)
	}

	if len(result) == 0 {
		return nil, ErrLocationNotFound
	}

	nodeID, ok := result[locationFieldNode]
	if !ok {
		return nil, ErrLocationNotFound
	}

	tierStr, ok := result[locationFieldTier]
	if !ok {
		return nil, ErrLocationNotFound
	}

	tier, err := strconv.Atoi(tierStr)
	if err != nil {
		c.logger.Warn("invalid tier value in Redis",
			zap.String("digest", digest),
			zap.String("tierStr", tierStr))
		return nil, ErrLocationNotFound
	}

	return &BlobLocation{
		NodeID: nodeID,
		Tier:   tier,
	}, nil
}

// setLocation stores the location of a blob in Redis.
func (c *coordinatorBlobCache) setLocation(ctx context.Context, digest string, nodeID string, tier int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := locationKey(digest)

	err := c.redis.HSet(ctx, key, map[string]any{
		locationFieldNode: nodeID,
		locationFieldTier: tier,
	}).Err()
	if err != nil {
		return fmt.Errorf("failed to set blob location: %w", err)
	}

	return nil
}

// deleteLocation removes the location of a blob from Redis.
func (c *coordinatorBlobCache) deleteLocation(ctx context.Context, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := locationKey(digest)

	err := c.redis.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete blob location: %w", err)
	}

	return nil
}

// accessCounter is used to ensure unique member IDs for access events
// even when multiple accesses occur in the same millisecond.
//
//nolint:gochecknoglobals // Required for atomic counter operations across instances
var accessCounter uint64

// recordAccess records an access event for a blob using a sorted set.
// Each access is stored with the timestamp as the score and a unique ID as the member.
func (c *coordinatorBlobCache) recordAccess(ctx context.Context, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := accessKey(digest)
	now := time.Now().UnixMilli()

	// Use atomic increment to ensure unique member even in same millisecond
	// Format: timestamp:counter to ensure uniqueness
	counter := atomic.AddUint64(&accessCounter, 1)
	member := fmt.Sprintf("%d:%d", now, counter)

	// Add the access event
	err := c.redis.ZAdd(ctx, key, redis.Z{
		Score:  float64(now),
		Member: member,
	}).Err()
	if err != nil {
		return fmt.Errorf("failed to record access: %w", err)
	}

	// TODO: Refactor sliding window cleanup into a background task.
	// Clean up old entries outside the window (do this asynchronously to not block)
	windowStart := now - c.accessWindowDuration.Milliseconds()
	go func() {
		cleanErr := c.redis.ZRemRangeByScore(
			context.Background(),
			key,
			"-inf",
			strconv.FormatInt(windowStart, 10),
		).Err()
		if cleanErr != nil {
			c.logger.Debug("failed to clean old access entries",
				zap.String("digest", digest),
				zap.Error(cleanErr))
		}
	}()

	return nil
}

// getAccessCount returns the number of accesses within the sliding window.
func (c *coordinatorBlobCache) getAccessCount(ctx context.Context, digest string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	key := accessKey(digest)
	windowStart := time.Now().Add(-c.accessWindowDuration).UnixMilli()

	count, err := c.redis.ZCount(ctx, key,
		strconv.FormatInt(windowStart, 10),
		"+inf",
	).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get access count: %w", err)
	}

	return count, nil
}

// getTierCache returns the cache for the specified tier, or nil if not found.
func (c *coordinatorBlobCache) getTierCache(tier int) cache.BlobCache {
	for _, t := range c.tiers {
		if t.Tier == tier {
			return t.Cache
		}
	}
	return nil
}

// Head returns the descriptor for a cached blob.
// If the blob is on another node, returns [httptk.RedirectError] with the redirect URL.
// If the blob is on this node, returns the descriptor from the appropriate tier.
func (c *coordinatorBlobCache) Head(ctx context.Context, upstream, repo, digest string) (*v1.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.logger.Debug("Head called",
		zap.String("upstream", upstream),
		zap.String("repo", repo),
		zap.String("digest", digest))

	// Check Redis for blob location
	location, err := c.getLocation(ctx, digest)
	if errors.Is(err, ErrLocationNotFound) {
		// Not tracked in Redis, check local tiers
		return c.headFromLocalTiers(ctx, upstream, repo, digest)
	}
	if err != nil {
		c.logger.Debug("Head: failed to get location", zap.String("digest", digest), zap.Error(err))
		return nil, err
	}

	// Check if blob is on this node
	if location.NodeID != c.nodeID {
		redirectErr := httptk.NewBlobRedirectError(location.NodeID, upstream, repo, digest)
		c.logger.Debug("Head: blob on another node",
			zap.String("digest", digest),
			zap.String("remoteNode", location.NodeID),
			zap.String("redirectURL", redirectErr.URL))
		return nil, redirectErr
	}

	// Blob is on this node, get from the appropriate tier
	tierCache := c.getTierCache(location.Tier)
	if tierCache == nil {
		c.logger.Warn("Head: tier cache not found",
			zap.String("digest", digest),
			zap.Int("tier", location.Tier))
		// Tier doesn't exist, clean up Redis and return not found
		_ = c.deleteLocation(ctx, digest)
		return nil, cache.ErrBlobNotFound
	}

	desc, err := tierCache.Head(ctx, upstream, repo, digest)
	if err != nil {
		c.logger.Debug("Head: not found in tier cache",
			zap.String("digest", digest),
			zap.Int("tier", location.Tier),
			zap.Error(err))
		// Blob not in expected tier, clean up Redis
		_ = c.deleteLocation(ctx, digest)
		return nil, cache.ErrBlobNotFound
	}

	c.logger.Debug("Head: success",
		zap.String("digest", digest),
		zap.Int("tier", location.Tier))

	return desc, nil
}

// headFromLocalTiers checks local tiers for a blob not tracked in Redis.
func (c *coordinatorBlobCache) headFromLocalTiers(
	ctx context.Context,
	upstream, repo, digest string,
) (*v1.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	for _, tier := range c.tiers {
		desc, headErr := tier.Cache.Head(ctx, upstream, repo, digest)
		if headErr == nil {
			// Found locally but not in Redis - register it
			c.logger.Debug("Head: found locally, registering in Redis",
				zap.String("digest", digest),
				zap.Int("tier", tier.Tier))
			if setErr := c.setLocation(ctx, digest, c.nodeID, tier.Tier); setErr != nil {
				c.logger.Warn("Head: failed to register location",
					zap.String("digest", digest),
					zap.Error(setErr))
			}
			return desc, nil
		}
	}
	c.logger.Debug("Head: blob not found", zap.String("digest", digest))
	return nil, cache.ErrBlobNotFound
}

// Get retrieves a cached blob by digest.
// If the blob is on another node, returns [httptk.RedirectError] with the redirect URL.
// If the blob is on this node, returns the content and records an access event.
func (c *coordinatorBlobCache) Get(ctx context.Context, upstream, repo, digest string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.logger.Debug("Get called",
		zap.String("upstream", upstream),
		zap.String("repo", repo),
		zap.String("digest", digest))

	// Check Redis for blob location
	location, err := c.getLocation(ctx, digest)
	if errors.Is(err, ErrLocationNotFound) {
		// Not tracked in Redis, check local tiers
		return c.getFromLocalTiers(ctx, upstream, repo, digest)
	}
	if err != nil {
		c.logger.Debug("Get: failed to get location", zap.String("digest", digest), zap.Error(err))
		return nil, err
	}

	// Check if blob is on this node
	if location.NodeID != c.nodeID {
		redirectErr := httptk.NewBlobRedirectError(location.NodeID, upstream, repo, digest)
		c.logger.Debug("Get: blob on another node",
			zap.String("digest", digest),
			zap.String("remoteNode", location.NodeID),
			zap.String("redirectURL", redirectErr.URL))
		return nil, redirectErr
	}

	// Blob is on this node, get from the appropriate tier
	tierCache := c.getTierCache(location.Tier)
	if tierCache == nil {
		c.logger.Warn("Get: tier cache not found",
			zap.String("digest", digest),
			zap.Int("tier", location.Tier))
		_ = c.deleteLocation(ctx, digest)
		return nil, cache.ErrBlobNotFound
	}

	reader, err := tierCache.Get(ctx, upstream, repo, digest)
	if err != nil {
		c.logger.Debug("Get: not found in tier cache",
			zap.String("digest", digest),
			zap.Int("tier", location.Tier),
			zap.Error(err))
		_ = c.deleteLocation(ctx, digest)
		return nil, cache.ErrBlobNotFound
	}

	// Record access for local retrieval (not for redirects)
	if accessErr := c.recordAccess(ctx, digest); accessErr != nil {
		c.logger.Debug("Get: failed to record access",
			zap.String("digest", digest),
			zap.Error(accessErr))
	}

	c.logger.Debug("Get: success",
		zap.String("digest", digest),
		zap.Int("tier", location.Tier))

	// Wrap with in-flight tracking if enabled
	return c.wrapWithTracking(ctx, digest, reader), nil
}

// getFromLocalTiers checks local tiers for a blob not tracked in Redis.
func (c *coordinatorBlobCache) getFromLocalTiers(
	ctx context.Context,
	upstream, repo, digest string,
) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	for _, tier := range c.tiers {
		reader, getErr := tier.Cache.Get(ctx, upstream, repo, digest)
		if getErr == nil {
			// Found locally but not in Redis - register it
			c.logger.Debug("Get: found locally, registering in Redis",
				zap.String("digest", digest),
				zap.Int("tier", tier.Tier))
			if setErr := c.setLocation(ctx, digest, c.nodeID, tier.Tier); setErr != nil {
				c.logger.Warn("Get: failed to register location",
					zap.String("digest", digest),
					zap.Error(setErr))
			}
			// Record access for local retrieval
			if accessErr := c.recordAccess(ctx, digest); accessErr != nil {
				c.logger.Debug("Get: failed to record access",
					zap.String("digest", digest),
					zap.Error(accessErr))
			}
			// Wrap with in-flight tracking if enabled
			return c.wrapWithTracking(ctx, digest, reader), nil
		}
	}
	c.logger.Debug("Get: blob not found", zap.String("digest", digest))
	return nil, cache.ErrBlobNotFound
}

// wrapWithTracking wraps a reader with in-flight tracking if enabled.
// Returns the original reader if tracking is not enabled.
func (c *coordinatorBlobCache) wrapWithTracking(
	ctx context.Context,
	digest string,
	reader io.ReadCloser,
) io.ReadCloser {
	if c.inFlightTracker == nil {
		return reader
	}

	release, err := c.inFlightTracker.Increment(ctx, digest)
	if err != nil {
		c.logger.Debug("failed to track in-flight request",
			zap.String("digest", digest),
			zap.Error(err))
		return reader
	}

	return newTrackingReader(reader, release)
}

// Put stores a blob in the tier specified by the decision and registers its location in Redis.
func (c *coordinatorBlobCache) Put(
	ctx context.Context,
	upstream, repo, digest string,
	descriptor *v1.Descriptor,
	content io.Reader,
	decision *CacheLocationDecision,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.logger.Debug("Put called",
		zap.String("upstream", upstream),
		zap.String("repo", repo),
		zap.String("digest", digest),
		zap.Int64("size", descriptor.Size),
		zap.Int("tier", decision.Tier))

	tierCache := c.getTierCache(decision.Tier)
	if tierCache == nil {
		return fmt.Errorf("tier %d not found", decision.Tier)
	}

	err := tierCache.Put(ctx, upstream, repo, digest, descriptor, content)
	if err != nil {
		c.logger.Debug("Put: failed to store in tier",
			zap.String("digest", digest),
			zap.Int("tier", decision.Tier),
			zap.Error(err))
		return err
	}

	// Register location in Redis
	if setErr := c.setLocation(ctx, digest, c.nodeID, decision.Tier); setErr != nil {
		c.logger.Warn("Put: failed to register location in Redis",
			zap.String("digest", digest),
			zap.Error(setErr))
		// Don't fail the Put - the blob is stored, just not tracked
	}

	c.logger.Debug("Put: success",
		zap.String("digest", digest),
		zap.Int("tier", decision.Tier))

	return nil
}

// Delete removes a blob from all local tiers and Redis.
func (c *coordinatorBlobCache) Delete(ctx context.Context, upstream, repo, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.logger.Debug("Delete called",
		zap.String("upstream", upstream),
		zap.String("repo", repo),
		zap.String("digest", digest))

	// Delete from all local tiers
	for _, tier := range c.tiers {
		if err := tier.Cache.Delete(ctx, upstream, repo, digest); err != nil {
			c.logger.Debug("Delete: failed to delete from tier",
				zap.String("digest", digest),
				zap.Int("tier", tier.Tier),
				zap.Error(err))
			// Continue trying other tiers
		}
	}

	// Delete from Redis
	if err := c.deleteLocation(ctx, digest); err != nil {
		c.logger.Debug("Delete: failed to delete location from Redis",
			zap.String("digest", digest),
			zap.Error(err))
	}

	// Delete access history
	accessKeyStr := accessKey(digest)
	if err := c.redis.Del(ctx, accessKeyStr).Err(); err != nil {
		c.logger.Debug("Delete: failed to delete access history",
			zap.String("digest", digest),
			zap.Error(err))
	}

	c.logger.Debug("Delete: success", zap.String("digest", digest))
	return nil
}

// GetAccessCount returns the number of accesses for a blob within the sliding window.
// This is exposed for use by promotion/demotion logic.
func (c *coordinatorBlobCache) GetAccessCount(ctx context.Context, digest string) (int64, error) {
	return c.getAccessCount(ctx, digest)
}

// GetLocation returns the current location of a blob.
// This is exposed for use by promotion/demotion logic.
func (c *coordinatorBlobCache) GetLocation(ctx context.Context, digest string) (*BlobLocation, error) {
	return c.getLocation(ctx, digest)
}

// SetLocation updates the location of a blob in Redis.
// This is exposed for use by promotion/demotion logic when moving blobs between tiers.
func (c *coordinatorBlobCache) SetLocation(ctx context.Context, digest string, nodeID string, tier int) error {
	return c.setLocation(ctx, digest, nodeID, tier)
}

// NodeID returns the node ID of this coordinator.
func (c *coordinatorBlobCache) NodeID() string {
	return c.nodeID
}

// FindCacheLocation determines where a blob of the given size should be cached.
func (c *coordinatorBlobCache) FindCacheLocation(
	ctx context.Context,
	blobSize int64,
) (*CacheLocationDecision, *Reservation, error) {
	return c.informer.FindCacheLocation(ctx, blobSize)
}

// LocalBlobInfo contains metadata about a cached blob with its tier annotation.
type LocalBlobInfo struct {
	// Digest is the content-addressable digest of the blob.
	Digest string
	// Size is the size of the blob in bytes.
	Size int64
	// MediaType is the OCI media type of the blob.
	MediaType string
	// Path is the filesystem path where the blob is stored.
	Path string
	// Tier is the cache tier number where this blob resides.
	Tier int
}

// ListLocal returns information about all blobs cached locally across all tiers.
func (c *coordinatorBlobCache) ListLocal(ctx context.Context) ([]LocalBlobInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var result []LocalBlobInfo
	for _, tier := range c.tiers {
		blobs, err := tier.Cache.List(ctx)
		if err != nil {
			c.logger.Warn("ListLocal: failed to list tier",
				zap.Int("tier", tier.Tier),
				zap.Error(err))
			continue
		}

		for _, b := range blobs {
			result = append(result, LocalBlobInfo{
				Digest:    b.Digest,
				Size:      b.Size,
				MediaType: b.MediaType,
				Path:      b.Path,
				Tier:      tier.Tier,
			})
		}
	}

	return result, nil
}

// GetAccessCountWindow returns the number of accesses for a blob within the given duration window.
func (c *coordinatorBlobCache) GetAccessCountWindow(
	ctx context.Context,
	digest string,
	window time.Duration,
) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	key := accessKey(digest)
	windowStart := time.Now().Add(-window).UnixMilli()

	count, err := c.redis.ZCount(ctx, key,
		strconv.FormatInt(windowStart, 10),
		"+inf",
	).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get access count: %w", err)
	}

	return count, nil
}

// DeleteLocalOnly removes a blob from all local disk tiers and conditionally cleans up Redis.
// Redis location and access history are only deleted if Redis indicates the blob is on this node.
func (c *coordinatorBlobCache) DeleteLocalOnly(ctx context.Context, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.logger.Debug("DeleteLocalOnly called", zap.String("digest", digest))

	// Delete from all local tiers
	for _, tier := range c.tiers {
		if err := tier.Cache.Delete(ctx, "", "", digest); err != nil {
			c.logger.Debug("DeleteLocalOnly: failed to delete from tier",
				zap.String("digest", digest),
				zap.Int("tier", tier.Tier),
				zap.Error(err))
		}
	}

	// Check Redis: only clean up if the blob is registered to this node
	location, err := c.getLocation(ctx, digest)
	if err != nil {
		if errors.Is(err, ErrLocationNotFound) {
			c.logger.Debug("DeleteLocalOnly: no location in Redis, skipping Redis cleanup",
				zap.String("digest", digest))
			return nil
		}
		c.logger.Debug("DeleteLocalOnly: failed to get location",
			zap.String("digest", digest),
			zap.Error(err))
		return nil
	}

	if location.NodeID != c.nodeID {
		c.logger.Debug("DeleteLocalOnly: blob registered to different node, skipping Redis cleanup",
			zap.String("digest", digest),
			zap.String("registeredNode", location.NodeID))
		return nil
	}

	// Blob is registered to this node - clean up Redis
	if delErr := c.deleteLocation(ctx, digest); delErr != nil {
		c.logger.Debug("DeleteLocalOnly: failed to delete location from Redis",
			zap.String("digest", digest),
			zap.Error(delErr))
	}

	accessKeyStr := accessKey(digest)
	if delErr := c.redis.Del(ctx, accessKeyStr).Err(); delErr != nil {
		c.logger.Debug("DeleteLocalOnly: failed to delete access history",
			zap.String("digest", digest),
			zap.Error(delErr))
	}

	c.logger.Debug("DeleteLocalOnly: success", zap.String("digest", digest))
	return nil
}
