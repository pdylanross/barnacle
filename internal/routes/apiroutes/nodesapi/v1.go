package nodesapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	nodedtos "github.com/pdylanross/barnacle/pkg/api/nodesapi"
)

// RegisterV1 registers the v1 nodes API routes on the provided router group.
// It creates a new controller instance and binds the following endpoints:
//   - GET / - List all registered nodes
//   - GET /me - Get this node's info
//   - GET /:nodeId - Get a specific node's info by ID
func RegisterV1(group *gin.RouterGroup, deps *dependencies.Dependencies) {
	controller := newControllerV1(deps)
	group.GET("/", controller.List)
	group.GET("/me", controller.Me)
	group.GET("/:nodeId", controller.Get)
}

// newControllerV1 creates a new v1 nodes API controller with the provided dependencies.
func newControllerV1(deps *dependencies.Dependencies) *nodesControllerV1 {
	return &nodesControllerV1{
		registry: deps.NodeRegistry(),
	}
}

// nodesControllerV1 handles HTTP requests for the v1 nodes API.
type nodesControllerV1 struct {
	registry *node.Registry
}

// List handles GET / requests and returns a list of all registered nodes.
// Returns [http.StatusOK] with a [nodedtos.ListNodesResponse].
//
// @Summary      List nodes
// @Description  Returns a list of all registered nodes in the cluster
// @Tags         nodes
// @Produce      json
// @Success      200  {object}  nodesapi.ListNodesResponse
// @Router       /api/v1/nodes [get].
func (c *nodesControllerV1) List(ctx *gin.Context) {
	nodes, err := c.registry.ListNodes(ctx.Request.Context())
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	resp := nodedtos.ListNodesResponse{
		Nodes: make([]nodedtos.NodeResponse, len(nodes)),
	}
	for i, n := range nodes {
		resp.Nodes[i] = mapNodeInfo(n)
	}

	ctx.JSON(http.StatusOK, resp)
}

// Me handles GET /me requests and returns this node's info.
// Returns [http.StatusOK] with a [nodedtos.NodeResponse].
//
// @Summary      Get current node
// @Description  Returns information about this node
// @Tags         nodes
// @Produce      json
// @Success      200  {object}  nodesapi.NodeResponse
// @Router       /api/v1/nodes/me [get].
func (c *nodesControllerV1) Me(ctx *gin.Context) {
	info := c.registry.GetNodeInfo()
	ctx.JSON(http.StatusOK, mapNodeInfo(info))
}

// Get handles GET /:nodeId requests and returns a specific node's info.
// Returns [http.StatusOK] with a [nodedtos.NodeResponse] if found.
// Returns [http.StatusNotFound] if the node ID does not exist.
//
// @Summary      Get node
// @Description  Returns information about a specific node
// @Tags         nodes
// @Produce      json
// @Param        nodeId  path      string  true  "Node identifier"
// @Success      200     {object}  nodesapi.NodeResponse
// @Failure      404     {object}  httptk.ErrorsList
// @Router       /api/v1/nodes/{nodeId} [get].
func (c *nodesControllerV1) Get(ctx *gin.Context) {
	nodeID := ctx.Param("nodeId")

	info, err := c.registry.GetNode(ctx.Request.Context(), nodeID)
	if err != nil {
		if errors.Is(err, node.ErrNodeNotFound) {
			_ = ctx.Error(httptk.NewHTTPError(http.StatusNotFound, "NODE_NOT_FOUND", "node not found", nodeID))
			return
		}
		_ = ctx.Error(err)
		return
	}

	ctx.JSON(http.StatusOK, mapNodeInfo(info))
}

// mapNodeInfo converts a node.Info to a NodeResponse DTO.
func mapNodeInfo(info *node.Info) nodedtos.NodeResponse {
	tierUsage := make([]nodedtos.DiskUsageResponse, len(info.Stats.TierDiskUsage))
	for i, du := range info.Stats.TierDiskUsage {
		tierUsage[i] = nodedtos.DiskUsageResponse{
			Path:       du.Path,
			TotalBytes: du.TotalBytes,
			FreeBytes:  du.FreeBytes,
			UsedBytes:  du.UsedBytes,
		}
	}

	return nodedtos.NodeResponse{
		NodeID:      info.NodeID,
		Status:      string(info.Status),
		LastUpdated: info.LastUpdated,
		Stats: nodedtos.StatsResponse{
			TierDiskUsage: tierUsage,
		},
	}
}
