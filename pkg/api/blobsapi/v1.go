// Package blobsapi contains DTOs for the node blob management API.
package blobsapi

// BlobResponse represents a single cached blob's information.
type BlobResponse struct {
	// Digest is the content-addressable digest of the blob (e.g., "sha256:abc123...").
	Digest string `json:"digest" example:"sha256:abc123def456..."`
	// Size is the size of the blob in bytes.
	Size int64 `json:"size" example:"1048576"`
	// MediaType is the OCI media type of the blob.
	MediaType string `json:"mediaType" example:"application/vnd.oci.image.layer.v1.tar+gzip"`
	// DiskPath is the filesystem path where the blob is stored.
	DiskPath string `json:"diskPath" example:"/var/cache/barnacle/hot/sha256/ab/abc123def456"`
	// Tier is the cache tier number where this blob resides.
	Tier int `json:"tier" example:"0"`
	// AccessCount5m is the number of accesses in the last 5 minutes.
	AccessCount5m int64 `json:"accessCount5m" example:"42"`
}

// ListBlobsResponse is the response body for GET /api/v1/nodes/:nodeId/blobs.
type ListBlobsResponse struct {
	// Blobs is a list of all cached blobs on this node.
	Blobs []BlobResponse `json:"blobs"`
}
