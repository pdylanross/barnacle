package upstream

import (
	"context"
	"errors"
	"io"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pdylanross/barnacle/internal/registry/cache"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/internal/registry/data"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	"go.uber.org/zap"
)

// NewCachingUpstream creates a new caching upstream that wraps another upstream.
// It uses the provided caches to store and retrieve manifests and blobs,
// falling back to the inner upstream when items are not cached.
// The upstreamName is used to namespace cache entries.
func NewCachingUpstream(
	logger *zap.Logger,
	upstreamName string,
	manifestCache cache.ManifestCache,
	blobCache coordinator.Cache,
	inner Upstream,
) (Upstream, error) {
	return &cachingUpstream{
		logger:        logger.Named("cachingUpstream"),
		upstreamName:  upstreamName,
		manifestCache: manifestCache,
		blobCache:     blobCache,
		inner:         inner,
	}, nil
}

type cachingUpstream struct {
	logger        *zap.Logger
	upstreamName  string
	manifestCache cache.ManifestCache
	blobCache     coordinator.Cache
	inner         Upstream
}

// isDigest returns true if the reference is a digest (algorithm:hash format).
func isDigest(reference string) bool {
	return strings.HasPrefix(reference, "sha256:") ||
		strings.HasPrefix(reference, "sha384:") ||
		strings.HasPrefix(reference, "sha512:")
}

// HeadManifest checks if a manifest exists, using the cache when possible.
// For tag references, it first checks the tag-to-digest cache mapping.
// For digest references, it checks if the manifest is already cached.
// When a cached tag is stale (past TTL), it revalidates against the upstream.
func (c *cachingUpstream) HeadManifest(ctx context.Context, repo string, reference string) (*v1.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.logger.Debug("HeadManifest called",
		zap.String("upstream", c.upstreamName),
		zap.String("repo", repo),
		zap.String("reference", reference))

	digest := reference
	if !isDigest(reference) {
		// Reference is a tag, try to resolve from cache
		cachedDigest, stale, err := c.manifestCache.TagToDigest(c.upstreamName, repo, reference)
		if err == nil {
			if stale {
				c.logger.Debug("tag is stale, revalidating from upstream",
					zap.String("tag", reference),
					zap.String("cachedDigest", cachedDigest))
				// Tag is stale, revalidate from upstream
				return c.headManifestFromUpstream(ctx, repo, reference)
			}
			c.logger.Debug("tag resolved from cache",
				zap.String("tag", reference),
				zap.String("digest", cachedDigest))
			digest = cachedDigest
		} else {
			c.logger.Debug("tag not in cache, fetching from upstream",
				zap.String("tag", reference))
			// Tag not cached, need to fetch from upstream
			return c.headManifestFromUpstream(ctx, repo, reference)
		}
	}

	// Try to get manifest info from cache
	elem, manifestType, err := c.manifestCache.GetUntyped(c.upstreamName, repo, digest)
	if err == nil {
		c.logger.Debug("manifest found in cache",
			zap.String("digest", digest),
			zap.Int("type", int(manifestType)))
		return c.descriptorFromCachedManifest(elem, manifestType)
	}

	c.logger.Debug("manifest not in cache, fetching from upstream",
		zap.String("digest", digest))
	return c.headManifestFromUpstream(ctx, repo, reference)
}

// headManifestFromUpstream fetches manifest metadata from the inner upstream.
func (c *cachingUpstream) headManifestFromUpstream(
	ctx context.Context,
	repo string,
	reference string,
) (*v1.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	desc, err := c.inner.HeadManifest(ctx, repo, reference)
	if err != nil {
		c.logger.Debug("upstream HeadManifest failed", zap.Error(err))
		return nil, err
	}

	c.logger.Debug("upstream HeadManifest success",
		zap.String("digest", desc.Digest.String()),
		zap.String("mediaType", string(desc.MediaType)),
		zap.Int64("size", desc.Size))

	return desc, nil
}

// descriptorFromCachedManifest creates a v1.Descriptor from a cached manifest.
func (c *cachingUpstream) descriptorFromCachedManifest(
	elem any,
	manifestType cache.ManifestType,
) (*v1.Descriptor, error) {
	switch manifestType {
	case cache.ManifestTypeImage:
		img, ok := elem.(*data.ImageManifestResponse)
		if !ok {
			return nil, cache.ErrItemTypeMismatch
		}
		return &v1.Descriptor{
			Digest:    img.Digest,
			Size:      img.Size,
			MediaType: img.MediaType,
		}, nil
	case cache.ManifestTypeIndex:
		idx, ok := elem.(*data.IndexManifestResponse)
		if !ok {
			return nil, cache.ErrItemTypeMismatch
		}
		return &v1.Descriptor{
			Digest:    idx.Digest,
			Size:      idx.Size,
			MediaType: idx.MediaType,
		}, nil
	default:
		c.logger.Warn("unknown manifest type in cache", zap.Int("type", int(manifestType)))
		return nil, cache.ErrItemTypeMismatch
	}
}

