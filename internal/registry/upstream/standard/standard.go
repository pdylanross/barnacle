package standard

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/pdylanross/barnacle/internal/registry/data"
	"github.com/pdylanross/barnacle/internal/registry/upstream"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"go.uber.org/zap"
)

// New creates a new standard upstream registry client.
func New(
	config *configuration.UpstreamConfiguration,
	logger *zap.Logger,
	upstreamName string,
) (upstream.Upstream, error) {
	return &standardUpstream{
		config: config,
		logger: logger.Named(upstreamName),
	}, nil
}

type standardUpstream struct {
	config *configuration.UpstreamConfiguration
	logger *zap.Logger
}

// remoteOptions returns the remote options for upstream registry operations,
// including authentication if configured.
func (s *standardUpstream) remoteOptions(ctx context.Context) []remote.Option {
	opts := []remote.Option{remote.WithContext(ctx)}

	auth := s.config.Authentication
	switch {
	case auth.Basic != nil:
		opts = append(opts, remote.WithAuth(authn.FromConfig(authn.AuthConfig{
			Username: auth.Basic.Username,
			Password: auth.Basic.Password,
		})))
	case auth.Bearer != nil:
		opts = append(opts, remote.WithAuth(authn.FromConfig(authn.AuthConfig{
			RegistryToken: auth.Bearer.Token,
		})))
	default:
		// Anonymous authentication - no auth option needed
	}

	return opts
}

// buildReference constructs a reference string from registry, repo, and tag/digest.
// If the reference is a digest (e.g., "sha256:abc123..."), uses "@" separator.
// Otherwise, uses ":" separator for tags.
func (s *standardUpstream) buildReference(repo, reference string) string {
	if isDigest(reference) {
		return fmt.Sprintf("%s/%s@%s", s.config.Registry, repo, reference)
	}
	return fmt.Sprintf("%s/%s:%s", s.config.Registry, repo, reference)
}

// isDigest returns true if the reference is a digest (algorithm:hash format).
func isDigest(reference string) bool {
	// Digests are in the format "algorithm:hash" (e.g., "sha256:abc123...")
	// Common algorithms: sha256, sha384, sha512
	return strings.HasPrefix(reference, "sha256:") ||
		strings.HasPrefix(reference, "sha384:") ||
		strings.HasPrefix(reference, "sha512:")
}

