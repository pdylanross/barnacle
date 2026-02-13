package node

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// DiskUsageStats contains disk usage information for a filesystem path.
type DiskUsageStats struct {
	// Path is the filesystem path that was checked.
	Path string `json:"path"`
	// TotalBytes is the total size of the filesystem in bytes.
	TotalBytes uint64 `json:"totalBytes"`
	// FreeBytes is the available space in bytes (for unprivileged users).
	FreeBytes uint64 `json:"freeBytes"`
	// UsedBytes is the used space in bytes.
	UsedBytes uint64 `json:"usedBytes"`
}

// GetDiskUsage returns disk usage statistics for the given path.
// It uses unix.Statfs to query the filesystem containing the path.
func GetDiskUsage(path string) (*DiskUsageStats, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("failed to get disk usage for %s: %w", path, err)
	}

	// Calculate sizes using block size
	// Bsize is always positive on valid filesystems, but we handle the conversion safely
	blockSize := uint64(stat.Bsize) //nolint:gosec // Bsize is always positive for valid filesystems
	totalBytes := stat.Blocks * blockSize
	freeBytes := stat.Bavail * blockSize // Bavail is available to unprivileged users
	usedBytes := totalBytes - (stat.Bfree * blockSize)

	return &DiskUsageStats{
		Path:       path,
		TotalBytes: totalBytes,
		FreeBytes:  freeBytes,
		UsedBytes:  usedBytes,
	}, nil
}
