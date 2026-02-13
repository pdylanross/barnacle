package mocks

import (
	"bytes"
	"context"
	"io"
	"sync"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pdylanross/barnacle/internal/registry/cache"
)

// BlobCache is a mock implementation of cache.BlobCache for testing.
// It stores blobs in memory and is safe for concurrent access.
type BlobCache struct {
	mu          sync.RWMutex
	blobs       map[string][]byte
	descriptors map[string]*v1.Descriptor
}

// NewBlobCache creates a new mock blob cache.
func NewBlobCache() *BlobCache {
	return &BlobCache{
		blobs:       make(map[string][]byte),
		descriptors: make(map[string]*v1.Descriptor),
	}
}

// Head returns the descriptor for a cached blob.
// The upstream and repo parameters are ignored (blobs are content-addressable).
func (m *BlobCache) Head(ctx context.Context, _, _, digest string) (*v1.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	desc, ok := m.descriptors[digest]
	if !ok {
		return nil, cache.ErrBlobNotFound
	}
	return desc, nil
}

// Get retrieves a cached blob by digest.
// The upstream and repo parameters are ignored (blobs are content-addressable).
func (m *BlobCache) Get(ctx context.Context, _, _, digest string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.blobs[digest]
	if !ok {
		return nil, cache.ErrBlobNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Put stores a blob and its descriptor in the cache.
// The upstream and repo parameters are ignored (blobs are content-addressable).
func (m *BlobCache) Put(ctx context.Context, _, _, digest string, descriptor *v1.Descriptor, content io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.blobs[digest] = data
	m.descriptors[digest] = descriptor
	return nil
}

// Delete removes a blob from the cache.
// The upstream and repo parameters are ignored (blobs are content-addressable).
func (m *BlobCache) Delete(ctx context.Context, _, _, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.blobs, digest)
	delete(m.descriptors, digest)
	return nil
}

// List returns information about all blobs in the mock cache.
func (m *BlobCache) List(ctx context.Context) ([]cache.BlobInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	blobs := make([]cache.BlobInfo, 0, len(m.descriptors))
	for digest, desc := range m.descriptors {
		blobs = append(blobs, cache.BlobInfo{
			Digest:    digest,
			Size:      desc.Size,
			MediaType: string(desc.MediaType),
			Path:      "/mock/" + digest,
		})
	}
	return blobs, nil
}

// Verify interface compliance.
var _ cache.BlobCache = (*BlobCache)(nil)
