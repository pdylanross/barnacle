package rebalance

import (
	"math"
	"testing"
)

func TestScoreBlob(t *testing.T) {
	tests := []struct {
		name        string
		accessCount int64
		sizeBytes   int64
		wantScore   float64
	}{
		{
			name:        "zero accesses returns zero score",
			accessCount: 0,
			sizeBytes:   1024 * 1024, // 1MB
			wantScore:   0,
		},
		{
			name:        "zero size with accesses",
			accessCount: 10,
			sizeBytes:   0,
			wantScore:   10 * math.Log2(1), // log2(0+1) = 0, so 10 * 0 = 0
		},
		{
			name:        "1MB blob with 100 accesses",
			accessCount: 100,
			sizeBytes:   1024 * 1024,
			wantScore:   100 * math.Log2(float64(1024*1024)+1),
		},
		{
			name:        "1GB blob with 10 accesses",
			accessCount: 10,
			sizeBytes:   1024 * 1024 * 1024,
			wantScore:   10 * math.Log2(float64(1024*1024*1024)+1),
		},
		{
			name:        "small blob high access",
			accessCount: 1000,
			sizeBytes:   1024, // 1KB
			wantScore:   1000 * math.Log2(float64(1024)+1),
		},
		{
			name:        "large blob low access",
			accessCount: 1,
			sizeBytes:   10 * 1024 * 1024 * 1024, // 10GB
			wantScore:   1 * math.Log2(float64(10*1024*1024*1024)+1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScoreBlob(tt.accessCount, tt.sizeBytes)
			if math.Abs(got-tt.wantScore) > 0.001 {
				t.Errorf("ScoreBlob(%d, %d) = %f, want %f",
					tt.accessCount, tt.sizeBytes, got, tt.wantScore)
			}
		})
	}
}

func TestScoreBlob_Ordering(t *testing.T) {
	// Test that high access count blobs score higher than low access count
	highAccess := ScoreBlob(1000, 1024*1024)
	lowAccess := ScoreBlob(10, 1024*1024)

	if highAccess <= lowAccess {
		t.Errorf("High access blob (%f) should score higher than low access blob (%f)",
			highAccess, lowAccess)
	}

	// Test that access count has more impact than size for typical scenarios
	smallHotBlob := ScoreBlob(100, 1024)         // 1KB, 100 accesses
	largeColdBlob := ScoreBlob(1, 1024*1024*100) // 100MB, 1 access

	if smallHotBlob <= largeColdBlob {
		t.Errorf("Small hot blob (%f) should score higher than large cold blob (%f)",
			smallHotBlob, largeColdBlob)
	}
}

func TestRankBlobs(t *testing.T) {
	blobs := []*EnrichedBlob{
		{Digest: "cold", AccessCount: 0, Size: 1024 * 1024},
		{Digest: "hot", AccessCount: 100, Size: 1024 * 1024},
		{Digest: "warm", AccessCount: 50, Size: 1024 * 1024},
		{Digest: "lukewarm", AccessCount: 25, Size: 1024 * 1024},
	}

	ranked := RankBlobs(blobs)

	// Verify order: hot, warm, lukewarm, cold
	expectedOrder := []string{"hot", "warm", "lukewarm", "cold"}
	for i, expected := range expectedOrder {
		if ranked[i].Digest != expected {
			t.Errorf("Position %d: got %s, want %s", i, ranked[i].Digest, expected)
		}
	}

	// Verify scores are in descending order
	for i := 1; i < len(ranked); i++ {
		if ranked[i].Score > ranked[i-1].Score {
			t.Errorf("Scores not in descending order: %f > %f",
				ranked[i].Score, ranked[i-1].Score)
		}
	}
}

func TestEnrichBlob(t *testing.T) {
	info := blobInfo{
		Digest:    "sha256:abc123",
		Size:      1024 * 1024,
		MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		Tier:      0,
	}

	enriched := EnrichBlob(info, "node-1", 42)

	if enriched.Digest != info.Digest {
		t.Errorf("Digest mismatch: got %s, want %s", enriched.Digest, info.Digest)
	}
	if enriched.Size != info.Size {
		t.Errorf("Size mismatch: got %d, want %d", enriched.Size, info.Size)
	}
	if enriched.MediaType != info.MediaType {
		t.Errorf("MediaType mismatch: got %s, want %s", enriched.MediaType, info.MediaType)
	}
	if enriched.CurrentNode != "node-1" {
		t.Errorf("CurrentNode mismatch: got %s, want node-1", enriched.CurrentNode)
	}
	if enriched.CurrentTier != info.Tier {
		t.Errorf("CurrentTier mismatch: got %d, want %d", enriched.CurrentTier, info.Tier)
	}
	if enriched.AccessCount != 42 {
		t.Errorf("AccessCount mismatch: got %d, want 42", enriched.AccessCount)
	}
}
