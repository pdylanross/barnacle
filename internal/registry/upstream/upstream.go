package upstream

import (
	"context"
	"io"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pdylanross/barnacle/internal/registry/data"
)

// Upstream defines the interface for interacting with upstream container registries.
// Methods that return metadata use v1.Descriptor from go-containerregistry, which
// provides Digest, Size, and MediaType fields as defined by the OCI distribution spec.
type Upstream interface {
	// HeadManifest checks if a manifest exists and returns its metadata.
	// Returns a v1.Descriptor containing the manifest's digest, size, and media type.
	// The caller should use the MediaType field to determine if this is an index or image manifest.
	HeadManifest(ctx context.Context, repo string, reference string) (*v1.Descriptor, error)

	// IndexManifest retrieves the image index manifest for the specified repository and reference.
	// Returns the manifest content along with its digest, size, and media type.
	IndexManifest(ctx context.Context, repo string, reference string) (*data.IndexManifestResponse, error)

	// ImageManifest retrieves the single image manifest for the specified repository and reference.
	// Returns the manifest content along with its digest, size, and media type.
	ImageManifest(ctx context.Context, repo string, reference string) (*data.ImageManifestResponse, error)

	// HeadBlob checks if a blob exists and returns its metadata without the content.
	// Returns a v1.Descriptor containing the blob's digest and size.
	HeadBlob(ctx context.Context, repo string, digest v1.Hash) (*v1.Descriptor, error)

	// GetBlob retrieves the blob content for the specified digest.
	// The caller is responsible for closing the returned ReadCloser.
	GetBlob(ctx context.Context, repo string, digest v1.Hash) (io.ReadCloser, error)
}