// IndexManifest retrieves an index manifest, using the cache when possible.
// For tag references, it first checks the tag-to-digest cache mapping.
// If the manifest is cached, it returns it directly.
// Otherwise, it fetches from the inner upstream and caches the result.
// When a cached tag is stale (past TTL), it revalidates against the upstream.
//
//nolint:dupl // IndexManifest and ImageManifest have similar structure but handle different types
func (c *cachingUpstream) IndexManifest(
	ctx context.Context,
	repo string,
	reference string,
) (*data.IndexManifestResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.logger.Debug("IndexManifest called",
		zap.String("upstream", c.upstreamName),
		zap.String("repo", repo),
		zap.String("reference", reference))

	digest := reference
	if !isDigest(reference) {
		// Reference is a tag, try to resolve from cache
		cachedDigest, stale, err := c.manifestCache.TagToDigest(c.upstreamName, repo, reference)
		if err == nil {
			if stale {
				c.logger.Debug("tag is stale, revalidating from upstream",
					zap.String("tag", reference),
					zap.String("cachedDigest", cachedDigest))
				// Tag is stale, revalidate from upstream
				return c.indexManifestFromUpstream(ctx, repo, reference)
			}
			c.logger.Debug("tag resolved from cache",
				zap.String("tag", reference),
				zap.String("digest", cachedDigest))
			digest = cachedDigest
		} else {
			c.logger.Debug("tag not in cache, will fetch from upstream",
				zap.String("tag", reference))
			// Tag not cached, fetch from upstream
			return c.indexManifestFromUpstream(ctx, repo, reference)
		}
	}

	// Try to get from cache by digest
	cached, err := c.manifestCache.GetIndex(c.upstreamName, repo, digest)
	if err == nil {
		c.logger.Debug("index manifest cache hit",
			zap.String("digest", digest))
		return cached, nil
	}

	c.logger.Debug("index manifest cache miss, fetching from upstream",
		zap.String("digest", digest))

	// Not in cache, fetch from upstream using digest
	return c.indexManifestFromUpstream(ctx, repo, digest)
}

// indexManifestFromUpstream fetches an index manifest from the inner upstream
// and caches it for future requests.
func (c *cachingUpstream) indexManifestFromUpstream(
	ctx context.Context,
	repo string,
	reference string,
) (*data.IndexManifestResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manifest, err := c.inner.IndexManifest(ctx, repo, reference)
	if err != nil {
		c.logger.Debug("upstream IndexManifest failed", zap.Error(err))
		return nil, err
	}

	c.logger.Debug("upstream IndexManifest success",
		zap.String("digest", manifest.Digest.String()),
		zap.Int("size", len(manifest.RawManifest)))

	// Cache the manifest by digest
	digestStr := manifest.Digest.String()
	if putErr := c.manifestCache.PutIndex(c.upstreamName, repo, digestStr, manifest); putErr != nil {
		c.logger.Warn("failed to cache index manifest",
			zap.String("digest", digestStr),
			zap.Error(putErr))
	} else {
		c.logger.Debug("cached index manifest",
			zap.String("digest", digestStr))
	}

	// If reference was a tag, also cache the tag-to-digest mapping
	if !isDigest(reference) {
		if putErr := c.manifestCache.PutTag(c.upstreamName, repo, reference, digestStr); putErr != nil {
			c.logger.Warn("failed to cache tag mapping",
				zap.String("tag", reference),
				zap.String("digest", digestStr),
				zap.Error(putErr))
		} else {
			c.logger.Debug("cached tag mapping",
				zap.String("tag", reference),
				zap.String("digest", digestStr))
		}
	}

	return manifest, nil
}

