package node_test

import (
	"testing"

	"github.com/pdylanross/barnacle/internal/node"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDiskUsage(t *testing.T) {
	t.Run("returns disk usage for valid path", func(t *testing.T) {
		tempDir := t.TempDir()

		stats, err := node.GetDiskUsage(tempDir)
		require.NoError(t, err)

		assert.Equal(t, tempDir, stats.Path)
		assert.NotZero(t, stats.TotalBytes, "TotalBytes should be non-zero")
		assert.NotZero(t, stats.FreeBytes, "FreeBytes should be non-zero")
		// UsedBytes could be zero on a fresh filesystem, so we don't assert on it
		assert.LessOrEqual(t, stats.UsedBytes, stats.TotalBytes, "UsedBytes should not exceed TotalBytes")
		assert.LessOrEqual(t, stats.FreeBytes, stats.TotalBytes, "FreeBytes should not exceed TotalBytes")
	})

	t.Run("returns error for non-existent path", func(t *testing.T) {
		_, err := node.GetDiskUsage("/non/existent/path/that/should/not/exist")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get disk usage")
	})

	t.Run("works with root path", func(t *testing.T) {
		stats, err := node.GetDiskUsage("/")
		require.NoError(t, err)

		assert.Equal(t, "/", stats.Path)
		assert.NotZero(t, stats.TotalBytes)
	})
}

func TestApplySizeLimit(t *testing.T) {
	t.Run("zero limit returns unchanged stats", func(t *testing.T) {
		stats := &node.DiskUsageStats{
			Path:       "/test",
			TotalBytes: 100 * 1024 * 1024 * 1024, // 100 GiB
			FreeBytes:  60 * 1024 * 1024 * 1024,  // 60 GiB
			UsedBytes:  40 * 1024 * 1024 * 1024,  // 40 GiB
		}

		result := node.ApplySizeLimit(stats, 0)

		assert.Equal(t, stats.TotalBytes, result.TotalBytes)
		assert.Equal(t, stats.FreeBytes, result.FreeBytes)
		assert.Equal(t, stats.UsedBytes, result.UsedBytes)
	})

	t.Run("limit smaller than total caps TotalBytes and adjusts FreeBytes", func(t *testing.T) {
		stats := &node.DiskUsageStats{
			Path:       "/test",
			TotalBytes: 100 * 1024 * 1024 * 1024, // 100 GiB
			FreeBytes:  60 * 1024 * 1024 * 1024,  // 60 GiB
			UsedBytes:  40 * 1024 * 1024 * 1024,  // 40 GiB
		}
		limit := uint64(50 * 1024 * 1024 * 1024) // 50 GiB

		result := node.ApplySizeLimit(stats, limit)

		assert.Equal(t, limit, result.TotalBytes)
		assert.Equal(t, stats.UsedBytes, result.UsedBytes) // UsedBytes unchanged
		// FreeBytes = TotalBytes - UsedBytes = 50 GiB - 40 GiB = 10 GiB
		assert.Equal(t, uint64(10*1024*1024*1024), result.FreeBytes)
	})

	t.Run("limit larger than total leaves stats unchanged", func(t *testing.T) {
		stats := &node.DiskUsageStats{
			Path:       "/test",
			TotalBytes: 100 * 1024 * 1024 * 1024, // 100 GiB
			FreeBytes:  60 * 1024 * 1024 * 1024,  // 60 GiB
			UsedBytes:  40 * 1024 * 1024 * 1024,  // 40 GiB
		}
		limit := uint64(200 * 1024 * 1024 * 1024) // 200 GiB (larger than actual)

		result := node.ApplySizeLimit(stats, limit)

		// TotalBytes should remain at the actual value, not increase
		assert.Equal(t, stats.TotalBytes, result.TotalBytes)
		assert.Equal(t, stats.UsedBytes, result.UsedBytes)
		// FreeBytes = TotalBytes - UsedBytes = 100 GiB - 40 GiB = 60 GiB
		assert.Equal(t, uint64(60*1024*1024*1024), result.FreeBytes)
	})

	t.Run("usage exceeds limit sets FreeBytes to 0", func(t *testing.T) {
		stats := &node.DiskUsageStats{
			Path:       "/test",
			TotalBytes: 100 * 1024 * 1024 * 1024, // 100 GiB
			FreeBytes:  60 * 1024 * 1024 * 1024,  // 60 GiB
			UsedBytes:  40 * 1024 * 1024 * 1024,  // 40 GiB
		}
		limit := uint64(30 * 1024 * 1024 * 1024) // 30 GiB (less than used)

		result := node.ApplySizeLimit(stats, limit)

		assert.Equal(t, limit, result.TotalBytes)
		assert.Equal(t, stats.UsedBytes, result.UsedBytes)
		assert.Equal(t, uint64(0), result.FreeBytes) // No free space
	})

	t.Run("does not mutate original stats", func(t *testing.T) {
		original := &node.DiskUsageStats{
			Path:       "/test",
			TotalBytes: 100 * 1024 * 1024 * 1024,
			FreeBytes:  60 * 1024 * 1024 * 1024,
			UsedBytes:  40 * 1024 * 1024 * 1024,
		}
		originalTotal := original.TotalBytes
		originalFree := original.FreeBytes

		limit := uint64(50 * 1024 * 1024 * 1024)
		node.ApplySizeLimit(original, limit)

		// Original stats should be unchanged
		assert.Equal(t, originalTotal, original.TotalBytes)
		assert.Equal(t, originalFree, original.FreeBytes)
	})
}
