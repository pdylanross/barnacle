package rebalance

import (
	"context"
	"fmt"

	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"go.uber.org/zap"
)

// maxTierCount is the maximum number of storage tiers supported for cooldown tracking.
const maxTierCount = 10

// Planner orchestrates the rebalance planning algorithm.
// It collects blobs across all nodes, scores them, and determines optimal placement.
type Planner struct {
	blobCache       coordinator.Cache
	nodeRegistry    *node.Registry
	nodeClient      *NodeClient
	cooldownManager *CooldownManager
	nodeID          string
	logger          *zap.Logger
	rebalanceConfig *configuration.RebalanceConfiguration
	cacheConfig     *configuration.CacheConfiguration
}

// NewPlanner creates a new Planner instance.
func NewPlanner(
	blobCache coordinator.Cache,
	nodeRegistry *node.Registry,
	nodeClient *NodeClient,
	cooldownManager *CooldownManager,
	nodeID string,
	logger *zap.Logger,
	rebalanceConfig *configuration.RebalanceConfiguration,
	cacheConfig *configuration.CacheConfiguration,
) *Planner {
	return &Planner{
		blobCache:       blobCache,
		nodeRegistry:    nodeRegistry,
		nodeClient:      nodeClient,
		cooldownManager: cooldownManager,
		nodeID:          nodeID,
		logger:          logger.Named("planner"),
		rebalanceConfig: rebalanceConfig,
		cacheConfig:     cacheConfig,
	}
}

// Plan executes the full rebalance planning algorithm and returns decisions.
// This is the main entry point called by Leader.Rebalance().
func (p *Planner) Plan(ctx context.Context) ([]*Decision, error) {
	// 1. List healthy nodes
	nodes, err := p.nodeRegistry.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	healthyNodes := GetHealthyNodes(nodes)

	if len(healthyNodes) == 0 {
		p.logger.Warn("no healthy nodes found, skipping rebalance")
		return nil, nil
	}

	p.logger.Debug("planning rebalance",
		zap.Int("totalNodes", len(nodes)),
		zap.Int("healthyNodes", len(healthyNodes)))

	// 2. Collect ALL blobs across cluster
	allBlobsByNode, err := p.collectAllBlobs(ctx, healthyNodes)
	if err != nil {
		return nil, fmt.Errorf("collect blobs: %w", err)
	}

	// 3. Filter out cooldown blobs EARLY (saves processing)
	rebalanceableBlobs, cooldownSizes, err := p.filterCooldownBlobs(ctx, allBlobsByNode)
	if err != nil {
		return nil, fmt.Errorf("filter cooldown: %w", err)
	}

	// 4. Build cluster capacity snapshot (accounts for cooldown blob space)
	capacity := BuildClusterCapacity(healthyNodes)

	// 5. Enrich with access counts and rank globally
	rankedBlobs, err := p.enrichAndRankBlobs(ctx, rebalanceableBlobs)
	if err != nil {
		return nil, fmt.Errorf("rank blobs: %w", err)
	}

	if len(rankedBlobs) == 0 {
		p.logger.Debug("no blobs to rebalance")
		return nil, nil
	}

	// 6. Create tier buckets with per-tier reservation
	buckets := CreateTierBuckets(capacity, p.cacheConfig.Disk.Tiers, cooldownSizes)

	// 7. Assign blobs to tiers based on rank
	// Returns unassigned blobs (lower priority blobs that don't fit anywhere)
	unassignedBlobs := AssignBlobsToTiers(rankedBlobs, buckets)

	// 8. Place blobs on nodes (prefer current placement)
	placements := PlaceBlobsOnNodes(buckets, capacity)

	// 9. Generate decisions for blobs that need to move
	decisions := p.generateDecisions(placements)

	// 10. Generate delete decisions for unassigned blobs when all tiers are full
	// These are lower priority blobs that don't fit in any tier
	if len(unassignedBlobs) > 0 {
		deleteDecisions := p.generateDeleteDecisions(unassignedBlobs)
		decisions = append(decisions, deleteDecisions...)

		p.logger.Info("generated delete decisions for overflow blobs",
			zap.Int("deleteCount", len(deleteDecisions)))
	}

	p.logger.Info("planning complete",
		zap.Int("totalBlobs", countTotalBlobs(allBlobsByNode)),
		zap.Int("rebalanceableBlobs", len(rankedBlobs)),
		zap.Int("unassignedBlobs", len(unassignedBlobs)),
		zap.Int("decisions", len(decisions)))

	return decisions, nil
}

// collectAllBlobs gathers blobs from all healthy nodes (local + remote via HTTP).
func (p *Planner) collectAllBlobs(
	ctx context.Context,
	nodes []*node.Info,
) (map[string][]blobInfo, error) {
	blobsByNode := make(map[string][]blobInfo)

	for _, nodeInfo := range nodes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var blobs []blobInfo
		var err error

		if nodeInfo.NodeID == p.nodeID {
			// Local node - use blob cache directly
			blobs, err = p.getLocalBlobs(ctx)
		} else {
			// Remote node - use HTTP client
			blobs, err = p.nodeClient.listBlobs(ctx, nodeInfo)
		}

		if err != nil {
			p.logger.Warn("failed to get blobs from node",
				zap.String("nodeID", nodeInfo.NodeID),
				zap.Error(err))
			// Continue with other nodes
			continue
		}

		blobsByNode[nodeInfo.NodeID] = blobs
	}

	return blobsByNode, nil
}

