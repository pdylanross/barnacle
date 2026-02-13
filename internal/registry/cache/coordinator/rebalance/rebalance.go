// Package rebalance implements leader election via Redis for cache rebalancing.
// A single leader node runs the Rebalance method while non-leaders poll until they win election.
package rebalance

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/internal/tasks"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	redisKey        = "rebalance:leader"
	lockTTL         = 30 * time.Second
	heartbeatTick   = 1 * time.Second
	nonLeaderPoll   = 10 * time.Second
	nonLeaderJitter = 2500 * time.Millisecond
)

// Leader manages leader election for cache rebalancing via Redis.
type Leader struct {
	redisClient     *redis.Client
	nodeID          string
	logger          *zap.Logger
	queueManager    *QueueManager
	nodeRegistry    *node.Registry
	blobCache       coordinator.Cache
	config          *configuration.RebalanceConfiguration
	cacheConfig     *configuration.CacheConfiguration
	cooldownManager *CooldownManager
	nodeClient      *NodeClient
}

// NewLeader creates a new Leader instance.
func NewLeader(
	redisClient *redis.Client,
	nodeID string,
	logger *zap.Logger,
	queueManager *QueueManager,
	nodeRegistry *node.Registry,
	blobCache coordinator.Cache,
	config *configuration.RebalanceConfiguration,
	cacheConfig *configuration.CacheConfiguration,
	cooldownManager *CooldownManager,
	nodeClient *NodeClient,
) *Leader {
	return &Leader{
		redisClient:     redisClient,
		nodeID:          nodeID,
		logger:          logger.Named("rebalance-leader"),
		queueManager:    queueManager,
		nodeRegistry:    nodeRegistry,
		blobCache:       blobCache,
		config:          config,
		cacheConfig:     cacheConfig,
		cooldownManager: cooldownManager,
		nodeClient:      nodeClient,
	}
}

// MakeTask returns a one-shot task that runs the leader election loop.
func (l *Leader) MakeTask() tasks.Task {
	return tasks.NewOneShot(l.run)
}

// run is the main leader election loop.
func (l *Leader) run(ctx context.Context) error {
	// Don't run if rebalancing is disabled
	if !l.config.Enabled {
		l.logger.Info("rebalancing is disabled, leader election not running")
		<-ctx.Done()
		return nil
	}

	var lastRebalance time.Time

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		isLeader, err := l.tryAcquireOrCheckLeadership(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr // intentionally return nil on context cancellation
			}
			l.logger.Error("failed to check leadership", zap.Error(err))
			// Fall through to non-leader wait path on error
			isLeader = false
		}

		if isLeader {
			l.runLeaderLoop(ctx, &lastRebalance)
		} else {
			// Reset lastRebalance when we lose leadership so we rebalance
			// immediately upon becoming leader again
			lastRebalance = time.Time{}

			//nolint:gosec // using weak RNG for jitter is acceptable
			jitter := time.Duration(rand.Int64N(int64(nonLeaderJitter)))
			wait := nonLeaderPoll + jitter
			l.logger.Debug("not leader, waiting before next attempt", zap.Duration("wait", wait))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(wait):
			}
		}
	}
}

// runLeaderLoop runs the leader heartbeat loop, maintaining the lock and
// periodically running rebalance cycles at the configured interval.
func (l *Leader) runLeaderLoop(ctx context.Context, lastRebalance *time.Time) {
	l.logger.Info("acquired leadership", zap.String("node_id", l.nodeID))

	ticker := time.NewTicker(heartbeatTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// Refresh the lock
		isLeader, err := l.tryAcquireOrCheckLeadership(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			l.logger.Error("failed to refresh leadership", zap.Error(err))
			return // Lost leadership due to error, return to main loop
		}
		if !isLeader {
			l.logger.Info("lost leadership")
			return // Return to main loop to retry
		}

		// Check if it's time to run a rebalance cycle
		rebalanceInterval := l.config.GetRebalanceInterval()
		if time.Since(*lastRebalance) >= rebalanceInterval {
			l.logger.Info("running rebalance cycle",
				zap.String("node_id", l.nodeID),
				zap.Duration("interval", rebalanceInterval))

			rebalanceCtx, cancel := context.WithTimeout(ctx, rebalanceInterval)
			l.Rebalance(rebalanceCtx)
			cancel()

			*lastRebalance = time.Now()
		}
	}
}

// tryAcquireOrCheckLeadership attempts to acquire the leader lock via SetNX,
// or checks if this node already holds it.
func (l *Leader) tryAcquireOrCheckLeadership(ctx context.Context) (bool, error) {
	acquired, err := l.redisClient.SetNX(ctx, redisKey, l.nodeID, lockTTL).Result()
	if err != nil {
		return false, err
	}
	if acquired {
		return true, nil
	}

	// Lock exists — check if we hold it
	currentLeader, err := l.redisClient.Get(ctx, redisKey).Result()
	if err != nil {
		return false, err
	}

	if currentLeader != l.nodeID {
		return false, nil
	}

	// We are the current leader — extend the lock TTL
	if expireErr := l.redisClient.Expire(ctx, redisKey, lockTTL).Err(); expireErr != nil {
		return false, expireErr
	}

	return true, nil
}

// Rebalance scans blobs across all nodes and enqueues rebalance decisions.
func (l *Leader) Rebalance(ctx context.Context) {
	l.logger.Info("rebalance cycle started")

	// Create planner with all dependencies
	planner := NewPlanner(
		l.blobCache,
		l.nodeRegistry,
		l.nodeClient,
		l.cooldownManager,
		l.nodeID,
		l.logger,
		l.config,
		l.cacheConfig,
	)

	// Plan returns decisions ready to enqueue
	decisions, err := planner.Plan(ctx)
	if err != nil {
		l.logger.Error("planning failed", zap.Error(err))
		return
	}

	// Enqueue decisions
	var enqueued int
	for _, d := range decisions {
		if enqueueErr := l.queueManager.Enqueue(ctx, d); enqueueErr != nil {
			l.logger.Error("failed to enqueue",
				zap.String("digest", d.Digest),
				zap.Error(enqueueErr))
			continue
		}
		enqueued++
	}

	l.logger.Info("rebalance cycle completed", zap.Int("decisionsEnqueued", enqueued))
}

// blobInfo represents a blob on a node for rebalancing purposes.
type blobInfo struct {
	Digest      string
	Size        int64
	MediaType   string
	Tier        int
	AccessCount int64
}
