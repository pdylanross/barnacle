package registry

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/registry"
	"github.com/pdylanross/barnacle/internal/registry/upstream"
	"github.com/pdylanross/barnacle/internal/tk"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	"go.uber.org/zap"
)

// OCI Distribution Spec headers.
const (
	HeaderDockerDistributionAPIVersion = "Docker-Distribution-API-Version"
	HeaderDockerContentDigest          = "Docker-Content-Digest"
	HeaderContentType                  = "Content-Type"
	HeaderContentLength                = "Content-Length"
	HeaderAccept                       = "Accept"

	// DistributionAPIVersion is the API version returned by the registry.
	DistributionAPIVersion = "registry/2.0"
)

type registryController struct {
	upstream *registry.UpstreamRegistry
	logger   *zap.Logger
}

func newRegistryController(deps *dependencies.Dependencies) *registryController {
	return &registryController{
		upstream: deps.UpstreamRegistry(),
		logger:   deps.Logger().Named("distribution"),
	}
}

func RegisterDistribution(group *gin.RouterGroup, deps *dependencies.Dependencies) {
	controller := newRegistryController(deps)

	group.GET("/v2/", controller.APIVersionCheck)
	// Use wildcard path to capture repositories with slashes (e.g., "library/nginx")
	// The path format is: <repository>/manifests/<reference> or <repository>/blobs/<digest>
	group.HEAD("/v2/:upstream/*path", controller.handlePath)
	group.GET("/v2/:upstream/*path", controller.handlePath)
}

// handlePath routes requests based on the path structure.
// It parses paths like "my/nested/repo/manifests/latest" or "repo/blobs/sha256:abc".
func (c *registryController) handlePath(ctx *gin.Context) {
	path := ctx.Param("path")
	// Remove leading slash from wildcard capture
	path = strings.TrimPrefix(path, "/")

	// Parse the path to find repository and action
	repo, action, param, err := parsePath(path)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Store parsed values for handlers to use
	ctx.Set("repository", repo)

	switch action {
	case "manifests":
		ctx.Set("reference", param)
		if ctx.Request.Method == http.MethodHead {
			c.HeadManifest(ctx)
		} else {
			c.GetManifest(ctx)
		}
	case "blobs":
		ctx.Set("digest", param)
		if ctx.Request.Method == http.MethodHead {
			c.HeadBlob(ctx)
		} else {
			c.GetBlob(ctx)
		}
	default:
		_ = ctx.Error(httptk.NewHTTPError(http.StatusNotFound, "NAME_UNKNOWN", "not found", nil))
	}
}

// parsePath extracts repository, action (manifests/blobs), and parameter from the path.
// Example: "library/nginx/manifests/latest" returns ("library/nginx", "manifests", "latest", nil).
func parsePath(path string) (string, string, string, error) {
	// Find /manifests/ or /blobs/ in the path
	for _, act := range []string{"/manifests/", "/blobs/"} {
		idx := strings.LastIndex(path, act)
		if idx != -1 {
			repo := path[:idx]
			remaining := path[idx+len(act):]
			// The parameter is everything after the action
			// For blobs, this includes the full digest (sha256:abc...)
			if remaining == "" {
				return "", "", "", httptk.NewHTTPError(
					http.StatusNotFound,
					"NAME_UNKNOWN",
					"missing reference or digest",
					nil,
				)
			}
			return repo, strings.Trim(act, "/"), remaining, nil
		}
	}
	return "", "", "", httptk.NewHTTPError(http.StatusNotFound, "NAME_UNKNOWN", "invalid path", nil)
}

// APIVersionCheck handles the /v2/ endpoint to verify API version support.
// Returns 200 OK with Docker-Distribution-API-Version header per OCI spec.
func (c *registryController) APIVersionCheck(ctx *gin.Context) {
	ctx.Header(HeaderDockerDistributionAPIVersion, DistributionAPIVersion)
	ctx.JSON(http.StatusOK, struct{}{})
}

