package rebalance

import (
	"testing"

	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/pkg/configuration"
)

func TestTierBucket_RemainingCapacity(t *testing.T) {
	tests := []struct {
		name           string
		usableCapacity uint64
		assignedBytes  uint64
		want           uint64
	}{
		{
			name:           "empty bucket",
			usableCapacity: 1000,
			assignedBytes:  0,
			want:           1000,
		},
		{
			name:           "partially filled",
			usableCapacity: 1000,
			assignedBytes:  400,
			want:           600,
		},
		{
			name:           "full bucket",
			usableCapacity: 1000,
			assignedBytes:  1000,
			want:           0,
		},
		{
			name:           "overfilled bucket",
			usableCapacity: 1000,
			assignedBytes:  1200,
			want:           0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &TierBucket{
				UsableCapacity: tt.usableCapacity,
				AssignedBytes:  tt.assignedBytes,
			}
			if got := b.RemainingCapacity(); got != tt.want {
				t.Errorf("RemainingCapacity() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTierBucket_CanFit(t *testing.T) {
	tests := []struct {
		name           string
		usableCapacity uint64
		assignedBytes  uint64
		blobSize       int64
		want           bool
	}{
		{
			name:           "fits in empty bucket",
			usableCapacity: 1000,
			assignedBytes:  0,
			blobSize:       500,
			want:           true,
		},
		{
			name:           "fits exactly",
			usableCapacity: 1000,
			assignedBytes:  500,
			blobSize:       500,
			want:           true,
		},
		{
			name:           "does not fit",
			usableCapacity: 1000,
			assignedBytes:  600,
			blobSize:       500,
			want:           false,
		},
		{
			name:           "negative size",
			usableCapacity: 1000,
			assignedBytes:  0,
			blobSize:       -1,
			want:           false,
		},
		{
			name:           "zero size",
			usableCapacity: 1000,
			assignedBytes:  0,
			blobSize:       0,
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &TierBucket{
				UsableCapacity: tt.usableCapacity,
				AssignedBytes:  tt.assignedBytes,
			}
			if got := b.CanFit(tt.blobSize); got != tt.want {
				t.Errorf("CanFit(%d) = %v, want %v", tt.blobSize, got, tt.want)
			}
		})
	}
}

func TestTierBucket_Assign(t *testing.T) {
	bucket := &TierBucket{
		TierIndex:      0,
		UsableCapacity: 10000,
		AssignedBlobs:  make([]*EnrichedBlob, 0),
	}

	blob1 := &EnrichedBlob{Digest: "blob1", Size: 1000}
	blob2 := &EnrichedBlob{Digest: "blob2", Size: 2000}

	bucket.Assign(blob1)
	if len(bucket.AssignedBlobs) != 1 {
		t.Errorf("Expected 1 blob, got %d", len(bucket.AssignedBlobs))
	}
	if bucket.AssignedBytes != 1000 {
		t.Errorf("Expected 1000 assigned bytes, got %d", bucket.AssignedBytes)
	}

	bucket.Assign(blob2)
	if len(bucket.AssignedBlobs) != 2 {
		t.Errorf("Expected 2 blobs, got %d", len(bucket.AssignedBlobs))
	}
	if bucket.AssignedBytes != 3000 {
		t.Errorf("Expected 3000 assigned bytes, got %d", bucket.AssignedBytes)
	}
}

func TestBuildClusterCapacity(t *testing.T) {
	nodes := []*node.Info{
		{
			NodeID: "node-1",
			Status: node.StatusHealthy,
			Stats: node.Stats{
				TierDiskUsage: []node.DiskUsageStats{
					{TotalBytes: 1000, FreeBytes: 600, UsedBytes: 400},
					{TotalBytes: 5000, FreeBytes: 4000, UsedBytes: 1000},
				},
			},
		},
		{
			NodeID: "node-2",
			Status: node.StatusDegraded,
			Stats: node.Stats{
				TierDiskUsage: []node.DiskUsageStats{
					{TotalBytes: 2000, FreeBytes: 1500, UsedBytes: 500},
				},
			},
		},
	}

	capacity := BuildClusterCapacity(nodes)

	if len(capacity.Nodes) != 2 {
		t.Fatalf("Expected 2 nodes, got %d", len(capacity.Nodes))
	}

	// Check first node
	if capacity.Nodes[0].NodeID != "node-1" {
		t.Errorf("Expected node-1, got %s", capacity.Nodes[0].NodeID)
	}
	if !capacity.Nodes[0].IsHealthy {
		t.Error("Expected node-1 to be healthy")
	}
	if len(capacity.Nodes[0].Tiers) != 2 {
		t.Errorf("Expected 2 tiers for node-1, got %d", len(capacity.Nodes[0].Tiers))
	}

	// Check second node
	if capacity.Nodes[1].NodeID != "node-2" {
		t.Errorf("Expected node-2, got %s", capacity.Nodes[1].NodeID)
	}
	if capacity.Nodes[1].IsHealthy {
		t.Error("Expected node-2 to be unhealthy (degraded)")
	}
}

func TestCreateTierBuckets(t *testing.T) {
	capacity := &ClusterCapacity{
		Nodes: []NodeCapacity{
			{
				NodeID:    "node-1",
				IsHealthy: true,
				Tiers: []TierCapacity{
					{TierIndex: 0, TotalBytes: 1000, FreeBytes: 600},
					{TierIndex: 1, TotalBytes: 5000, FreeBytes: 4000},
				},
			},
			{
				NodeID:    "node-2",
				IsHealthy: true,
				Tiers: []TierCapacity{
					{TierIndex: 0, TotalBytes: 2000, FreeBytes: 1500},
					{TierIndex: 1, TotalBytes: 6000, FreeBytes: 5000},
				},
			},
		},
	}

	tierConfigs := []configuration.DiskTierConfiguration{
		{Tier: 0, ReservePercent: 0.20}, // 20% reserve
		{Tier: 1, ReservePercent: 0.05}, // 5% reserve
	}

	buckets := CreateTierBuckets(capacity, tierConfigs, nil)

	if len(buckets) != 2 {
		t.Fatalf("Expected 2 buckets, got %d", len(buckets))
	}

	// Tier 0: total = 1000 + 2000 = 3000, reserve 20% = 600
	// Usable = 3000 - 600 = 2400
	if buckets[0].TotalCapacity != 3000 {
		t.Errorf("Tier 0 total capacity: got %d, want 3000", buckets[0].TotalCapacity)
	}
	if buckets[0].ReservedBytes != 600 {
		t.Errorf("Tier 0 reserved bytes: got %d, want 600", buckets[0].ReservedBytes)
	}
	if buckets[0].UsableCapacity != 2400 {
		t.Errorf("Tier 0 usable capacity: got %d, want 2400", buckets[0].UsableCapacity)
	}

	// Tier 1: total = 5000 + 6000 = 11000, reserve 5% = 550
	// Usable = 11000 - 550 = 10450
	if buckets[1].TotalCapacity != 11000 {
		t.Errorf("Tier 1 total capacity: got %d, want 11000", buckets[1].TotalCapacity)
	}
	if buckets[1].ReservedBytes != 550 {
		t.Errorf("Tier 1 reserved bytes: got %d, want 550", buckets[1].ReservedBytes)
	}
	if buckets[1].UsableCapacity != 10450 {
		t.Errorf("Tier 1 usable capacity: got %d, want 10450", buckets[1].UsableCapacity)
	}
}

func TestCreateTierBuckets_WithCooldownSizes(t *testing.T) {
	capacity := &ClusterCapacity{
		Nodes: []NodeCapacity{
			{
				NodeID:    "node-1",
				IsHealthy: true,
				Tiers: []TierCapacity{
					{TierIndex: 0, TotalBytes: 1000, FreeBytes: 600},
				},
			},
		},
	}

	tierConfigs := []configuration.DiskTierConfiguration{
		{Tier: 0, ReservePercent: 0.0}, // No reserve
	}

	cooldownSizes := map[string][]uint64{
		"node-1": {200}, // 200 bytes in cooldown for tier 0
	}

	buckets := CreateTierBuckets(capacity, tierConfigs, cooldownSizes)

	// Total = 1000, no reserve, cooldown = 200
	// Usable = 1000 - 0 - 200 = 800
	if buckets[0].UsableCapacity != 800 {
		t.Errorf("Tier 0 usable capacity with cooldown: got %d, want 800", buckets[0].UsableCapacity)
	}
}

func TestAssignBlobsToTiers(t *testing.T) {
	buckets := []*TierBucket{
		{TierIndex: 0, UsableCapacity: 500, AssignedBlobs: make([]*EnrichedBlob, 0)},
		{TierIndex: 1, UsableCapacity: 2000, AssignedBlobs: make([]*EnrichedBlob, 0)},
	}

	// Ranked blobs (hottest first)
	rankedBlobs := []*EnrichedBlob{
		{Digest: "hot", Size: 200, Score: 100},
		{Digest: "warm", Size: 200, Score: 50},
		{Digest: "cool", Size: 200, Score: 25},
		{Digest: "cold", Size: 200, Score: 10},
	}

	unassigned := AssignBlobsToTiers(rankedBlobs, buckets)

	// All blobs should fit, so none should be unassigned
	if len(unassigned) != 0 {
		t.Errorf("Expected no unassigned blobs, got %d", len(unassigned))
	}

	// Hot tier (500 capacity) should have: hot (200) + warm (200) = 400, fits
	// Cool blob (200) would make 600 > 500, so goes to cold tier
	if len(buckets[0].AssignedBlobs) != 2 {
		t.Errorf("Hot tier should have 2 blobs, got %d", len(buckets[0].AssignedBlobs))
	}
	if buckets[0].AssignedBlobs[0].Digest != "hot" {
		t.Errorf("First blob in hot tier should be 'hot', got %s", buckets[0].AssignedBlobs[0].Digest)
	}
	if buckets[0].AssignedBlobs[1].Digest != "warm" {
		t.Errorf("Second blob in hot tier should be 'warm', got %s", buckets[0].AssignedBlobs[1].Digest)
	}

	// Cold tier should have: cool (200) + cold (200) = 400
	if len(buckets[1].AssignedBlobs) != 2 {
		t.Errorf("Cold tier should have 2 blobs, got %d", len(buckets[1].AssignedBlobs))
	}
}

func TestAssignBlobsToTiers_BlobTooLarge(t *testing.T) {
	buckets := []*TierBucket{
		{TierIndex: 0, UsableCapacity: 100, AssignedBlobs: make([]*EnrichedBlob, 0)},
		{TierIndex: 1, UsableCapacity: 100, AssignedBlobs: make([]*EnrichedBlob, 0)},
	}

	// A blob that's too large for any tier
	rankedBlobs := []*EnrichedBlob{
		{Digest: "huge", Size: 500, Score: 100},
	}

	unassigned := AssignBlobsToTiers(rankedBlobs, buckets)

	// Blob should not be assigned to any tier
	if len(buckets[0].AssignedBlobs) != 0 {
		t.Errorf("Hot tier should have 0 blobs, got %d", len(buckets[0].AssignedBlobs))
	}
	if len(buckets[1].AssignedBlobs) != 0 {
		t.Errorf("Cold tier should have 0 blobs, got %d", len(buckets[1].AssignedBlobs))
	}

	// Blob should be in unassigned list (for deletion when tiers are full)
	if len(unassigned) != 1 {
		t.Errorf("Expected 1 unassigned blob, got %d", len(unassigned))
	}
	if len(unassigned) > 0 && unassigned[0].Digest != "huge" {
		t.Errorf("Unassigned blob should be 'huge', got %s", unassigned[0].Digest)
	}
}

func TestAssignBlobsToTiers_AllTiersFull(t *testing.T) {
	buckets := []*TierBucket{
		{TierIndex: 0, UsableCapacity: 300, AssignedBlobs: make([]*EnrichedBlob, 0)},
		{TierIndex: 1, UsableCapacity: 300, AssignedBlobs: make([]*EnrichedBlob, 0)},
	}

	// More blobs than can fit in all tiers
	rankedBlobs := []*EnrichedBlob{
		{Digest: "hot", Size: 200, Score: 100},
		{Digest: "warm", Size: 200, Score: 50},
		{Digest: "cool", Size: 200, Score: 25},
		{Digest: "cold", Size: 200, Score: 10}, // This one won't fit
	}

	unassigned := AssignBlobsToTiers(rankedBlobs, buckets)

	// Hot tier (300 capacity) should have: hot (200) only (warm won't fit)
	if len(buckets[0].AssignedBlobs) != 1 {
		t.Errorf("Hot tier should have 1 blob, got %d", len(buckets[0].AssignedBlobs))
	}

	// Cold tier (300 capacity) should have: warm (200) + cool (200) won't fit
	// So warm (200) only
	if len(buckets[1].AssignedBlobs) != 1 {
		t.Errorf("Cold tier should have 1 blob, got %d", len(buckets[1].AssignedBlobs))
	}

	// Lower priority blobs (cool, cold) should be unassigned
	if len(unassigned) != 2 {
		t.Errorf("Expected 2 unassigned blobs, got %d", len(unassigned))
	}
}

func TestGetHealthyNodes(t *testing.T) {
	nodes := []*node.Info{
		{NodeID: "healthy-1", Status: node.StatusHealthy},
		{NodeID: "degraded", Status: node.StatusDegraded},
		{NodeID: "healthy-2", Status: node.StatusHealthy},
		{NodeID: "starting", Status: node.StatusStarting},
	}

	healthy := GetHealthyNodes(nodes)

	if len(healthy) != 2 {
		t.Fatalf("Expected 2 healthy nodes, got %d", len(healthy))
	}
	if healthy[0].NodeID != "healthy-1" {
		t.Errorf("First healthy node should be healthy-1, got %s", healthy[0].NodeID)
	}
	if healthy[1].NodeID != "healthy-2" {
		t.Errorf("Second healthy node should be healthy-2, got %s", healthy[1].NodeID)
	}
}
