package rebalance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pdylanross/barnacle/pkg/configuration"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Redis key prefix for in-flight request counters.
const keyPrefixInFlight = "barnacle:blob:inflight:"

// InFlightTracker tracks in-flight requests for blobs using Redis counters.
// This enables safe blob deletion by waiting for all active requests to complete.
type InFlightTracker struct {
	redis  *redis.Client
	config *configuration.RebalanceConfiguration
	logger *zap.Logger
}

// NewInFlightTracker creates a new InFlightTracker.
func NewInFlightTracker(
	redisClient *redis.Client,
	config *configuration.RebalanceConfiguration,
	logger *zap.Logger,
) *InFlightTracker {
	return &InFlightTracker{
		redis:  redisClient,
		config: config,
		logger: logger.Named("inflight-tracker"),
	}
}

// inFlightKey returns the Redis key for a blob's in-flight counter.
func inFlightKey(digest string) string {
	return keyPrefixInFlight + digest
}

// Increment increments the in-flight counter for a blob and returns a release function.
// The release function must be called when the request completes.
// The counter has a TTL to prevent orphaned counters from stuck requests.
func (t *InFlightTracker) Increment(ctx context.Context, digest string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := inFlightKey(digest)

	// Increment the counter
	_, err := t.redis.Incr(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to increment in-flight counter: %w", err)
	}

	// Set a TTL to prevent orphaned counters (e.g., if a node crashes mid-request)
	// Use a reasonable timeout that's longer than any expected request
	ttl := 10 * time.Minute //nolint:mnd // 10 minutes is a reasonable TTL for in-flight tracking
	if err = t.redis.Expire(ctx, key, ttl).Err(); err != nil {
		t.logger.Warn("failed to set TTL on in-flight counter",
			zap.String("digest", digest),
			zap.Error(err))
	}

	t.logger.Debug("incremented in-flight counter",
		zap.String("digest", digest))

	// Return a release function that decrements the counter
	released := false
	return func() {
		if released {
			return
		}
		released = true

		// Use a background context since the original context may be cancelled
		//nolint:mnd // 5 seconds is reasonable for release
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, decrErr := t.redis.Decr(releaseCtx, key).Result()
		if decrErr != nil {
			t.logger.Warn("failed to decrement in-flight counter",
				zap.String("digest", digest),
				zap.Error(decrErr))
			return
		}

		// Clean up the key if counter reached zero
		if result <= 0 {
			if delErr := t.redis.Del(releaseCtx, key).Err(); delErr != nil {
				t.logger.Debug("failed to delete zero counter",
					zap.String("digest", digest),
					zap.Error(delErr))
			}
		}

		t.logger.Debug("decremented in-flight counter",
			zap.String("digest", digest),
			zap.Int64("remaining", result))
	}, nil
}

// GetCount returns the current in-flight request count for a blob.
func (t *InFlightTracker) GetCount(ctx context.Context, digest string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	key := inFlightKey(digest)
	result, err := t.redis.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get in-flight count: %w", err)
	}

	return result, nil
}

// WaitForDrain waits until the in-flight counter for a blob reaches zero.
// Returns nil when drained, or an error if the context is cancelled or timeout expires.
func (t *InFlightTracker) WaitForDrain(ctx context.Context, digest string) error {
	timeout := t.config.GetInFlightDrainTimeout()
	deadline := time.Now().Add(timeout)
	pollInterval := 100 * time.Millisecond //nolint:mnd // 100ms is a reasonable poll interval

	t.logger.Debug("waiting for in-flight requests to drain",
		zap.String("digest", digest),
		zap.Duration("timeout", timeout))

	for {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return err
		}

		// Check timeout
		if time.Now().After(deadline) {
			count, _ := t.GetCount(ctx, digest)
			t.logger.Warn("in-flight drain timeout exceeded",
				zap.String("digest", digest),
				zap.Int64("remainingCount", count))
			return fmt.Errorf("timeout waiting for in-flight requests to drain (remaining: %d)", count)
		}

		// Check current count
		count, err := t.GetCount(ctx, digest)
		if err != nil {
			return err
		}

		if count <= 0 {
			t.logger.Debug("in-flight requests drained",
				zap.String("digest", digest))
			return nil
		}

		// Wait before polling again
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
