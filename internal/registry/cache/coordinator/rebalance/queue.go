package rebalance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pdylanross/barnacle/pkg/configuration"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Redis key patterns for rebalance operations.
const (
	// keyPrefixQueue is the prefix for per-node rebalance queues.
	// Format: barnacle:rebalance:queue:{nodeId}.
	keyPrefixQueue = "barnacle:rebalance:queue:"
)

// ErrQueueEmpty is returned when the queue is empty after the timeout.
var ErrQueueEmpty = errors.New("queue is empty")

// QueueManager handles Redis queue operations for rebalance decisions.
type QueueManager struct {
	redis  *redis.Client
	config *configuration.RebalanceConfiguration
	logger *zap.Logger
}

// NewQueueManager creates a new QueueManager.
func NewQueueManager(
	redisClient *redis.Client,
	config *configuration.RebalanceConfiguration,
	logger *zap.Logger,
) *QueueManager {
	return &QueueManager{
		redis:  redisClient,
		config: config,
		logger: logger.Named("rebalance-queue"),
	}
}

// queueKey returns the Redis key for a node's rebalance queue.
func queueKey(nodeID string) string {
	return keyPrefixQueue + nodeID
}

// Enqueue adds a rebalance decision to the source node's queue.
// Decisions are pushed to the left (LPUSH) and consumed from the right (BRPOP).
func (q *QueueManager) Enqueue(ctx context.Context, decision *Decision) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("failed to marshal decision: %w", err)
	}

	key := queueKey(decision.SourceNodeID)
	if err = q.redis.LPush(ctx, key, data).Err(); err != nil {
		return fmt.Errorf("failed to enqueue decision: %w", err)
	}

	q.logger.Debug("enqueued rebalance decision",
		zap.String("decisionID", decision.ID),
		zap.String("sourceNode", decision.SourceNodeID),
		zap.String("targetNode", decision.TargetNodeID),
		zap.String("digest", decision.Digest))

	return nil
}

// Dequeue blocks waiting for a rebalance decision from this node's queue.
// Returns ErrQueueEmpty if the timeout expires with no decision available.
func (q *QueueManager) Dequeue(ctx context.Context, nodeID string, timeout time.Duration) (*Decision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := queueKey(nodeID)
	result, err := q.redis.BRPop(ctx, timeout, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrQueueEmpty
	}
	if err != nil {
		return nil, fmt.Errorf("failed to dequeue decision: %w", err)
	}

	// BRPop returns [key, value]
	if len(result) < 2 { //nolint:mnd // BRPop returns exactly 2 elements
		return nil, ErrQueueEmpty
	}

	var decision Decision
	if err = json.Unmarshal([]byte(result[1]), &decision); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decision: %w", err)
	}

	q.logger.Debug("dequeued rebalance decision",
		zap.String("decisionID", decision.ID),
		zap.String("digest", decision.Digest))

	return &decision, nil
}

// QueueLength returns the current length of a node's rebalance queue.
func (q *QueueManager) QueueLength(ctx context.Context, nodeID string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	key := queueKey(nodeID)
	length, err := q.redis.LLen(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get queue length: %w", err)
	}

	return length, nil
}
