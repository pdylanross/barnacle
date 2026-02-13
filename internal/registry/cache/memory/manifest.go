// Package memory provides an in-memory implementation of the manifest cache.
// It uses ristretto, a high-performance concurrent cache, for both tag mappings
// and manifest storage.
//
// The cache enforces memory limits through cost-based eviction. Manifests are
// stored with their byte size as cost, while tags use a fixed cost of 1.
// When the cache exceeds its configured limits, least-recently-used entries
// are automatically evicted.
package memory

import (
	"errors"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/pdylanross/barnacle/internal/registry/cache"
	"github.com/pdylanross/barnacle/internal/registry/data"
)

// Cache errors returned by the memory manifest cache.
var (
	// ErrTagNotFound is returned when a tag lookup fails because the tag is not cached.
	ErrTagNotFound = errors.New("tag not found")
	// ErrItemNotFound is returned when a manifest lookup fails because the digest is not cached.
	ErrItemNotFound = errors.New("item not found")
)

const (
	// tagCost is the fixed cost assigned to each tag entry in the cache.
	// Tags are lightweight mappings from tag name to digest string.
	tagCost int64 = 1

	// ristrettoNumCountersMultiplier is the multiplier used to calculate NumCounters
	// from the max cost. Ristretto recommends 10x the max number of items.
	ristrettoNumCountersMultiplier = 10

	// ristrettoBufferItems is the number of keys per Get buffer in ristretto.
	ristrettoBufferItems = 64

	// bytesPerMebibyte is the number of bytes in a mebibyte (MiB).
	bytesPerMebibyte int64 = 1024 * 1024
)

// NewManifestCache creates a new in-memory manifest cache with the given options.
// The cache uses ristretto for high-performance concurrent access with automatic
// LRU eviction when memory limits are exceeded.
//
// Returns an error if the underlying ristretto caches fail to initialize.
func NewManifestCache(opt *CacheOptions) (cache.ManifestCache, error) {
	tagCacheOptions := ristretto.Config[string, tagCacheElem]{
		MaxCost:     int64(opt.TagLimit),
		NumCounters: int64(ristrettoNumCountersMultiplier * opt.TagLimit),
		BufferItems: ristrettoBufferItems,
	}

	tagCache, err := ristretto.NewCache(&tagCacheOptions)
	if err != nil {
		return nil, err
	}

	cacheSizeBytes := int64(opt.ManifestMemoryLimitMi) * bytesPerMebibyte
	manifestCacheOptions := ristretto.Config[string, cacheElem]{
		MaxCost:     cacheSizeBytes,
		NumCounters: cacheSizeBytes * ristrettoNumCountersMultiplier,
		BufferItems: ristrettoBufferItems,
	}

	manifestCache, err := ristretto.NewCache(&manifestCacheOptions)
	if err != nil {
		return nil, err
	}

	return &memoryManifestCache{
		tagCache:      tagCache,
		manifestCache: manifestCache,
		tagTTL:        opt.TagTTL,
		now:           time.Now,
	}, nil
}

// CacheOptions configures the memory limits for the manifest cache.
type CacheOptions struct {
	// TagLimit is the maximum number of tag-to-digest mappings to cache.
	TagLimit int `json:"tagLimit"`
	// ManifestMemoryLimitMi is the maximum memory in mebibytes (MiB) for manifest storage.
	ManifestMemoryLimitMi int `json:"manifestMemoryLimit"`
	// TagTTL is the duration after which a cached tag is considered stale
	// and should be revalidated against the upstream.
	TagTTL time.Duration `json:"tagTTL"`
}

// tagCacheElem stores a tag-to-digest mapping along with when it was cached.
type tagCacheElem struct {
	// digest is the content-addressable digest the tag resolves to.
	digest string
	// cachedAt is when this tag mapping was stored.
	cachedAt time.Time
}

// memoryManifestCache implements cache.ManifestCache using in-memory ristretto caches.
// It maintains two separate caches: one for tag-to-digest mappings and one for
// manifest content keyed by digest.
type memoryManifestCache struct {
	// tagCache maps "upstream/repo:tag" keys to tag cache elements containing digest and timestamp.
	tagCache *ristretto.Cache[string, tagCacheElem]
	// manifestCache maps "upstream/repo:digest" keys to cached manifest elements.
	manifestCache *ristretto.Cache[string, cacheElem]
	// tagTTL is the duration after which a cached tag is considered stale.
	tagTTL time.Duration
	// now returns the current time. Defaults to time.Now but can be overridden for testing.
	now func() time.Time
}

