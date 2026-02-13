package rebalance

import (
	"math"
	"sort"
)

// EnrichedBlob contains blob metadata with access patterns for placement decisions.
type EnrichedBlob struct {
	// Digest is the content-addressable digest of the blob.
	Digest string
	// Size is the size of the blob in bytes.
	Size int64
	// MediaType is the OCI media type of the blob.
	MediaType string
	// AccessCount is the number of accesses within the scoring window.
	AccessCount int64
	// CurrentNode is the node ID where the blob currently resides.
	CurrentNode string
	// CurrentTier is the tier index where the blob currently resides.
	CurrentTier int
	// Score is the computed placement score (higher = hotter).
	Score float64
}

// ScoreBlob calculates the placement score for a blob.
// The formula is: accessCount * log2(size + 1)
//
// This scoring approach:
//   - Rewards high access counts linearly (frequently accessed blobs get higher scores)
//   - Applies logarithmic scaling to size (large blobs get a modest boost)
//   - Blobs with zero accesses get score 0 (eligible for cold tier)
//
// Higher score = hotter blob (should be in hot tier).
func ScoreBlob(accessCount int64, sizeBytes int64) float64 {
	if accessCount == 0 {
		return 0
	}
	// log2(size + 1) gives a logarithmic boost based on size
	// +1 prevents log(0) and ensures minimum multiplier of 1 for empty blobs
	sizeFactor := math.Log2(float64(sizeBytes) + 1)
	return float64(accessCount) * sizeFactor
}

// RankBlobs scores and sorts blobs by hotness (descending score).
// Returns a new slice with scored blobs, sorted from hottest to coldest.
func RankBlobs(blobs []*EnrichedBlob) []*EnrichedBlob {
	// Score each blob
	for _, b := range blobs {
		b.Score = ScoreBlob(b.AccessCount, b.Size)
	}

	// Sort descending by score (hottest first)
	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].Score > blobs[j].Score
	})

	return blobs
}

// EnrichBlob creates an EnrichedBlob from blobInfo with node context.
func EnrichBlob(info blobInfo, nodeID string, accessCount int64) *EnrichedBlob {
	return &EnrichedBlob{
		Digest:      info.Digest,
		Size:        info.Size,
		MediaType:   info.MediaType,
		AccessCount: accessCount,
		CurrentNode: nodeID,
		CurrentTier: info.Tier,
	}
}
