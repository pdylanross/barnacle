// Package rebalanceapi contains DTOs for the blob rebalancing API.
package rebalanceapi

import "time"

// ReserveRequest is the request body for POST /api/v1/nodes/:nodeId/blobs/reserve.
type ReserveRequest struct {
	// Digest is the content-addressable digest of the blob to reserve space for.
	Digest string `json:"digest" binding:"required" example:"sha256:abc123def456..."`
	// Size is the size of the blob in bytes.
	Size int64 `json:"size" binding:"required,min=1" example:"1048576"`
	// MediaType is the OCI media type of the blob.
	MediaType string `json:"mediaType" binding:"required" example:"application/vnd.oci.image.layer.v1.tar+gzip"`
	// SourceNodeID is the node that currently holds the blob.
	SourceNodeID string `json:"sourceNodeId" binding:"required" example:"node-1"`
}

// ReserveResponse is the response body for POST /api/v1/nodes/:nodeId/blobs/reserve.
type ReserveResponse struct {
	// ReservationID is a unique identifier for this reservation.
	ReservationID string `json:"reservationId" example:"550e8400-e29b-41d4-a716-446655440000"`
	// ExpiresAt is when this reservation expires if not used.
	ExpiresAt time.Time `json:"expiresAt" example:"2024-01-15T10:30:00Z"`
}

// ReceiveResponse is the response body for POST /api/v1/nodes/:nodeId/blobs/receive.
type ReceiveResponse struct {
	// Digest is the digest of the received blob.
	Digest string `json:"digest" example:"sha256:abc123def456..."`
	// Size is the size of the received blob in bytes.
	Size int64 `json:"size" example:"1048576"`
}

// FinalizeRequest is the request body for POST /api/v1/nodes/:nodeId/blobs/finalize.
type FinalizeRequest struct {
	// ReservationID is the reservation to finalize.
	ReservationID string `json:"reservationId" binding:"required" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Digest is the digest of the blob being finalized.
	Digest string `json:"digest" binding:"required" example:"sha256:abc123def456..."`
	// Tier is the cache tier where the blob was stored.
	Tier int `json:"tier" example:"0"`
}

// FinalizeResponse is the response body for POST /api/v1/nodes/:nodeId/blobs/finalize.
type FinalizeResponse struct {
	// Success indicates whether finalization succeeded.
	Success bool `json:"success" example:"true"`
}