// cacheElem wraps a cached manifest with its type and size metadata.
type cacheElem struct {
	// elemType indicates whether this is an image or index manifest.
	elemType cache.ManifestType
	// elemValue holds the actual manifest (*data.ImageManifestResponse or *data.IndexManifestResponse).
	elemValue any
	// size is the byte size of the manifest, used as the cost for cache eviction.
	size int64
}

func (m *memoryManifestCache) GetUntyped(upstream, repo, digest string) (any, cache.ManifestType, error) {
	key := m.toManifestCacheKey(upstream, repo, digest)
	elem, ok := m.manifestCache.Get(key)
	if !ok {
		return nil, cache.ManifestTypeImage, ErrItemNotFound
	}
	return elem.elemValue, elem.elemType, nil
}

func (m *memoryManifestCache) GetType(upstream, repo, digest string) (cache.ManifestType, error) {
	key := m.toManifestCacheKey(upstream, repo, digest)
	elem, ok := m.manifestCache.Get(key)
	if !ok {
		return cache.ManifestTypeImage, ErrItemNotFound
	}
	return elem.elemType, nil
}

func (m *memoryManifestCache) TagToDigest(upstream, repo, tag string) (string, bool, error) {
	elem, ok := m.tagCache.Get(m.toTagCacheKey(upstream, repo, tag))
	if !ok {
		return "", false, ErrTagNotFound
	}

	// Check if the cached entry is stale (past TTL).
	// If tagTTL is 0, entries never become stale.
	stale := false
	if m.tagTTL > 0 {
		stale = m.now().Sub(elem.cachedAt) > m.tagTTL
	}

	return elem.digest, stale, nil
}

func (m *memoryManifestCache) GetImage(upstream, repo, digest string) (*data.ImageManifestResponse, error) {
	elem, elemType, err := m.GetUntyped(upstream, repo, digest)
	if err != nil {
		return nil, err
	}

	if elemType != cache.ManifestTypeImage {
		return nil, cache.ErrItemTypeMismatch
	}

	result, ok := elem.(*data.ImageManifestResponse)
	if !ok {
		return nil, cache.ErrItemTypeMismatch
	}

	return result, nil
}

func (m *memoryManifestCache) GetIndex(upstream, repo, digest string) (*data.IndexManifestResponse, error) {
	elem, elemType, err := m.GetUntyped(upstream, repo, digest)
	if err != nil {
		return nil, err
	}

	if elemType != cache.ManifestTypeIndex {
		return nil, cache.ErrItemTypeMismatch
	}

	result, ok := elem.(*data.IndexManifestResponse)
	if !ok {
		return nil, cache.ErrItemTypeMismatch
	}

	return result, nil
}

func (m *memoryManifestCache) PutImage(upstream, repo, digest string, elem *data.ImageManifestResponse) error {
	cElem := cacheElem{
		size:      elem.Size,
		elemValue: elem,
		elemType:  cache.ManifestTypeImage,
	}

	return m.put(upstream, repo, digest, cElem)
}

func (m *memoryManifestCache) PutIndex(upstream, repo, digest string, elem *data.IndexManifestResponse) error {
	cElem := cacheElem{
		size:      elem.Size,
		elemValue: elem,
		elemType:  cache.ManifestTypeIndex,
	}

	return m.put(upstream, repo, digest, cElem)
}

func (m *memoryManifestCache) put(upstream, repo, digest string, elem cacheElem) error {
	m.manifestCache.Set(m.toManifestCacheKey(upstream, repo, digest), elem, elem.size)
	return nil
}

func (m *memoryManifestCache) PutTag(upstream, repo, tag, digest string) error {
	elem := tagCacheElem{
		digest:   digest,
		cachedAt: m.now(),
	}
	m.tagCache.Set(m.toTagCacheKey(upstream, repo, tag), elem, tagCost)
	return nil
}

// toManifestCacheKey builds the cache key for manifest storage.
// Format: "upstream/repo:digest".
func (m *memoryManifestCache) toManifestCacheKey(upstream, repo, digest string) string {
	return upstream + "/" + repo + ":" + digest
}

// toTagCacheKey builds the cache key for tag-to-digest mappings.
// Format: "upstream/repo:tag".
func (m *memoryManifestCache) toTagCacheKey(upstream, repo, tag string) string {
	return upstream + "/" + repo + ":" + tag
}
