package rebalance

import (
	"time"

	"github.com/google/uuid"
)

// Decision represents a decision to move or delete a blob.
type Decision struct {
	// ID is a unique identifier for this decision.
	ID string `json:"id"`
	// SourceNodeID is the node that currently holds the blob.
	SourceNodeID string `json:"sourceNodeId"`
	// TargetNodeID is the node where the blob should be moved.
	// Empty if DeleteOnly is true.
	TargetNodeID string `json:"targetNodeId"`
	// Digest is the content-addressable digest of the blob.
	Digest string `json:"digest"`
	// Size is the size of the blob in bytes.
	Size int64 `json:"size"`
	// MediaType is the OCI media type of the blob.
	MediaType string `json:"mediaType"`
	// SourceTier is the cache tier on the source node.
	SourceTier int `json:"sourceTier"`
	// TargetTier is the intended cache tier on the target node.
	// Ignored if DeleteOnly is true.
	TargetTier int `json:"targetTier"`
	// DeleteOnly indicates this blob should be deleted from cache without
	// being moved. This happens when all tiers are full and this blob has
	// lower priority than others.
	DeleteOnly bool `json:"deleteOnly"`
	// CreatedAt is when this decision was created.
	CreatedAt time.Time `json:"createdAt"`
}

// NewDecision creates a new rebalance decision with a generated ID.
func NewDecision(
	sourceNodeID, targetNodeID string,
	digest string,
	size int64,
	mediaType string,
	sourceTier, targetTier int,
) *Decision {
	return &Decision{
		ID:           uuid.New().String(),
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNodeID,
		Digest:       digest,
		Size:         size,
		MediaType:    mediaType,
		SourceTier:   sourceTier,
		TargetTier:   targetTier,
		DeleteOnly:   false,
		CreatedAt:    time.Now(),
	}
}

// NewDeleteDecision creates a decision to delete a blob from cache.
// This is used when all storage tiers are full and the blob has lower
// priority than others that need to be kept.
func NewDeleteDecision(
	sourceNodeID string,
	digest string,
	size int64,
	mediaType string,
	sourceTier int,
) *Decision {
	return &Decision{
		ID:           uuid.New().String(),
		SourceNodeID: sourceNodeID,
		Digest:       digest,
		Size:         size,
		MediaType:    mediaType,
		SourceTier:   sourceTier,
		DeleteOnly:   true,
		CreatedAt:    time.Now(),
	}
}