// getLocalBlobs retrieves blob information from the local node.
func (p *Planner) getLocalBlobs(ctx context.Context) ([]blobInfo, error) {
	localBlobs, err := p.blobCache.ListLocal(ctx)
	if err != nil {
		return nil, err
	}

	accessWindow := p.rebalanceConfig.GetAccessWindow()
	blobs := make([]blobInfo, len(localBlobs))

	for i, b := range localBlobs {
		accessCount, _ := p.blobCache.GetAccessCountWindow(ctx, b.Digest, accessWindow)
		blobs[i] = blobInfo{
			Digest:      b.Digest,
			Size:        b.Size,
			MediaType:   b.MediaType,
			Tier:        b.Tier,
			AccessCount: accessCount,
		}
	}

	return blobs, nil
}

// filterCooldownBlobs separates blobs into rebalanceable vs on-cooldown.
// Returns:
//   - rebalanceableBlobs: map of nodeID -> blobs that can be moved
//   - cooldownSizes: map of nodeID -> per-tier bytes occupied by cooldown blobs
func (p *Planner) filterCooldownBlobs(
	ctx context.Context,
	blobsByNode map[string][]blobInfo,
) (map[string][]blobInfo, map[string][]uint64, error) {
	rebalanceable := make(map[string][]blobInfo)
	cooldownSizes := make(map[string][]uint64)

	for nodeID, blobs := range blobsByNode {
		var nodeRebalanceable []blobInfo
		nodeCooldownSizes := make([]uint64, maxTierCount)

		for _, blob := range blobs {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}

			onCooldown, err := p.cooldownManager.IsOnCooldown(ctx, blob.Digest)
			if err != nil {
				p.logger.Debug("failed to check cooldown",
					zap.String("digest", blob.Digest),
					zap.Error(err))
				// Treat as not on cooldown if check fails
				onCooldown = false
			}

			if onCooldown {
				// Track size by tier for capacity calculation
				if blob.Tier < len(nodeCooldownSizes) {
					nodeCooldownSizes[blob.Tier] += uint64(max(0, blob.Size))
				}
			} else {
				nodeRebalanceable = append(nodeRebalanceable, blob)
			}
		}

		rebalanceable[nodeID] = nodeRebalanceable
		cooldownSizes[nodeID] = nodeCooldownSizes
	}

	return rebalanceable, cooldownSizes, nil
}

// enrichAndRankBlobs adds access counts and scores, then sorts descending by score.
func (p *Planner) enrichAndRankBlobs(
	ctx context.Context,
	blobsByNode map[string][]blobInfo,
) ([]*EnrichedBlob, error) {
	var allBlobs []*EnrichedBlob

	for nodeID, blobs := range blobsByNode {
		for _, blob := range blobs {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			// Get access count if not already set
			accessCount := blob.AccessCount
			if accessCount == 0 && nodeID == p.nodeID {
				// Try to get access count for local blobs
				count, _ := p.blobCache.GetAccessCountWindow(
					ctx,
					blob.Digest,
					p.rebalanceConfig.GetAccessWindow(),
				)
				accessCount = count
			}

			enriched := EnrichBlob(blob, nodeID, accessCount)
			allBlobs = append(allBlobs, enriched)
		}
	}

	return RankBlobs(allBlobs), nil
}

// generateDecisions creates Decision objects for blobs that need to move.
func (p *Planner) generateDecisions(placements []*NodePlacement) []*Decision {
	var decisions []*Decision

	for _, placement := range placements {
		if !placement.NeedsMove {
			continue
		}

		decision := NewDecision(
			placement.Blob.CurrentNode,
			placement.TargetNode,
			placement.Blob.Digest,
			placement.Blob.Size,
			placement.Blob.MediaType,
			placement.Blob.CurrentTier,
			placement.TargetTier,
		)

		decisions = append(decisions, decision)

		p.logger.Debug("generated decision",
			zap.String("digest", placement.Blob.Digest),
			zap.String("sourceNode", placement.Blob.CurrentNode),
			zap.String("targetNode", placement.TargetNode),
			zap.Int("sourceTier", placement.Blob.CurrentTier),
			zap.Int("targetTier", placement.TargetTier))
	}

	return decisions
}

// generateDeleteDecisions creates delete decisions for blobs that could not be
// assigned to any tier. These are lower priority blobs that should be evicted
// when all storage tiers are full (including spare space).
func (p *Planner) generateDeleteDecisions(unassignedBlobs []*EnrichedBlob) []*Decision {
	var decisions []*Decision

	for _, blob := range unassignedBlobs {
		decision := NewDeleteDecision(
			blob.CurrentNode,
			blob.Digest,
			blob.Size,
			blob.MediaType,
			blob.CurrentTier,
		)

		decisions = append(decisions, decision)

		p.logger.Debug("generated delete decision",
			zap.String("digest", blob.Digest),
			zap.String("sourceNode", blob.CurrentNode),
			zap.Int("sourceTier", blob.CurrentTier),
			zap.Float64("score", blob.Score))
	}

	return decisions
}

// countTotalBlobs counts total blobs across all nodes.
func countTotalBlobs(blobsByNode map[string][]blobInfo) int {
	total := 0
	for _, blobs := range blobsByNode {
		total += len(blobs)
	}
	return total
}
