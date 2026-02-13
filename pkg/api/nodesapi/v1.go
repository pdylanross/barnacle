// Package nodesapi contains DTOs for the nodes management API.
package nodesapi

import "time"

// DiskUsageResponse represents disk usage statistics for a single cache tier.
type DiskUsageResponse struct {
	// Path is the filesystem path for this cache tier.
	Path string `json:"path" example:"/var/cache/barnacle/hot"`
	// TotalBytes is the total size of the filesystem in bytes.
	TotalBytes uint64 `json:"totalBytes" example:"107374182400"`
	// FreeBytes is the available space in bytes.
	FreeBytes uint64 `json:"freeBytes" example:"53687091200"`
	// UsedBytes is the used space in bytes.
	UsedBytes uint64 `json:"usedBytes" example:"53687091200"`
}

// StatsResponse contains high-level statistics about a node.
type StatsResponse struct {
	// TierDiskUsage contains disk usage statistics for each cache tier.
	TierDiskUsage []DiskUsageResponse `json:"tierDiskUsage"`
}

// NodeResponse is the response body for GET /api/v1/nodes/:nodeId and GET /api/v1/nodes/me.
type NodeResponse struct {
	// NodeID is the unique identifier for this node.
	NodeID string `json:"nodeId" example:"node-1"`
	// Status indicates the current operational state of the node.
	Status string `json:"status" example:"ready"`
	// LastUpdated is the timestamp when this information was last updated.
	LastUpdated time.Time `json:"lastUpdated"`
	// Stats contains high-level statistics about the node.
	Stats StatsResponse `json:"stats"`
}

// ListNodesResponse is the response body for GET /api/v1/nodes.
type ListNodesResponse struct {
	// Nodes is a list of all registered nodes.
	Nodes []NodeResponse `json:"nodes"`
}
