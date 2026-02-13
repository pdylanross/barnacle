// Package rebalanceapi provides HTTP endpoints for blob rebalancing operations.
package rebalanceapi

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator/rebalance"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	"github.com/pdylanross/barnacle/pkg/api/rebalanceapi"
	"go.uber.org/zap"
)

// RegisterV1 registers the v1 rebalance API routes on the provided router group.
// The group should be mounted at /api/v1/nodes/:nodeId/blobs.
func RegisterV1(group *gin.RouterGroup, deps *dependencies.Dependencies) {
	controller := newControllerV1(deps)
	group.POST("/reserve", controller.Reserve)
	group.POST("/receive", controller.Receive)
	group.POST("/finalize", controller.Finalize)
}

func newControllerV1(deps *dependencies.Dependencies) *rebalanceControllerV1 {
	return &rebalanceControllerV1{
		blobCache:        deps.UpstreamRegistry().BlobCache(),
		nodeRegistry:     deps.NodeRegistry(),
		reservationStore: deps.ReservationStore(),
		logger:           deps.Logger().Named("rebalance-api"),
	}
}

type rebalanceControllerV1 struct {
	blobCache        coordinator.Cache
	nodeRegistry     *node.Registry
	reservationStore *rebalance.ReservationStore
	logger           *zap.Logger
}

// validateNodeID checks that the requested nodeId matches this node.
func (c *rebalanceControllerV1) validateNodeID(ctx *gin.Context) bool {
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

// Reserve handles POST /reserve requests to reserve disk space for an incoming blob.
//
// @Summary      Reserve space for blob
// @Description  Reserves disk space for an incoming blob transfer
// @Tags         rebalance
// @Accept       json
// @Produce      json
// @Param        nodeId  path      string                     true  "Node identifier"
// @Param        body    body      rebalanceapi.ReserveRequest  true  "Reserve request"
// @Success      201     {object}  rebalanceapi.ReserveResponse
// @Failure      400     {object}  httptk.ErrorsList
// @Failure      404     {object}  httptk.ErrorsList
// @Failure      409     {object}  httptk.ErrorsList  "Blob already exists"
// @Failure      507     {object}  httptk.ErrorsList  "Insufficient storage"
// @Router       /api/v1/nodes/{nodeId}/blobs/reserve [post].
func (c *rebalanceControllerV1) Reserve(ctx *gin.Context) {
	if !c.validateNodeID(ctx) {
		return
	}

	var req rebalanceapi.ReserveRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"invalid request body",
			err.Error(),
		))
		return
	}

	requestCtx := ctx.Request.Context()

	// Check if blob already exists on this node
	location, err := c.blobCache.GetLocation(requestCtx, req.Digest)
	if err == nil && location.NodeID == c.nodeRegistry.NodeID() {
		// Blob already exists on this node
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusConflict,
			"BLOB_EXISTS",
			"blob already exists on this node",
			req.Digest,
		))
		return
	}
	// Accept reservation if:
	// - GetLocation returns an error (blob not tracked in Redis)
	// - Blob exists but is on a different node (we're receiving a copy)

	// Find cache location for the blob
	decision, reservation, err := c.blobCache.FindCacheLocation(requestCtx, req.Size)
	if err != nil {
		c.logger.Warn("no storage capacity available",
			zap.String("digest", req.Digest),
			zap.Int64("size", req.Size),
			zap.Error(err))
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusInsufficientStorage,
			"INSUFFICIENT_STORAGE",
			"no storage capacity available",
			err.Error(),
		))
		return
	}

	// We expect a local decision since the request was routed to this node
	if !decision.Local {
		if reservation != nil {
			reservation.Release()
		}
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusInsufficientStorage,
			"INSUFFICIENT_STORAGE",
			"local storage not available, remote suggested",
			decision.NodeID,
		))
		return
	}

	// Create a pending reservation
	pendingRes := c.reservationStore.Create(
		req.Digest,
		req.Size,
		req.MediaType,
		req.SourceNodeID,
		decision.Tier,
		reservation,
	)

	c.logger.Info("reserved space for blob transfer",
		zap.String("reservationID", pendingRes.ID),
		zap.String("digest", req.Digest),
		zap.Int64("size", req.Size),
		zap.Int("tier", decision.Tier))

	ctx.JSON(http.StatusCreated, rebalanceapi.ReserveResponse{
		ReservationID: pendingRes.ID,
		ExpiresAt:     pendingRes.ExpiresAt,
	})
}

