package blobsapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	blobdtos "github.com/pdylanross/barnacle/pkg/api/blobsapi"
)

const accessCountWindow = 5 * time.Minute

// RegisterV1 registers the v1 node blobs API routes on the provided router group.
// The group should be mounted at /api/v1/nodes/:nodeId/blobs.
// It creates a new controller instance and binds the following endpoints:
//   - GET / - List all blobs on this node
//   - GET /:digest - Get a single blob's info
//   - DELETE /:digest - Delete a blob from this node
func RegisterV1(group *gin.RouterGroup, deps *dependencies.Dependencies) {
	controller := newControllerV1(deps)
	group.GET("/", controller.List)
	group.GET("/:digest", controller.Get)
	group.DELETE("/:digest", controller.Delete)
}

func newControllerV1(deps *dependencies.Dependencies) *blobsControllerV1 {
	return &blobsControllerV1{
		blobCache:    deps.UpstreamRegistry().BlobCache(),
		nodeRegistry: deps.NodeRegistry(),
	}
}

type blobsControllerV1 struct {
	blobCache    coordinator.Cache
	nodeRegistry *node.Registry
}

// validateNodeID checks that the requested nodeId matches this node.
// Returns true if valid, false if it wrote a 404 error response.
func (c *blobsControllerV1) validateNodeID(ctx *gin.Context) bool {
	nodeID := ctx.Param("nodeId")
	if nodeID != c.nodeRegistry.NodeID() {
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusNotFound,
			"NODE_NOT_FOUND",
			"node not found",
			nodeID,
		))
		return false
	}
	return true
}

// List handles GET / requests and returns all blobs cached on this node.
//
// @Summary      List blobs
// @Description  Returns all blobs cached on the specified node
// @Tags         blobs
// @Produce      json
// @Param        nodeId  path      string  true  "Node identifier"
// @Success      200     {object}  blobsapi.ListBlobsResponse
// @Failure      404     {object}  httptk.ErrorsList
// @Router       /api/v1/nodes/{nodeId}/blobs [get].
func (c *blobsControllerV1) List(ctx *gin.Context) {
	if !c.validateNodeID(ctx) {
		return
	}

	blobs, err := c.blobCache.ListLocal(ctx.Request.Context())
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	resp := blobdtos.ListBlobsResponse{
		Blobs: make([]blobdtos.BlobResponse, len(blobs)),
	}

	for i, b := range blobs {
		accessCount, _ := c.blobCache.GetAccessCountWindow(ctx.Request.Context(), b.Digest, accessCountWindow)
		resp.Blobs[i] = blobdtos.BlobResponse{
			Digest:        b.Digest,
			Size:          b.Size,
			MediaType:     b.MediaType,
			DiskPath:      b.Path,
			Tier:          b.Tier,
			AccessCount5m: accessCount,
		}
	}

	ctx.JSON(http.StatusOK, resp)
}

// Get handles GET /:digest requests and returns info for a single blob.
//
// @Summary      Get blob
// @Description  Returns information about a specific blob on the node
// @Tags         blobs
// @Produce      json
// @Param        nodeId  path      string  true  "Node identifier"
// @Param        digest  path      string  true  "Blob digest"
// @Success      200     {object}  blobsapi.BlobResponse
// @Failure      404     {object}  httptk.ErrorsList
// @Router       /api/v1/nodes/{nodeId}/blobs/{digest} [get].
func (c *blobsControllerV1) Get(ctx *gin.Context) {
	if !c.validateNodeID(ctx) {
		return
	}

	digest := ctx.Param("digest")

	blobs, err := c.blobCache.ListLocal(ctx.Request.Context())
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	for _, b := range blobs {
		if b.Digest == digest {
			accessCount, _ := c.blobCache.GetAccessCountWindow(ctx.Request.Context(), b.Digest, accessCountWindow)
			ctx.JSON(http.StatusOK, blobdtos.BlobResponse{
				Digest:        b.Digest,
				Size:          b.Size,
				MediaType:     b.MediaType,
				DiskPath:      b.Path,
				Tier:          b.Tier,
				AccessCount5m: accessCount,
			})
			return
		}
	}

	_ = ctx.Error(httptk.ErrBlobUnknown(digest))
}

// Delete handles DELETE /:digest requests and removes a blob from this node.
//
// @Summary      Delete blob
// @Description  Deletes a specific blob from the node's cache
// @Tags         blobs
// @Produce      json
// @Param        nodeId  path      string  true  "Node identifier"
// @Param        digest  path      string  true  "Blob digest"
// @Success      204     "No Content"
// @Failure      404     {object}  httptk.ErrorsList
// @Router       /api/v1/nodes/{nodeId}/blobs/{digest} [delete].
func (c *blobsControllerV1) Delete(ctx *gin.Context) {
	if !c.validateNodeID(ctx) {
		return
	}

	digest := ctx.Param("digest")

	if err := c.blobCache.DeleteLocalOnly(ctx.Request.Context(), digest); err != nil {
		_ = ctx.Error(err)
		return
	}

	ctx.Status(http.StatusNoContent)
}
