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
