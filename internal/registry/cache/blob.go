package cache

import (
	"context"
	"errors"
	"io"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// Blob cache errors.
var (
	// ErrBlobNotFound is returned when a blob lookup fails because the blob is not cached.
	ErrBlobNotFound = errors.New("blob not found")
)

// BlobInfo contains metadata about a cached blob.
type BlobInfo struct {
	// Digest is the content-addressable digest of the blob (e.g., "sha256:abc123...").
	Digest string
	// Size is the size of the blob in bytes.
	Size int64
	// MediaType is the OCI media type of the blob.
	MediaType string
	// Path is the filesystem path where the blob is stored.
	Path string
}

// BlobCache defines the interface for caching OCI blobs.
// Implementations must be safe for concurrent access.
//
// Blobs are large binary data (typically container image layers) that are
// content-addressable by their digest. Since blobs are uniquely identified
// by their digest, the same blob can be shared across different upstreams
// and repositories. The upstream and repo parameters are used for routing
// purposes (e.g., building redirect URLs) but do not affect storage.
//
// Each cached blob stores both the content and its OCI descriptor metadata.
// The descriptor contains digest, size, and media type information.
type BlobCache interface {
	// Head returns the descriptor for a cached blob without reading its content.
	// The upstream and repo parameters are used for routing (e.g., redirect URLs).
	// Returns ErrBlobNotFound if the blob is not cached.
	Head(ctx context.Context, upstream, repo, digest string) (*v1.Descriptor, error)

	// Get retrieves a cached blob by digest.
	// The upstream and repo parameters are used for routing (e.g., redirect URLs).
	// Returns an [io.ReadCloser] for streaming the blob content.
	// The caller is responsible for closing the reader.
	// Returns ErrBlobNotFound if the blob is not cached.
	Get(ctx context.Context, upstream, repo, digest string) (io.ReadCloser, error)

	// Put stores a blob and its descriptor in the cache.
	// The upstream and repo parameters are used for routing (e.g., redirect URLs).
	// The content is read from the provided [io.Reader] until EOF.
	// The descriptor contains the blob's digest, size, and media type.
	Put(ctx context.Context, upstream, repo, digest string, descriptor *v1.Descriptor, content io.Reader) error

	// Delete removes a blob from the cache.
	// The upstream and repo parameters are used for routing (e.g., redirect URLs).
	// Returns nil if the blob was deleted or did not exist.
	Delete(ctx context.Context, upstream, repo, digest string) error

	// List returns information about all blobs in the cache.
	List(ctx context.Context) ([]BlobInfo, error)
}