// ImageManifest retrieves an image manifest, using the cache when possible.
// For tag references, it first checks the tag-to-digest cache mapping.
// If the manifest is cached, it returns it directly.
// Otherwise, it fetches from the inner upstream and caches the result.
// When a cached tag is stale (past TTL), it revalidates against the upstream.
//
//nolint:dupl // IndexManifest and ImageManifest have similar structure but handle different types
func (c *cachingUpstream) ImageManifest(
	ctx context.Context,
	repo string,
	reference string,
) (*data.ImageManifestResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.logger.Debug("ImageManifest called",
		zap.String("upstream", c.upstreamName),
		zap.String("repo", repo),
		zap.String("reference", reference))

	digest := reference
	if !isDigest(reference) {
		// Reference is a tag, try to resolve from cache
		cachedDigest, stale, err := c.manifestCache.TagToDigest(c.upstreamName, repo, reference)
		if err == nil {
			if stale {
				c.logger.Debug("tag is stale, revalidating from upstream",
					zap.String("tag", reference),
					zap.String("cachedDigest", cachedDigest))
				// Tag is stale, revalidate from upstream
				return c.imageManifestFromUpstream(ctx, repo, reference)
			}
			c.logger.Debug("tag resolved from cache",
				zap.String("tag", reference),
				zap.String("digest", cachedDigest))
			digest = cachedDigest
		} else {
			c.logger.Debug("tag not in cache, will fetch from upstream",
				zap.String("tag", reference))
			// Tag not cached, fetch from upstream
			return c.imageManifestFromUpstream(ctx, repo, reference)
		}
	}

	// Try to get from cache by digest
	cached, err := c.manifestCache.GetImage(c.upstreamName, repo, digest)
	if err == nil {
		c.logger.Debug("image manifest cache hit",
			zap.String("digest", digest))
		return cached, nil
	}

	c.logger.Debug("image manifest cache miss, fetching from upstream",
		zap.String("digest", digest))

	// Not in cache, fetch from upstream using digest
	return c.imageManifestFromUpstream(ctx, repo, digest)
}

// imageManifestFromUpstream fetches an image manifest from the inner upstream
// and caches it for future requests.
func (c *cachingUpstream) imageManifestFromUpstream(
	ctx context.Context,
	repo string,
	reference string,
) (*data.ImageManifestResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manifest, err := c.inner.ImageManifest(ctx, repo, reference)
	if err != nil {
		c.logger.Debug("upstream ImageManifest failed", zap.Error(err))
		return nil, err
	}

	c.logger.Debug("upstream ImageManifest success",
		zap.String("digest", manifest.Digest.String()),
		zap.Int("size", len(manifest.RawManifest)))

	// Cache the manifest by digest
	digestStr := manifest.Digest.String()
	if putErr := c.manifestCache.PutImage(c.upstreamName, repo, digestStr, manifest); putErr != nil {
		c.logger.Warn("failed to cache image manifest",
			zap.String("digest", digestStr),
			zap.Error(putErr))
	} else {
		c.logger.Debug("cached image manifest",
			zap.String("digest", digestStr))
	}

	// If reference was a tag, also cache the tag-to-digest mapping
	if !isDigest(reference) {
		if putErr := c.manifestCache.PutTag(c.upstreamName, repo, reference, digestStr); putErr != nil {
			c.logger.Warn("failed to cache tag mapping",
				zap.String("tag", reference),
				zap.String("digest", digestStr),
				zap.Error(putErr))
		} else {
			c.logger.Debug("cached tag mapping",
				zap.String("tag", reference),
				zap.String("digest", digestStr))
		}
	}

	return manifest, nil
}

// HeadBlob checks if a blob exists, using the cache when possible.
// It first checks the blob cache, falling back to the upstream if not cached.
func (c *cachingUpstream) HeadBlob(ctx context.Context, repo string, digest v1.Hash) (*v1.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	digestStr := digest.String()
	c.logger.Debug("HeadBlob called",
		zap.String("upstream", c.upstreamName),
		zap.String("repo", repo),
		zap.String("digest", digestStr))

	// Check cache first (blobs are content-addressable, upstream/repo used for routing)
	desc, err := c.blobCache.Head(ctx, c.upstreamName, repo, digestStr)
	if err == nil {
		c.logger.Debug("blob cache hit",
			zap.String("digest", digestStr),
			zap.Int64("size", desc.Size))
		return desc, nil
	}

	// Propagate RedirectError — the blob exists on another node
	var redirectErr *httptk.RedirectError
	if errors.As(err, &redirectErr) {
		c.logger.Debug("blob cached on another node, redirecting",
			zap.String("digest", digestStr),
			zap.String("redirectURL", redirectErr.URL))
		return nil, err
	}

	c.logger.Debug("blob cache miss", zap.String("digest", digestStr))

	// Not in cache, fetch from upstream
	desc, err = c.inner.HeadBlob(ctx, repo, digest)
	if err != nil {
		c.logger.Debug("upstream HeadBlob failed", zap.Error(err))
		return nil, err
	}

	c.logger.Debug("upstream HeadBlob success",
		zap.String("digest", digestStr),
		zap.Int64("size", desc.Size))

	return desc, nil
}