// HeadManifest checks if a manifest exists and returns its metadata.
// The returned descriptor's MediaType indicates whether this is an index or image manifest.
func (s *standardUpstream) HeadManifest(
	ctx context.Context,
	repo string,
	reference string,
) (*v1.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	refStr := s.buildReference(repo, reference)
	ref, err := name.ParseReference(refStr)
	if err != nil {
		s.logger.Warn("failed to parse reference",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrNameInvalid(err)
	}

	desc, err := remote.Head(ref, s.remoteOptions(ctx)...)
	if err != nil {
		s.logger.Warn("failed to head manifest from upstream",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.TranslateTransportError(err, httptk.ErrManifestUnknown(err))
	}

	s.logger.Debug("HeadManifest success",
		zap.String("reference", refStr),
		zap.String("digest", desc.Digest.String()),
		zap.String("mediaType", string(desc.MediaType)),
		zap.Int64("size", desc.Size))

	return desc, nil
}

// IndexManifest retrieves the index manifest for the specified image reference.
//
//nolint:dupl // IndexManifest and ImageManifest have similar structure but handle different types
func (s *standardUpstream) IndexManifest(
	ctx context.Context,
	repo string,
	reference string,
) (*data.IndexManifestResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	refStr := s.buildReference(repo, reference)
	s.logger.Debug("IndexManifest: fetching index manifest",
		zap.String("reference", refStr))

	ref, err := name.ParseReference(refStr)
	if err != nil {
		s.logger.Warn("failed to parse reference",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrNameInvalid(err)
	}

	s.logger.Debug("IndexManifest: calling remote.Index()")
	idx, err := remote.Index(ref, s.remoteOptions(ctx)...)
	if err != nil {
		s.logger.Warn("failed to fetch index manifest from upstream",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.TranslateTransportError(err, httptk.ErrManifestUnknown(err))
	}

	manifest, err := idx.IndexManifest()
	if err != nil {
		s.logger.Warn("failed to parse index manifest",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrManifestInvalid(err)
	}

	rawManifest, err := idx.RawManifest()
	if err != nil {
		s.logger.Warn("failed to get raw manifest",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrManifestInvalid(err)
	}

	digest, err := idx.Digest()
	if err != nil {
		s.logger.Warn("failed to get manifest digest",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrManifestInvalid(err)
	}

	mediaType, err := idx.MediaType()
	if err != nil {
		s.logger.Warn("failed to get manifest media type",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrManifestInvalid(err)
	}

	s.logger.Debug("IndexManifest: success",
		zap.String("reference", refStr),
		zap.String("digest", digest.String()),
		zap.String("mediaType", string(mediaType)),
		zap.Int("size", len(rawManifest)))

	return &data.IndexManifestResponse{
		Manifest:    manifest,
		RawManifest: rawManifest,
		Digest:      digest,
		Size:        int64(len(rawManifest)),
		MediaType:   mediaType,
	}, nil
}

// ImageManifest retrieves the single image manifest for the specified reference.
//
//nolint:dupl // IndexManifest and ImageManifest have similar structure but handle different types
func (s *standardUpstream) ImageManifest(
	ctx context.Context,
	repo string,
	reference string,
) (*data.ImageManifestResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	refStr := s.buildReference(repo, reference)
	s.logger.Debug("ImageManifest: fetching image manifest",
		zap.String("reference", refStr))

	ref, err := name.ParseReference(refStr)
	if err != nil {
		s.logger.Warn("failed to parse reference",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrNameInvalid(err)
	}

	s.logger.Debug("ImageManifest: calling remote.Image()")
	img, err := remote.Image(ref, s.remoteOptions(ctx)...)
	if err != nil {
		s.logger.Warn("failed to fetch image manifest from upstream",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.TranslateTransportError(err, httptk.ErrManifestUnknown(err))
	}

	manifest, err := img.Manifest()
	if err != nil {
		s.logger.Warn("failed to parse image manifest",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrManifestInvalid(err)
	}

	rawManifest, err := img.RawManifest()
	if err != nil {
		s.logger.Warn("failed to get raw manifest",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrManifestInvalid(err)
	}

	digest, err := img.Digest()
	if err != nil {
		s.logger.Warn("failed to get manifest digest",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrManifestInvalid(err)
	}

	mediaType, err := img.MediaType()
	if err != nil {
		s.logger.Warn("failed to get manifest media type",
			zap.String("reference", refStr),
			zap.Error(err))
		return nil, httptk.ErrManifestInvalid(err)
	}

	s.logger.Debug("ImageManifest: success",
		zap.String("reference", refStr),
		zap.String("digest", digest.String()),
		zap.String("mediaType", string(mediaType)),
		zap.Int("size", len(rawManifest)))

	return &data.ImageManifestResponse{
		Manifest:    manifest,
		RawManifest: rawManifest,
		Digest:      digest,
		Size:        int64(len(rawManifest)),
		MediaType:   mediaType,
	}, nil
}

// HeadBlob checks if a blob exists and returns its metadata without the content.
func (s *standardUpstream) HeadBlob(
	ctx context.Context,
	repo string,
	digest v1.Hash,
) (*v1.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	repoStr := fmt.Sprintf("%s/%s", s.config.Registry, repo)
	repoRef, err := name.NewRepository(repoStr)
	if err != nil {
		s.logger.Warn("failed to parse repository",
			zap.String("repository", repoStr),
			zap.Error(err))
		return nil, httptk.ErrNameInvalid(err)
	}

	layer, err := remote.Layer(repoRef.Digest(digest.String()), s.remoteOptions(ctx)...)
	if err != nil {
		s.logger.Warn("failed to fetch layer from upstream",
			zap.String("repository", repoStr),
			zap.String("digest", digest.String()),
			zap.Error(err))
		return nil, httptk.TranslateTransportError(err, httptk.ErrBlobUnknown(err))
	}

	size, err := layer.Size()
	if err != nil {
		s.logger.Warn("failed to get layer size",
			zap.String("repository", repoStr),
			zap.String("digest", digest.String()),
			zap.Error(err))
		return nil, httptk.ErrBlobUnknown(err)
	}

	layerDigest, err := layer.Digest()
	if err != nil {
		s.logger.Warn("failed to get layer digest",
			zap.String("repository", repoStr),
			zap.String("digest", digest.String()),
			zap.Error(err))
		return nil, httptk.ErrBlobUnknown(err)
	}

	mediaType, err := layer.MediaType()
	if err != nil {
		s.logger.Warn("failed to get layer media type",
			zap.String("repository", repoStr),
			zap.String("digest", digest.String()),
			zap.Error(err))
		return nil, httptk.ErrBlobUnknown(err)
	}

	s.logger.Debug("HeadBlob success",
		zap.String("repository", repoStr),
		zap.String("digest", layerDigest.String()),
		zap.Int64("size", size),
		zap.String("mediaType", string(mediaType)))

	return &v1.Descriptor{
		Digest:    layerDigest,
		Size:      size,
		MediaType: mediaType,
	}, nil
}

// GetBlob retrieves the blob content for the specified digest.
// The caller is responsible for closing the returned ReadCloser.
func (s *standardUpstream) GetBlob(
	ctx context.Context,
	repo string,
	digest v1.Hash,
) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	repoStr := fmt.Sprintf("%s/%s", s.config.Registry, repo)
	repoRef, err := name.NewRepository(repoStr)
	if err != nil {
		s.logger.Warn("failed to parse repository",
			zap.String("repository", repoStr),
			zap.Error(err))
		return nil, httptk.ErrNameInvalid(err)
	}

	layer, err := remote.Layer(repoRef.Digest(digest.String()), s.remoteOptions(ctx)...)
	if err != nil {
		s.logger.Warn("failed to fetch layer from upstream",
			zap.String("repository", repoStr),
			zap.String("digest", digest.String()),
			zap.Error(err))
		return nil, httptk.TranslateTransportError(err, httptk.ErrBlobUnknown(err))
	}

	// Use Compressed() to get the raw blob content
	rc, err := layer.Compressed()
	if err != nil {
		s.logger.Warn("failed to get compressed blob content",
			zap.String("repository", repoStr),
			zap.String("digest", digest.String()),
			zap.Error(err))
		return nil, httptk.ErrBlobUnknown(err)
	}

	return rc, nil
}
