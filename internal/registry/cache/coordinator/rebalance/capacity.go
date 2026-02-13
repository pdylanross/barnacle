package rebalance

import (
	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/pkg/configuration"
)

// ClusterCapacity is a snapshot of storage capacity across all nodes.
type ClusterCapacity struct {
	Nodes []NodeCapacity
}

// NodeCapacity represents the storage capacity of a single node.
type NodeCapacity struct {
	NodeID    string
	IsHealthy bool
	Tiers     []TierCapacity
}

// TierCapacity represents the storage capacity of a single tier on a node.
type TierCapacity struct {
	TierIndex  int
	TotalBytes uint64
	FreeBytes  uint64
	UsedBytes  uint64
}

// TierBucket holds blobs assigned to a tier across the cluster.
type TierBucket struct {
	TierIndex      int
	TotalCapacity  uint64
	ReservedBytes  uint64 // Space reserved for new blobs (not for rebalancing)
	UsableCapacity uint64 // TotalCapacity - ReservedBytes - CooldownBytes
	AssignedBlobs  []*EnrichedBlob
	AssignedBytes  uint64
}

// RemainingCapacity returns how much space is left in this bucket for assignment.
func (b *TierBucket) RemainingCapacity() uint64 {
	if b.AssignedBytes >= b.UsableCapacity {
		return 0
	}
	return b.UsableCapacity - b.AssignedBytes
}

// CanFit returns true if a blob of the given size can fit in this bucket.
func (b *TierBucket) CanFit(sizeBytes int64) bool {
	if sizeBytes < 0 {
		return false
	}
	return b.RemainingCapacity() >= uint64(sizeBytes)
}

// Assign adds a blob to this bucket and updates the assigned bytes.
func (b *TierBucket) Assign(blob *EnrichedBlob) {
	b.AssignedBlobs = append(b.AssignedBlobs, blob)
	b.AssignedBytes += uint64(max(0, blob.Size)) //nolint:gosec // size validated by CanFit before Assign
}

// BuildClusterCapacity creates a capacity snapshot from the node list.
func BuildClusterCapacity(nodes []*node.Info) *ClusterCapacity {
	capacity := &ClusterCapacity{
		Nodes: make([]NodeCapacity, 0, len(nodes)),
	}

	for _, n := range nodes {
		nodeCapacity := NodeCapacity{
			NodeID:    n.NodeID,
			IsHealthy: n.Status == node.StatusHealthy,
			Tiers:     make([]TierCapacity, len(n.Stats.TierDiskUsage)),
		}

		for i, tierUsage := range n.Stats.TierDiskUsage {
			nodeCapacity.Tiers[i] = TierCapacity{
				TierIndex:  i,
				TotalBytes: tierUsage.TotalBytes,
				FreeBytes:  tierUsage.FreeBytes,
				UsedBytes:  tierUsage.UsedBytes,
			}
		}

		capacity.Nodes = append(capacity.Nodes, nodeCapacity)
	}

	return capacity
}

// CreateTierBuckets builds tier buckets with per-tier reservation applied.
// Each tier's ReservePercent from config is applied to calculate usable capacity.
// cooldownSizes maps nodeID -> per-tier bytes occupied by cooldown blobs.
//
//nolint:gocognit // complexity is inherent to the algorithm
func CreateTierBuckets(
	capacity *ClusterCapacity,
	tierConfigs []configuration.DiskTierConfiguration,
	cooldownSizes map[string][]uint64,
) []*TierBucket {
	// Determine number of tiers
	numTiers := len(tierConfigs)
	if numTiers == 0 && len(capacity.Nodes) > 0 {
		// Fall back to first node's tier count if no config
		numTiers = len(capacity.Nodes[0].Tiers)
	}

	buckets := make([]*TierBucket, numTiers)

	for tierIdx := range numTiers {
		bucket := &TierBucket{
			TierIndex:     tierIdx,
			AssignedBlobs: make([]*EnrichedBlob, 0),
		}

		// Sum up capacity across all healthy nodes for this tier
		var totalCapacity uint64
		var totalCooldownBytes uint64

		for _, nodeCapacity := range capacity.Nodes {
			if !nodeCapacity.IsHealthy {
				continue
			}
			if tierIdx >= len(nodeCapacity.Tiers) {
				continue
			}

			tierCap := nodeCapacity.Tiers[tierIdx]
			totalCapacity += tierCap.TotalBytes

			// Add cooldown bytes for this node/tier
			if cooldown, ok := cooldownSizes[nodeCapacity.NodeID]; ok {
				if tierIdx < len(cooldown) {
					totalCooldownBytes += cooldown[tierIdx]
				}
			}
		}

		bucket.TotalCapacity = totalCapacity

		// Apply reserve percent from config
		var reservePercent float64
		if tierIdx < len(tierConfigs) {
			reservePercent = tierConfigs[tierIdx].GetReservePercent()
		}
		bucket.ReservedBytes = uint64(float64(totalCapacity) * reservePercent)

		// Usable = Total - Reserved - Cooldown
		usable := totalCapacity
		if bucket.ReservedBytes < usable {
			usable -= bucket.ReservedBytes
		} else {
			usable = 0
		}
		if totalCooldownBytes < usable {
			usable -= totalCooldownBytes
		} else {
			usable = 0
		}
		bucket.UsableCapacity = usable

		buckets[tierIdx] = bucket
	}

	return buckets
}