// HeadManifest handles HEAD requests to check if a manifest exists.
// Returns 200 OK with required headers if the manifest exists.
func (c *registryController) HeadManifest(ctx *gin.Context) {
	up := c.getUpstream(ctx)
	if up == nil {
		return
	}

	repo := c.getRepository(ctx)
	reference := c.getReference(ctx)

	c.logger.Debug("HEAD manifest request",
		zap.String("repo", repo),
		zap.String("reference", reference))

	info, err := up.HeadManifest(ctx, repo, reference)
	if err != nil {
		c.logger.Debug("HEAD manifest failed", zap.Error(err))
		_ = ctx.Error(err)
		return
	}

	c.logger.Debug("HEAD manifest success",
		zap.String("digest", info.Digest.String()),
		zap.String("mediaType", string(info.MediaType)),
		zap.Int64("size", info.Size))

	ctx.Header(HeaderDockerDistributionAPIVersion, DistributionAPIVersion)
	ctx.Header(HeaderDockerContentDigest, info.Digest.String())
	ctx.Header(HeaderContentType, string(info.MediaType))
	ctx.Header(HeaderContentLength, strconv.FormatInt(info.Size, 10))
	ctx.Status(http.StatusOK)
}

// GetManifest handles GET requests to retrieve a manifest.
// Returns 200 OK with the manifest content and required headers.
// For digest references, uses the actual content-type from the upstream.
// For tag references, respects the Accept header to determine manifest type.
func (c *registryController) GetManifest(ctx *gin.Context) {
	up := c.getUpstream(ctx)
	if up == nil {
		return
	}

	repo := c.getRepository(ctx)
	reference := c.getReference(ctx)
	accept := ctx.GetHeader(HeaderAccept)

	c.logger.Debug("GET manifest request",
		zap.String("repo", repo),
		zap.String("reference", reference),
		zap.String("accept", accept),
		zap.Bool("isDigest", isDigest(reference)))

	// First, call HEAD to get the actual content-type of the manifest
	headInfo, err := up.HeadManifest(ctx, repo, reference)
	if err != nil {
		c.logger.Debug("HEAD manifest failed during GET", zap.Error(err))
		_ = ctx.Error(err)
		return
	}

	c.logger.Debug("HEAD manifest result",
		zap.String("actualMediaType", string(headInfo.MediaType)),
		zap.String("digest", headInfo.Digest.String()))

	// Determine whether to fetch as image or index manifest:
	// - For digest references: use the actual content-type from HEAD
	// - For tag references: prefer Accept header over actual content-type
	var useImageManifest bool
	if isDigest(reference) {
		// Digest reference: use actual content-type
		useImageManifest = isImageManifestMediaType(headInfo.MediaType)
		c.logger.Debug("digest reference: using actual content-type",
			zap.Bool("useImageManifest", useImageManifest))
	} else {
		// Tag reference: prefer Accept header
		useImageManifest = c.acceptsImageManifest(ctx)
		c.logger.Debug("tag reference: using Accept header preference",
			zap.Bool("useImageManifest", useImageManifest))
	}

	var digest v1.Hash
	var mediaType types.MediaType
	var rawManifest []byte
	var manifestType string

	if useImageManifest {
		manifestType = "image"
		c.logger.Debug("fetching image manifest")
		manifest, fetchErr := up.ImageManifest(ctx, repo, reference)
		if fetchErr != nil {
			c.logger.Debug("GET image manifest failed", zap.Error(fetchErr))
			_ = ctx.Error(fetchErr)
			return
		}
		digest = manifest.Digest
		mediaType = manifest.MediaType
		rawManifest = manifest.RawManifest
	} else {
		manifestType = "index"
		c.logger.Debug("fetching index manifest")
		manifest, fetchErr := up.IndexManifest(ctx, repo, reference)
		if fetchErr != nil {
			c.logger.Debug("GET index manifest failed", zap.Error(fetchErr))
			_ = ctx.Error(fetchErr)
			return
		}
		digest = manifest.Digest
		mediaType = manifest.MediaType
		rawManifest = manifest.RawManifest
	}

	c.logger.Debug("GET manifest success",
		zap.String("manifestType", manifestType),
		zap.String("digest", digest.String()),
		zap.String("mediaType", string(mediaType)),
		zap.Int("size", len(rawManifest)))

	ctx.Header(HeaderDockerDistributionAPIVersion, DistributionAPIVersion)
	ctx.Header(HeaderDockerContentDigest, digest.String())
	ctx.Data(http.StatusOK, string(mediaType), rawManifest)
}

// acceptsImageManifest checks if the Accept header indicates a single image manifest is preferred.
// Returns true if the Accept header contains image manifest media types.
// Returns false (default to index) if no Accept header or if index media types are present.
func (c *registryController) acceptsImageManifest(ctx *gin.Context) bool {
	accept := ctx.GetHeader(HeaderAccept)
	if accept == "" {
		c.logger.Debug("no Accept header, defaulting to index manifest")
		return false // Default to index manifest
	}

	// Check if any image manifest media types are accepted
	// OCI image manifest or Docker manifest schema v2 indicate single image
	hasOCIManifest := strings.Contains(accept, string(types.OCIManifestSchema1))
	hasDockerV2Manifest := strings.Contains(accept, string(types.DockerManifestSchema2))
	hasDockerV1Manifest := strings.Contains(accept, string(types.DockerManifestSchema1))

	return hasOCIManifest || hasDockerV2Manifest || hasDockerV1Manifest
}

