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

// keyPrefixLastRebalance is the prefix for tracking last rebalance time per blob.
// Format: barnacle:blob:rebalance:{digest}.
const keyPrefixLastRebalance = "barnacle:blob:rebalance:"

// lastRebalanceKey returns the Redis key for a blob's last rebalance timestamp.
func lastRebalanceKey(digest string) string {
	return keyPrefixLastRebalance + digest
}

// CooldownManager tracks when blobs were last rebalanced to prevent thrashing.
// It uses Redis to store cooldown timestamps with automatic expiration.
type CooldownManager struct {
	redis            *redis.Client
	cooldownDuration time.Duration
	logger           *zap.Logger
}

// NewCooldownManager creates a new CooldownManager.
func NewCooldownManager(
	redisClient *redis.Client,
	config *configuration.RebalanceConfiguration,
	logger *zap.Logger,
) *CooldownManager {
	return &CooldownManager{
		redis:            redisClient,
		cooldownDuration: config.GetCooldownDuration(),
		logger:           logger.Named("cooldown-manager"),
	}
}

// IsOnCooldown checks if a blob was rebalanced too recently.
// Returns true if the blob is still within its cooldown period.
func (m *CooldownManager) IsOnCooldown(ctx context.Context, digest string) (bool, error) {
	lastTime, err := m.GetLastRebalanceTime(ctx, digest)
	if err != nil {
		return false, err
	}

	if lastTime.IsZero() {
		return false, nil
	}

	return time.Since(lastTime) < m.cooldownDuration, nil
}

// SetCooldown marks a blob as recently rebalanced.
// This should be called after a successful transfer.
func (m *CooldownManager) SetCooldown(ctx context.Context, digest string) error {
	return m.SetLastRebalanceTime(ctx, digest, time.Now())
}

// GetLastRebalanceTime returns the last time a blob was rebalanced.
// Returns zero time if the blob has never been rebalanced.
func (m *CooldownManager) GetLastRebalanceTime(ctx context.Context, digest string) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}

	key := lastRebalanceKey(digest)
	result, err := m.redis.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get last rebalance time: %w", err)
	}

	var t time.Time
	if err = t.UnmarshalText([]byte(result)); err != nil {
		m.logger.Warn("invalid timestamp in Redis, treating as never rebalanced",
			zap.String("digest", digest),
			zap.String("value", result))
		return time.Time{}, nil //nolint:nilerr // intentionally treat invalid timestamp as never rebalanced
	}

	return t, nil
}

// SetLastRebalanceTime records when a blob was last rebalanced.
// The record expires after the configured cooldown duration.
func (m *CooldownManager) SetLastRebalanceTime(ctx context.Context, digest string, t time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := lastRebalanceKey(digest)
	data, err := t.MarshalText()
	if err != nil {
		return fmt.Errorf("failed to marshal timestamp: %w", err)
	}

	// Set with TTL equal to cooldown duration so old records are automatically cleaned up
	if err = m.redis.Set(ctx, key, data, m.cooldownDuration).Err(); err != nil {
		return fmt.Errorf("failed to set last rebalance time: %w", err)
	}

	m.logger.Debug("recorded rebalance timestamp",
		zap.String("digest", digest),
		zap.Time("timestamp", t))

	return nil
}
