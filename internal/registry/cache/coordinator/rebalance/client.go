package rebalance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/pkg/api/blobsapi"
	"go.uber.org/zap"
)

// NodeClient fetches blob information from remote nodes via HTTP.
type NodeClient struct {
	httpClient *http.Client
	logger     *zap.Logger
}

// NewNodeClient creates a new NodeClient with the specified timeout.
func NewNodeClient(timeout time.Duration, logger *zap.Logger) *NodeClient {
	return &NodeClient{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger.Named("node-client"),
	}
}

// listBlobs fetches blobs from a remote node via GET /api/v1/nodes/:nodeId/blobs.
// Returns the list of blobs on the remote node, or an error if the request fails.
func (c *NodeClient) listBlobs(ctx context.Context, nodeInfo *node.Info) ([]blobInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Build the URL using the node's ID as the address
	url := fmt.Sprintf("http://%s/api/v1/nodes/%s/blobs", nodeInfo.NodeID, nodeInfo.NodeID)

	c.logger.Debug("fetching blobs from remote node",
		zap.String("nodeID", nodeInfo.NodeID),
		zap.String("url", url))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var listResp blobsapi.ListBlobsResponse
	if err = json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert from API response to internal blobInfo type
	blobs := make([]blobInfo, len(listResp.Blobs))
	for i, b := range listResp.Blobs {
		blobs[i] = blobInfo{
			Digest:      b.Digest,
			Size:        b.Size,
			MediaType:   b.MediaType,
			Tier:        b.Tier,
			AccessCount: b.AccessCount5m,
		}
	}

	c.logger.Debug("fetched blobs from remote node",
		zap.String("nodeID", nodeInfo.NodeID),
		zap.Int("blobCount", len(blobs)))

	return blobs, nil
}
