package coordinator

import (
	"context"
	"errors"

	"github.com/pdylanross/barnacle/internal/node"
	"go.uber.org/zap"
)

// ErrNoCapacity is returned when no cache tier on any node has enough free space for a blob.
var ErrNoCapacity = errors.New("no cache capacity available in any tier on any node")

// NodeRegistryProvider is the interface for accessing node information.
// This is satisfied by *node.Registry.
type NodeRegistryProvider interface {
	NodeID() string
	GetNodeInfo() *node.Info
	ListOtherNodes(ctx context.Context) ([]*node.Info, error)
}

// CacheLocationDecision describes where a blob should be cached.
type CacheLocationDecision struct {
	// Local is true when the blob should be cached on this node.
	Local bool
	// NodeID is the target node ID. Set when Local is false.
	NodeID string
	// Tier is the tier number where the blob should be cached.
	Tier int
}

// cacheInformer determines optimal blob cache placement across the cluster.
type cacheInformer struct {
	nodeRegistry  NodeRegistryProvider
	tiers         []TierCache
	logger        *zap.Logger
	spaceReserver *SpaceReserver
}

// newCacheInformer creates a new cacheInformer.
func newCacheInformer(
	nodeRegistry NodeRegistryProvider,
	tiers []TierCache,
	logger *zap.Logger,
	spaceReserver *SpaceReserver,
) *cacheInformer {
	return &cacheInformer{
		nodeRegistry:  nodeRegistry,
		tiers:         tiers,
		logger:        logger.Named("cacheInformer"),
		spaceReserver: spaceReserver,
	}
}

// FindCacheLocation determines where a blob of the given size should be cached.
// It examines disk capacity across all nodes and tiers, preferring the local node
// and lower-numbered tiers.
//
// When the decision is local, a non-nil Reservation is returned that the caller
// must Release after the blob write completes (success or failure).
// When the decision is remote, the returned Reservation is nil.
func (ci *cacheInformer) FindCacheLocation(
	ctx context.Context,
	blobSize int64,
) (*CacheLocationDecision, *Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	size := uint64(blobSize) //nolint:gosec // blob sizes are non-negative

	localInfo := ci.nodeRegistry.GetNodeInfo()

	remoteNodes, err := ci.nodeRegistry.ListOtherNodes(ctx)
	if err != nil {
		ci.logger.Warn("failed to list other nodes, degrading to local-only",
			zap.Error(err))
		remoteNodes = nil
	}

	for tierIdx, tier := range ci.tiers {
		// Check local node first (accounts for in-flight reservations)
		if ci.localNodeHasCapacity(localInfo, tierIdx, size) {
			reservation := ci.spaceReserver.Reserve(tierIdx, size)
			return &CacheLocationDecision{
				Local: true,
				Tier:  tier.Tier,
			}, reservation, nil
		}

		// Check remote nodes
		for _, remoteNode := range remoteNodes {
			if remoteNode.Status != node.StatusHealthy {
				continue
			}

			if ci.nodeHasCapacity(remoteNode, tierIdx, size) {
				return &CacheLocationDecision{
					Local:  false,
					NodeID: remoteNode.NodeID,
					Tier:   tier.Tier,
				}, nil, nil
			}
		}
	}

	ci.logger.Warn("no cache capacity available for blob",
		zap.Int64("blobSize", blobSize))

	return nil, nil, ErrNoCapacity
}

// localNodeHasCapacity checks whether the local node has enough free space in the
// given tier index, subtracting in-flight reservations from the reported free space.
func (ci *cacheInformer) localNodeHasCapacity(info *node.Info, tierIndex int, blobSize uint64) bool {
	if tierIndex >= len(info.Stats.TierDiskUsage) {
		return false
	}

	usage := info.Stats.TierDiskUsage[tierIndex]

	if usage.TotalBytes == 0 {
		return false
	}

	claimed := ci.spaceReserver.ClaimedBytes(tierIndex)

	// Underflow protection: if claimed >= FreeBytes, effective free is 0
	var effectiveFree uint64
	if usage.FreeBytes > claimed {
		effectiveFree = usage.FreeBytes - claimed
	}

	return effectiveFree >= blobSize
}

// nodeHasCapacity checks whether a node has enough free space in the given tier index.
func (ci *cacheInformer) nodeHasCapacity(info *node.Info, tierIndex int, blobSize uint64) bool {
	if tierIndex >= len(info.Stats.TierDiskUsage) {
		return false
	}

	usage := info.Stats.TierDiskUsage[tierIndex]

	// Skip tiers where stats collection failed (TotalBytes == 0)
	if usage.TotalBytes == 0 {
		return false
	}

	return usage.FreeBytes >= blobSize
}