// GetBlob retrieves blob content, using the cache when possible.
// If the blob is cached, it returns the cached content.
// If not cached, it fetches from the upstream and writes to the cache while returning
// the content to the caller (using a TeeReader to multiplex the stream).
func (c *cachingUpstream) GetBlob(ctx context.Context, repo string, digest v1.Hash) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	digestStr := digest.String()
	c.logger.Debug("GetBlob called",
		zap.String("upstream", c.upstreamName),
		zap.String("repo", repo),
		zap.String("digest", digestStr))

	// Check cache first (blobs are content-addressable, upstream/repo used for routing)
	reader, err := c.blobCache.Get(ctx, c.upstreamName, repo, digestStr)
	if err == nil {
		c.logger.Debug("blob cache hit, returning cached content",
			zap.String("digest", digestStr))
		return reader, nil
	}

	// Propagate RedirectError — the blob exists on another node
	var redirectErr *httptk.RedirectError
	if errors.As(err, &redirectErr) {
		c.logger.Debug("blob cached on another node, redirecting",
			zap.String("digest", digestStr),
			zap.String("redirectURL", redirectErr.URL))
		return nil, err
	}

	c.logger.Debug("blob cache miss", zap.String("digest", digestStr))

	// Not in cache, need to fetch from upstream
	// First get the descriptor so we can cache it along with the content
	desc, err := c.inner.HeadBlob(ctx, repo, digest)
	if err != nil {
		c.logger.Debug("upstream HeadBlob failed", zap.Error(err))
		return nil, err
	}

	// Check where this blob should be cached
	decision, reservation, findErr := c.blobCache.FindCacheLocation(ctx, desc.Size)
	if findErr != nil {
		// No capacity or other error — stream upstream directly without caching
		c.logger.Warn("FindCacheLocation failed, streaming without caching",
			zap.String("digest", digestStr),
			zap.Error(findErr))
		return c.inner.GetBlob(ctx, repo, digest)
	}

	if !decision.Local {
		// Blob should be cached on a remote node — redirect (reservation is nil for remote)
		newRedirectErr := httptk.NewBlobRedirectError(decision.NodeID, c.upstreamName, repo, digestStr)
		c.logger.Debug("blob should be cached on another node, redirecting",
			zap.String("digest", digestStr),
			zap.String("targetNode", decision.NodeID),
			zap.String("redirectURL", newRedirectErr.URL))
		return nil, newRedirectErr
	}

	// Get the blob content from upstream
	upstreamReader, err := c.inner.GetBlob(ctx, repo, digest)
	if err != nil {
		c.logger.Debug("upstream GetBlob failed", zap.Error(err))
		reservation.Release()
		return nil, err
	}

	c.logger.Debug("upstream GetBlob success, will cache while streaming",
		zap.String("digest", digestStr),
		zap.Int64("size", desc.Size))

	// Create a pipe to multiplex the stream: one end for the caller, one for caching
	pipeReader, pipeWriter := io.Pipe()

	// Use TeeReader to write to both the pipe (for caller) and capture for caching
	teeReader := io.TeeReader(upstreamReader, pipeWriter)

	// Start a goroutine to read from upstream, write to cache, and close the pipe
	// Capture repo for use in the goroutine since it may outlive the request
	repoName := repo
	go func() { //nolint:gosec // intentionally outlives request context for background caching
		defer reservation.Release()
		defer upstreamReader.Close()
		defer pipeWriter.Close()

		// Read all content through the TeeReader (which writes to pipeWriter)
		// and simultaneously write to the cache.
		// Use Background context since this goroutine outlives the request.
		if putErr := c.blobCache.Put(
			context.Background(),
			c.upstreamName,
			repoName,
			digestStr,
			desc,
			teeReader,
			decision,
		); putErr != nil {
			c.logger.Warn("failed to cache blob",
				zap.String("digest", digestStr),
				zap.Error(putErr))
		} else {
			c.logger.Debug("blob cached successfully",
				zap.String("digest", digestStr))
		}
	}()

	return pipeReader, nil
}