// AssignBlobsToTiers places ranked blobs into tiers based on capacity.
// Blobs are processed in order (hottest first) and placed in the hottest tier
// that has remaining capacity. Falls through to colder tiers if needed.
// Returns the list of blobs that could not be assigned to any tier (lower priority
// blobs that should be deleted when all tiers are full).
func AssignBlobsToTiers(rankedBlobs []*EnrichedBlob, buckets []*TierBucket) []*EnrichedBlob {
	var unassigned []*EnrichedBlob

	for _, blob := range rankedBlobs {
		assigned := false
		// Try to assign to tiers starting from hottest (tier 0)
		for _, bucket := range buckets {
			if bucket.CanFit(blob.Size) {
				bucket.Assign(blob)
				assigned = true
				break
			}
		}

		// If no tier could fit the blob, add it to unassigned list
		// These are lower priority blobs that should be deleted when all tiers are full
		if !assigned {
			unassigned = append(unassigned, blob)
		}
	}

	return unassigned
}

// GetHealthyNodes filters nodes to return only healthy ones.
func GetHealthyNodes(nodes []*node.Info) []*node.Info {
	healthy := make([]*node.Info, 0, len(nodes))
	for _, n := range nodes {
		if n.Status == node.StatusHealthy {
			healthy = append(healthy, n)
		}
	}
	return healthy
}

// NodePlacement represents where a blob should be placed.
type NodePlacement struct {
	Blob       *EnrichedBlob
	TargetNode string
	TargetTier int
	NeedsMove  bool // True if blob needs to move from current location
}

// PlaceBlobsOnNodes determines node placement for tiered blobs.
// For each blob in the buckets:
//   - Prefers keeping blob on its current node if that node has capacity in the target tier
//   - Falls back to first healthy node with capacity otherwise
//
// Returns placements for all blobs, with NeedsMove set appropriately.
//
//nolint:gocognit,nestif // complexity is inherent to the placement algorithm
func PlaceBlobsOnNodes(buckets []*TierBucket, capacity *ClusterCapacity) []*NodePlacement {
	placements := make([]*NodePlacement, 0)

	// Build a map of node capacity for quick lookup
	nodeCapMap := make(map[string]*NodeCapacity)
	for i := range capacity.Nodes {
		nodeCapMap[capacity.Nodes[i].NodeID] = &capacity.Nodes[i]
	}

	// Track how much space we've assigned to each node/tier
	// Key: "nodeID:tierIdx", Value: bytes assigned
	assignedBytes := make(map[string]uint64)

	for _, bucket := range buckets {
		targetTier := bucket.TierIndex

		for _, blob := range bucket.AssignedBlobs {
			placement := &NodePlacement{
				Blob:       blob,
				TargetTier: targetTier,
			}

			// Check if current node can hold this blob in the target tier
			currentNode := blob.CurrentNode
			if currentNodeCap, ok := nodeCapMap[currentNode]; ok && currentNodeCap.IsHealthy {
				if targetTier < len(currentNodeCap.Tiers) {
					tierCap := currentNodeCap.Tiers[targetTier]
					key := currentNode + ":" + string(rune(targetTier))
					used := assignedBytes[key]
					available := tierCap.FreeBytes
					if used < available {
						available -= used
					} else {
						available = 0
					}

					blobSize := uint64(max(0, blob.Size)) //nolint:gosec // size is always non-negative for valid blobs
					if blobSize <= available {
						// Keep on current node
						placement.TargetNode = currentNode
						placement.NeedsMove = targetTier != blob.CurrentTier
						assignedBytes[key] += blobSize
						placements = append(placements, placement)
						continue
					}
				}
			}

			// Fall back to first healthy node with capacity
			placed := false
			for _, nodeCap := range capacity.Nodes {
				if !nodeCap.IsHealthy {
					continue
				}
				if targetTier >= len(nodeCap.Tiers) {
					continue
				}

				tierCap := nodeCap.Tiers[targetTier]
				key := nodeCap.NodeID + ":" + string(rune(targetTier))
				used := assignedBytes[key]
				available := tierCap.FreeBytes
				if used < available {
					available -= used
				} else {
					available = 0
				}

				blobSize := uint64(max(0, blob.Size)) //nolint:gosec // size is always non-negative for valid blobs
				if blobSize <= available {
					placement.TargetNode = nodeCap.NodeID
					placement.NeedsMove = nodeCap.NodeID != blob.CurrentNode || targetTier != blob.CurrentTier
					assignedBytes[key] += blobSize
					placed = true
					break
				}
			}

			if !placed {
				// No node has capacity - blob stays in place
				placement.TargetNode = blob.CurrentNode
				placement.TargetTier = blob.CurrentTier
				placement.NeedsMove = false
			}

			placements = append(placements, placement)
		}
	}

	return placements
}