// isDigest returns true if the reference is a digest (algorithm:hash format).
func isDigest(reference string) bool {
	return strings.HasPrefix(reference, "sha256:") ||
		strings.HasPrefix(reference, "sha384:") ||
		strings.HasPrefix(reference, "sha512:")
}

// isImageManifestMediaType returns true if the media type indicates a single image manifest.
func isImageManifestMediaType(mediaType types.MediaType) bool {
	return mediaType == types.OCIManifestSchema1 ||
		mediaType == types.DockerManifestSchema2 ||
		mediaType == types.DockerManifestSchema1
}

// HeadBlob handles HEAD requests to check if a blob exists.
// Returns 200 OK with Content-Length and Docker-Content-Digest headers if the blob exists.
// Returns 404 Not Found if the blob does not exist.
func (c *registryController) HeadBlob(ctx *gin.Context) {
	up := c.getUpstream(ctx)
	if up == nil {
		return
	}

	digest, err := c.parseDigest(ctx)
	if err != nil {
		return
	}

	repo := c.getRepository(ctx)
	blobInfo, err := up.HeadBlob(ctx, repo, digest)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	ctx.Header(HeaderDockerDistributionAPIVersion, DistributionAPIVersion)
	ctx.Header(HeaderContentLength, strconv.FormatInt(blobInfo.Size, 10))
	ctx.Header(HeaderDockerContentDigest, blobInfo.Digest.String())
	ctx.Status(http.StatusOK)
}

// GetBlob handles GET requests to retrieve blob content.
// Returns 200 OK with the blob content and appropriate headers.
// Returns 404 Not Found if the blob does not exist.
func (c *registryController) GetBlob(ctx *gin.Context) {
	up := c.getUpstream(ctx)
	if up == nil {
		return
	}

	digest, err := c.parseDigest(ctx)
	if err != nil {
		return
	}

	repo := c.getRepository(ctx)

	// First get blob info for headers
	blobInfo, err := up.HeadBlob(ctx, repo, digest)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Then get the blob content
	reader, err := up.GetBlob(ctx, repo, digest)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	defer tk.IgnoreDeferError(reader.Close)

	ctx.Header(HeaderDockerDistributionAPIVersion, DistributionAPIVersion)
	ctx.Header(HeaderDockerContentDigest, blobInfo.Digest.String())
	ctx.DataFromReader(http.StatusOK, blobInfo.Size, "application/octet-stream", reader, nil)
}

// parseDigest extracts and validates the digest parameter from the request.
func (c *registryController) parseDigest(ctx *gin.Context) (v1.Hash, error) {
	digestVal, ok := ctx.Get("digest")
	if !ok {
		err := httptk.ErrDigestInvalid(nil)
		_ = ctx.Error(err)
		return v1.Hash{}, err
	}
	digestStr, ok := digestVal.(string)
	if !ok {
		err := httptk.ErrDigestInvalid(nil)
		_ = ctx.Error(err)
		return v1.Hash{}, err
	}
	digest, err := v1.NewHash(digestStr)
	if err != nil {
		_ = ctx.Error(httptk.ErrDigestInvalid(err))
		return v1.Hash{}, err
	}
	return digest, nil
}

// getRepository retrieves the repository from the context.
func (c *registryController) getRepository(ctx *gin.Context) string {
	repo, ok := ctx.Get("repository")
	if !ok {
		return ""
	}
	repoStr, ok := repo.(string)
	if !ok {
		return ""
	}
	return repoStr
}

// getReference retrieves the reference (tag or digest) from the context.
func (c *registryController) getReference(ctx *gin.Context) string {
	ref, ok := ctx.Get("reference")
	if !ok {
		return ""
	}
	refStr, ok := ref.(string)
	if !ok {
		return ""
	}
	return refStr
}

func (c *registryController) getUpstream(ctx *gin.Context) upstream.Upstream {
	up, err := c.upstream.GetUpstream(ctx.Param("upstream"))
	if err != nil {
		_ = ctx.Error(err)
		return nil
	}

	return up
}
