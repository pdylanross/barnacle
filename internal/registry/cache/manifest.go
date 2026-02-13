// Package cache provides caching interfaces and implementations for OCI registry artifacts.
// It supports caching of manifests (both image and index types) and blobs to reduce
// upstream registry requests and improve response times.
package cache

import (
	"errors"

	"github.com/pdylanross/barnacle/internal/registry/data"
)

// Common cache errors.
var (
	// ErrItemTypeMismatch is returned when attempting to retrieve a manifest as the wrong type
	// (e.g., calling GetImage on an index manifest).
	ErrItemTypeMismatch = errors.New("item type mismatch")
)

// ManifestType indicates whether a cached manifest is an image or index manifest.
type ManifestType int

// Manifest type constants for distinguishing between image and index manifests in the cache.
const (
	// ManifestTypeIndex indicates the manifest is an OCI image index (multi-platform manifest list).
	ManifestTypeIndex ManifestType = iota + 1
	// ManifestTypeImage indicates the manifest is a single OCI image manifest.
	ManifestTypeImage
)

// ManifestCache defines the interface for caching OCI manifests.
// Implementations must be safe for concurrent access.
//
// The cache stores manifests by their content-addressable digest and maintains
// a separate mapping from tags to digests. This allows efficient lookups by
// either tag or digest while ensuring content integrity.
//
// All methods require an upstream name parameter to namespace the cache entries.
// Cache keys are formatted as "upstream/repo:<tag or digest>".
type ManifestCache interface {
	// GetType returns the manifest type (image or index) for the given digest.
	// Returns ErrItemNotFound if the manifest is not cached.
	GetType(upstream, repo, digest string) (ManifestType, error)

	// TagToDigest resolves a tag to its corresponding digest.
	// Returns the digest and whether the cached entry is stale (past its TTL).
	// When stale is true, callers should revalidate the tag against the upstream.
	// Returns ErrTagNotFound if the tag is not cached.
	TagToDigest(upstream, repo, tag string) (digest string, stale bool, err error)

	// GetImage retrieves a cached image manifest by digest.
	// Returns ErrItemNotFound if not cached, or ErrItemTypeMismatch if the
	// cached item is not an image manifest.
	GetImage(upstream, repo, digest string) (*data.ImageManifestResponse, error)

	// GetIndex retrieves a cached index manifest by digest.
	// Returns ErrItemNotFound if not cached, or ErrItemTypeMismatch if the
	// cached item is not an index manifest.
	GetIndex(upstream, repo, digest string) (*data.IndexManifestResponse, error)

	// GetUntyped retrieves any cached manifest by digest without type assertion.
	// Returns the manifest, its type, and any error.
	// Returns ErrItemNotFound if not cached.
	GetUntyped(upstream, repo, digest string) (any, ManifestType, error)

	// PutImage stores an image manifest in the cache.
	// The manifest is keyed by upstream, repository, and digest.
	PutImage(upstream, repo, digest string, elem *data.ImageManifestResponse) error

	// PutIndex stores an index manifest in the cache.
	// The manifest is keyed by upstream, repository, and digest.
	PutIndex(upstream, repo, digest string, elem *data.IndexManifestResponse) error

	// PutTag creates a mapping from a tag to a digest.
	// This allows subsequent lookups by tag to resolve to the cached manifest.
	PutTag(upstream, repo, tag, digest string) error
}
