package rebalance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/pkg/api/rebalanceapi"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"go.uber.org/zap"
)

// Transfer errors.
var (
	// ErrBlobExists indicates the target node already has the blob.
	ErrBlobExists = errors.New("blob already exists on target node")

	// ErrNoCapacity indicates the target node has no storage capacity.
	ErrNoCapacity = errors.New("target node has no storage capacity")

	// ErrTransferFailed indicates the transfer failed.
	ErrTransferFailed = errors.New("blob transfer failed")
)

// Transferrer executes blob transfers between nodes.
type Transferrer struct {
	nodeRegistry    *node.Registry
	blobCache       coordinator.Cache
	inflight        *InFlightTracker
	queue           *QueueManager
	cooldownManager *CooldownManager
	httpClient      *http.Client
	config          *configuration.RebalanceConfiguration
	logger          *zap.Logger
	nodeID          string
}

// NewTransferrer creates a new Transferrer.
func NewTransferrer(
	nodeRegistry *node.Registry,
	blobCache coordinator.Cache,
	inflight *InFlightTracker,
	queue *QueueManager,
	cooldownManager *CooldownManager,
	config *configuration.RebalanceConfiguration,
	logger *zap.Logger,
	nodeID string,
) *Transferrer {
	return &Transferrer{
		nodeRegistry:    nodeRegistry,
		blobCache:       blobCache,
		inflight:        inflight,
		queue:           queue,
		cooldownManager: cooldownManager,
		httpClient: &http.Client{
			Timeout: config.GetTransferTimeout(),
		},
		config: config,
		logger: logger.Named("transferrer"),
		nodeID: nodeID,
	}
}

// Execute performs a blob transfer or deletion as described by the decision.
// For delete-only decisions (when all tiers are full):
//  1. Wait for in-flight requests to drain
//  2. Delete the blob and clear from cache
//
// For cross-node transfers, follows a 4-step handshake:
//  1. Reserve disk space on target node
//  2. Stream blob data to target node
//  3. Finalize (update Redis location)
//  4. Wait for in-flight requests to drain, then delete source
//
// For same-node tier moves (local tier promotion/demotion):
//  1. Move blob between tier directories locally
//  2. Update Redis location
func (t *Transferrer) Execute(ctx context.Context, decision *Decision) error {
	// Check if this is a delete-only decision (all tiers full, evicting lower priority blob)
	if decision.DeleteOnly {
		return t.executeDelete(ctx, decision)
	}

	// Check if this is a local tier move (same node, different tier)
	if decision.SourceNodeID == decision.TargetNodeID {
		return t.executeLocalTierMove(ctx, decision)
	}

	return t.executeRemoteTransfer(ctx, decision)
}

// executeDelete handles deleting a blob when all tiers are full.
// This is used to evict lower priority blobs to make room for higher priority ones.
func (t *Transferrer) executeDelete(ctx context.Context, decision *Decision) error {
	t.logger.Info("starting blob deletion (all tiers full)",
		zap.String("decisionID", decision.ID),
		zap.String("digest", decision.Digest),
		zap.Int("tier", decision.SourceTier))

	// Wait for in-flight requests to drain before deletion
	err := t.inflight.WaitForDrain(ctx, decision.Digest)
	if err != nil {
		t.logger.Warn("in-flight drain timeout, proceeding with deletion",
			zap.String("digest", decision.Digest),
			zap.Error(err))
		// Continue with deletion anyway - it's best effort
	}

	// Delete the blob from local cache and Redis
	// DeleteLocalOnly removes from all local tiers and cleans up Redis
	// (location and access history) if the blob is registered to this node
	err = t.blobCache.DeleteLocalOnly(ctx, decision.Digest)
	if err != nil {
		return fmt.Errorf("failed to delete blob: %w", err)
	}

	t.logger.Info("blob deletion completed (all tiers full)",
		zap.String("decisionID", decision.ID),
		zap.String("digest", decision.Digest))

	return nil
}

// executeLocalTierMove handles moving a blob between tiers on the same node.
// This is a local operation that doesn't require HTTP transfers.
func (t *Transferrer) executeLocalTierMove(ctx context.Context, decision *Decision) error {
	t.logger.Info("starting local tier move",
		zap.String("decisionID", decision.ID),
		zap.String("digest", decision.Digest),
		zap.Int("sourceTier", decision.SourceTier),
		zap.Int("targetTier", decision.TargetTier))

	// For local tier moves, we need to:
	// 1. Read the blob from source tier
	// 2. Write it to target tier
	// 3. Update Redis location
	// 4. Delete from source tier

	// Note: This requires access to per-tier caches, which the coordinator.Cache
	// interface doesn't directly expose. For now, we'll update the Redis location
	// and rely on the cache's natural tier migration to handle the actual file move.
	// A more complete implementation would need direct access to tier caches.

	// Update Redis location to new tier
	if err := t.blobCache.SetLocation(ctx, decision.Digest, decision.TargetNodeID, decision.TargetTier); err != nil {
		return fmt.Errorf("failed to update location: %w", err)
	}

	// Record rebalance timestamp via CooldownManager
	if err := t.cooldownManager.SetCooldown(ctx, decision.Digest); err != nil {
		t.logger.Warn("failed to record rebalance timestamp",
			zap.String("digest", decision.Digest),
			zap.Error(err))
	}

	t.logger.Info("local tier move completed",
		zap.String("decisionID", decision.ID),
		zap.String("digest", decision.Digest),
		zap.Int("targetTier", decision.TargetTier))

	return nil
}