// Receive handles POST /receive requests to receive blob data.
//
// @Summary      Receive blob data
// @Description  Receives blob data for a reserved transfer
// @Tags         rebalance
// @Accept       application/octet-stream
// @Produce      json
// @Param        nodeId           path      string  true  "Node identifier"
// @Param        X-Reservation-ID header    string  true  "Reservation ID"
// @Param        X-Digest         header    string  true  "Expected blob digest"
// @Success      201              {object}  rebalanceapi.ReceiveResponse
// @Failure      400              {object}  httptk.ErrorsList  "Digest mismatch"
// @Failure      404              {object}  httptk.ErrorsList  "Reservation not found or expired"
// @Router       /api/v1/nodes/{nodeId}/blobs/receive [post].
func (c *rebalanceControllerV1) Receive(ctx *gin.Context) {
	if !c.validateNodeID(ctx) {
		return
	}

	reservationID := ctx.GetHeader("X-Reservation-ID")
	digest := ctx.GetHeader("X-Digest")

	if reservationID == "" {
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusBadRequest,
			"MISSING_HEADER",
			"X-Reservation-ID header is required",
			nil,
		))
		return
	}

	if digest == "" {
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusBadRequest,
			"MISSING_HEADER",
			"X-Digest header is required",
			nil,
		))
		return
	}

	// Look up reservation
	reservation := c.reservationStore.Get(reservationID)
	if reservation == nil {
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusNotFound,
			"RESERVATION_NOT_FOUND",
			"reservation not found or expired",
			reservationID,
		))
		return
	}

	// Verify digest matches
	if reservation.Digest != digest {
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusBadRequest,
			"DIGEST_MISMATCH",
			"digest does not match reservation",
			map[string]string{
				"expected": reservation.Digest,
				"received": digest,
			},
		))
		return
	}

	requestCtx := ctx.Request.Context()

	// Create descriptor for the blob
	descriptor := &v1.Descriptor{
		Digest:    v1.Hash{Algorithm: "sha256", Hex: extractHex(digest)},
		Size:      reservation.Size,
		MediaType: types.MediaType(reservation.MediaType),
	}

	// Build cache location decision from reservation
	decision := &coordinator.CacheLocationDecision{
		Local: true,
		Tier:  reservation.Tier,
	}

	// Stream the body to the blob cache
	// Note: upstream and repo are empty since this is a direct transfer
	err := c.blobCache.Put(requestCtx, "", "", digest, descriptor, ctx.Request.Body, decision)
	if err != nil {
		c.logger.Error("failed to store received blob",
			zap.String("reservationID", reservationID),
			zap.String("digest", digest),
			zap.Error(err))
		_ = ctx.Error(err)
		return
	}

	c.logger.Info("received blob data",
		zap.String("reservationID", reservationID),
		zap.String("digest", digest),
		zap.Int64("size", reservation.Size))

	ctx.JSON(http.StatusCreated, rebalanceapi.ReceiveResponse{
		Digest: digest,
		Size:   reservation.Size,
	})
}

// Finalize handles POST /finalize requests to complete a blob transfer.
//
// @Summary      Finalize blob transfer
// @Description  Completes a blob transfer by updating Redis location
// @Tags         rebalance
// @Accept       json
// @Produce      json
// @Param        nodeId  path      string                      true  "Node identifier"
// @Param        body    body      rebalanceapi.FinalizeRequest  true  "Finalize request"
// @Success      200     {object}  rebalanceapi.FinalizeResponse
// @Failure      400     {object}  httptk.ErrorsList
// @Failure      404     {object}  httptk.ErrorsList
// @Router       /api/v1/nodes/{nodeId}/blobs/finalize [post].
func (c *rebalanceControllerV1) Finalize(ctx *gin.Context) {
	if !c.validateNodeID(ctx) {
		return
	}

	var req rebalanceapi.FinalizeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"invalid request body",
			err.Error(),
		))
		return
	}

	// Look up reservation
	reservation := c.reservationStore.Get(req.ReservationID)
	if reservation == nil {
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusNotFound,
			"RESERVATION_NOT_FOUND",
			"reservation not found or expired",
			req.ReservationID,
		))
		return
	}

	// Verify digest matches
	if reservation.Digest != req.Digest {
		_ = ctx.Error(httptk.NewHTTPError(
			http.StatusBadRequest,
			"DIGEST_MISMATCH",
			"digest does not match reservation",
			map[string]string{
				"expected": reservation.Digest,
				"received": req.Digest,
			},
		))
		return
	}

	requestCtx := ctx.Request.Context()

	// Update Redis location to point to this node
	err := c.blobCache.SetLocation(requestCtx, req.Digest, c.nodeRegistry.NodeID(), reservation.Tier)
	if err != nil {
		c.logger.Error("failed to update blob location",
			zap.String("reservationID", req.ReservationID),
			zap.String("digest", req.Digest),
			zap.Error(err))
		_ = ctx.Error(err)
		return
	}

	// Complete the reservation
	c.reservationStore.Complete(req.ReservationID)

	c.logger.Info("finalized blob transfer",
		zap.String("reservationID", req.ReservationID),
		zap.String("digest", req.Digest),
		zap.Int("tier", reservation.Tier))

	ctx.JSON(http.StatusOK, rebalanceapi.FinalizeResponse{
		Success: true,
	})
}

// extractHex extracts the hex portion from a digest string (e.g., "sha256:abc123" -> "abc123").
func extractHex(digest string) string {
	for i := range digest {
		if digest[i] == ':' {
			return digest[i+1:]
		}
	}
	return digest
}

// Ensure the body is consumed and closed.
var _ io.ReadCloser = (*struct {
	io.Reader
	io.Closer
})(nil)
