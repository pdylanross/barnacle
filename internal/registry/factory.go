package registry

import (
	"fmt"

	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/internal/registry/cache"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/internal/registry/cache/disk"
	"github.com/pdylanross/barnacle/internal/registry/cache/memory"
	"github.com/pdylanross/barnacle/internal/registry/upstream"
	"github.com/pdylanross/barnacle/internal/registry/upstream/standard"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// buildUpstream creates an upstream registry instance from the given configuration.
// The upstream is wrapped with a caching layer that uses the provided caches.
func buildUpstream(
	name string,
	cfg *configuration.UpstreamConfiguration,
	logger *zap.Logger,
	manifestCache cache.ManifestCache,
	blobCache coordinator.Cache,
) (upstream.Upstream, error) {
	var inner upstream.Upstream

	//nolint:gocritic // Switch statement reserved for future upstream types
	switch cfg.Registry {
	default:
		up, err := standard.New(cfg, logger, name)
		if err != nil {
			return nil, err
		}
		inner = up
	}

	return upstream.NewCachingUpstream(logger, name, manifestCache, blobCache, inner)
}

// buildUpstreams creates a map of upstream registries from the application configuration.
// The map keys are the upstream aliases, and the values are the upstream instances.
// All upstreams share the same manifest and blob caches.
// Returns an error if any upstream fails to initialize.
func buildUpstreams(
	cfg *configuration.Configuration,
	logger *zap.Logger,
	redisClient *redis.Client,
	nodeRegistry *node.Registry,
	inFlightTracker coordinator.InFlightTracker,
) (map[string]upstream.Upstream, coordinator.Cache, error) {
	// Create shared caches for all upstreams
	manifestCache, err := memory.NewManifestCache(&memory.CacheOptions{
		TagLimit:              cfg.Cache.Memory.TagLimit,
		ManifestMemoryLimitMi: cfg.Cache.Memory.ManifestMemoryLimitMi,
		TagTTL:                cfg.Cache.Memory.TagTTL,
	})
	if err != nil {
		return nil, nil, err
	}

	// Create disk-based blob cache for each tier
	tierCaches := make([]coordinator.TierCache, 0, len(cfg.Cache.Disk.Tiers))
	for _, tierCfg := range cfg.Cache.Disk.Tiers {
		diskCache, diskErr := disk.NewBlobCache(&disk.BlobCacheOptions{
			BasePath:        tierCfg.Path,
			DescriptorLimit: cfg.Cache.Disk.DescriptorLimit,
			Logger:          logger,
		})
		if diskErr != nil {
			return nil, nil, fmt.Errorf("failed to create disk cache for tier %d: %w", tierCfg.Tier, diskErr)
		}

		tierCaches = append(tierCaches, coordinator.TierCache{
			Tier:  tierCfg.Tier,
			Cache: diskCache,
		})
	}

	// Create coordinator cache that manages all tiers
	blobCache, err := coordinator.NewBlobCache(&coordinator.Options{
		Redis:           redisClient,
		Tiers:           tierCaches,
		Logger:          logger,
		NodeRegistry:    nodeRegistry,
		InFlightTracker: inFlightTracker,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create coordinator cache: %w", err)
	}

	ret := make(map[string]upstream.Upstream)
	for key, upstreamCfg := range cfg.Upstreams {
		var newUpstream upstream.Upstream
		newUpstream, err = buildUpstream(key, &upstreamCfg, logger, manifestCache, blobCache)
		if err != nil {
			return nil, nil, err
		}
		ret[key] = newUpstream
	}

	return ret, blobCache, nil
}