// executeRemoteTransfer handles transferring a blob to a different node.
func (t *Transferrer) executeRemoteTransfer(ctx context.Context, decision *Decision) error {
	t.logger.Info("starting blob transfer",
		zap.String("decisionID", decision.ID),
		zap.String("digest", decision.Digest),
		zap.String("targetNode", decision.TargetNodeID),
		zap.Int64("size", decision.Size))

	// Step 1: Reserve disk space on target node
	reservation, err := t.reserve(ctx, decision)
	if err != nil {
		if errors.Is(err, ErrBlobExists) {
			t.logger.Info("blob already exists on target, skipping transfer",
				zap.String("digest", decision.Digest),
				zap.String("targetNode", decision.TargetNodeID))
			return nil // Success case - blob is already where we want it
		}
		if errors.Is(err, ErrNoCapacity) {
			t.logger.Warn("target node has no capacity, discarding decision",
				zap.String("digest", decision.Digest),
				zap.String("targetNode", decision.TargetNodeID))
			return nil // Discard decision, don't retry
		}
		return fmt.Errorf("reserve failed: %w", err)
	}

	t.logger.Debug("reservation acquired",
		zap.String("reservationID", reservation.ReservationID),
		zap.String("digest", decision.Digest))

	// Step 2: Stream blob data to target node
	err = t.streamBlob(ctx, decision, reservation.ReservationID)
	if err != nil {
		return fmt.Errorf("stream failed: %w", err)
	}

	t.logger.Debug("blob streamed successfully",
		zap.String("digest", decision.Digest))

	// Step 3: Finalize (update Redis location)
	err = t.finalize(ctx, decision, reservation.ReservationID)
	if err != nil {
		return fmt.Errorf("finalize failed: %w", err)
	}

	t.logger.Debug("transfer finalized",
		zap.String("digest", decision.Digest))

	// Step 4: Wait for in-flight requests to drain, then delete source
	err = t.drainAndDelete(ctx, decision)
	if err != nil {
		// Log but don't fail - the blob is already on the target
		t.logger.Warn("drain/delete had issues (blob is on target)",
			zap.String("digest", decision.Digest),
			zap.Error(err))
	}

	// Record rebalance timestamp via CooldownManager
	if err = t.cooldownManager.SetCooldown(ctx, decision.Digest); err != nil {
		t.logger.Warn("failed to record rebalance timestamp",
			zap.String("digest", decision.Digest),
			zap.Error(err))
	}

	t.logger.Info("blob transfer completed",
		zap.String("decisionID", decision.ID),
		zap.String("digest", decision.Digest),
		zap.String("targetNode", decision.TargetNodeID))

	return nil
}

// reserve calls the target node's /reserve endpoint.
func (t *Transferrer) reserve(ctx context.Context, decision *Decision) (*rebalanceapi.ReserveResponse, error) {
	url := fmt.Sprintf("http://%s/api/v1/nodes/%s/blobs/reserve",
		decision.TargetNodeID, decision.TargetNodeID)

	reqBody := rebalanceapi.ReserveRequest{
		Digest:       decision.Digest,
		Size:         decision.Size,
		MediaType:    decision.MediaType,
		SourceNodeID: decision.SourceNodeID,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated:
		var result rebalanceapi.ReserveResponse
		if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &result, nil

	case http.StatusConflict:
		return nil, ErrBlobExists

	case http.StatusInsufficientStorage:
		return nil, ErrNoCapacity

	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", ErrTransferFailed, resp.StatusCode, string(respBody))
	}
}

// streamBlob streams the blob data to the target node's /receive endpoint.
func (t *Transferrer) streamBlob(ctx context.Context, decision *Decision, reservationID string) error {
	// Open the blob from local cache
	reader, err := t.blobCache.Get(ctx, "", "", decision.Digest)
	if err != nil {
		return fmt.Errorf("failed to open local blob: %w", err)
	}
	defer reader.Close()

	url := fmt.Sprintf("http://%s/api/v1/nodes/%s/blobs/receive",
		decision.TargetNodeID, decision.TargetNodeID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Reservation-Id", reservationID)
	req.Header.Set("X-Digest", decision.Digest)
	req.ContentLength = decision.Size

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: status %d: %s", ErrTransferFailed, resp.StatusCode, string(respBody))
	}

	return nil
}

// finalize calls the target node's /finalize endpoint.
func (t *Transferrer) finalize(ctx context.Context, decision *Decision, reservationID string) error {
	url := fmt.Sprintf("http://%s/api/v1/nodes/%s/blobs/finalize",
		decision.TargetNodeID, decision.TargetNodeID)

	reqBody := rebalanceapi.FinalizeRequest{
		ReservationID: reservationID,
		Digest:        decision.Digest,
		Tier:          decision.TargetTier,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: status %d: %s", ErrTransferFailed, resp.StatusCode, string(respBody))
	}

	return nil
}

// drainAndDelete waits for in-flight requests to drain, then deletes the local blob.
func (t *Transferrer) drainAndDelete(ctx context.Context, decision *Decision) error {
	// Wait for in-flight requests to drain
	err := t.inflight.WaitForDrain(ctx, decision.Digest)
	if err != nil {
		t.logger.Warn("in-flight drain timeout, proceeding with deletion",
			zap.String("digest", decision.Digest),
			zap.Error(err))
		// Continue with deletion anyway - it's best effort
	}

	// Delete the local blob
	err = t.blobCache.DeleteLocalOnly(ctx, decision.Digest)
	if err != nil {
		return fmt.Errorf("failed to delete local blob: %w", err)
	}

	t.logger.Debug("deleted local blob after transfer",
		zap.String("digest", decision.Digest))

	return nil
}
