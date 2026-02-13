package dependencies

import (
	"context"
	"time"

	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/internal/registry"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator/rebalance"
	"github.com/pdylanross/barnacle/internal/tasks"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Dependencies holds all application dependencies.
// All dependencies are eagerly initialized during construction.
type Dependencies struct {
	config           *configuration.Configuration
	logger           *zap.Logger
	taskRunner       *tasks.Runner
	upstreamRegistry *registry.UpstreamRegistry
	redisClient      *redis.Client
	nodeRegistry     *node.Registry
	rebalanceLeader  *rebalance.Leader
	rebalanceWorker  *rebalance.Worker
	queueManager     *rebalance.QueueManager
	inFlightTracker  *rebalance.InFlightTracker
	reservationStore *rebalance.ReservationStore
	cooldownManager  *rebalance.CooldownManager
	nodeClient       *rebalance.NodeClient
}

// NewDependencies creates a new Dependencies instance with all dependencies initialized.
// All dependencies are constructed eagerly during this call.
func NewDependencies(
	ctx context.Context,
	config *configuration.Configuration,
	logger *zap.Logger,
) (*Dependencies, error) {
	taskRunner := tasks.NewRunner(&config.Server, logger)

	redisClient, err := config.Redis.BuildClientAndPing(ctx)
	if err != nil {
		return nil, err
	}
	logger.Info("connected to redis", zap.String("addr", config.Redis.Addr))

	nodeRegistry, err := node.NewRegistry(&config.NodeHealth, &config.Cache, logger, redisClient)
	if err != nil {
		_ = redisClient.Close()
		return nil, err
	}

	// Create rebalance infrastructure components
	queueManager := rebalance.NewQueueManager(redisClient, &config.Rebalance, logger)
	inFlightTracker := rebalance.NewInFlightTracker(redisClient, &config.Rebalance, logger)
	reservationStore := rebalance.NewReservationStore(&config.Rebalance, logger)
	cooldownManager := rebalance.NewCooldownManager(redisClient, &config.Rebalance, logger)
	nodeClient := rebalance.NewNodeClient(30*time.Second, logger) //nolint:mnd // 30s is reasonable for HTTP timeout

	// Create upstream registry with in-flight tracking
	upstreamRegistry, err := registry.NewUpstreamRegistry(config, logger, redisClient, nodeRegistry, inFlightTracker)
	if err != nil {
		_ = redisClient.Close()
		return nil, err
	}

	// Create transfer and worker components
	transferrer := rebalance.NewTransferrer(
		nodeRegistry,
		upstreamRegistry.BlobCache(),
		inFlightTracker,
		queueManager,
		cooldownManager,
		&config.Rebalance,
		logger,
		nodeRegistry.NodeID(),
	)

	rebalanceWorker := rebalance.NewWorker(
		nodeRegistry.NodeID(),
		queueManager,
		transferrer,
		&config.Rebalance,
		logger,
	)

	// Create leader with all dependencies
	rebalanceLeader := rebalance.NewLeader(
		redisClient,
		nodeRegistry.NodeID(),
		logger,
		queueManager,
		nodeRegistry,
		upstreamRegistry.BlobCache(),
		&config.Rebalance,
		&config.Cache,
		cooldownManager,
		nodeClient,
	)

	return &Dependencies{
		config:           config,
		logger:           logger,
		taskRunner:       taskRunner,
		upstreamRegistry: upstreamRegistry,
		redisClient:      redisClient,
		nodeRegistry:     nodeRegistry,
		rebalanceLeader:  rebalanceLeader,
		rebalanceWorker:  rebalanceWorker,
		queueManager:     queueManager,
		inFlightTracker:  inFlightTracker,
		reservationStore: reservationStore,
		cooldownManager:  cooldownManager,
		nodeClient:       nodeClient,
	}, nil
}

// Config returns the application configuration.
func (d *Dependencies) Config() *configuration.Configuration {
	return d.config
}

// Logger returns the application logger.
func (d *Dependencies) Logger() *zap.Logger {
	return d.logger
}

// TaskRunner returns the task runner for managing long-running tasks.
func (d *Dependencies) TaskRunner() *tasks.Runner {
	return d.taskRunner
}

// UpstreamRegistry returns the upstream container registry used for managing and fetching upstream references.
func (d *Dependencies) UpstreamRegistry() *registry.UpstreamRegistry {
	return d.upstreamRegistry
}

// RedisClient returns the Redis client.
func (d *Dependencies) RedisClient() *redis.Client {
	return d.redisClient
}

// NodeRegistry returns the node registry for distributed node tracking.
func (d *Dependencies) NodeRegistry() *node.Registry {
	return d.nodeRegistry
}

// RebalanceLeader returns the rebalance leader election manager.
func (d *Dependencies) RebalanceLeader() *rebalance.Leader {
	return d.rebalanceLeader
}

// RebalanceWorker returns the rebalance worker.
func (d *Dependencies) RebalanceWorker() *rebalance.Worker {
	return d.rebalanceWorker
}

// QueueManager returns the rebalance queue manager.
func (d *Dependencies) QueueManager() *rebalance.QueueManager {
	return d.queueManager
}

// InFlightTracker returns the in-flight request tracker.
func (d *Dependencies) InFlightTracker() *rebalance.InFlightTracker {
	return d.inFlightTracker
}

// ReservationStore returns the reservation store for rebalance transfers.
func (d *Dependencies) ReservationStore() *rebalance.ReservationStore {
	return d.reservationStore
}

// CooldownManager returns the cooldown manager for rebalance cooldowns.
func (d *Dependencies) CooldownManager() *rebalance.CooldownManager {
	return d.cooldownManager
}

// NodeClient returns the node client for remote blob listing.
func (d *Dependencies) NodeClient() *rebalance.NodeClient {
	return d.nodeClient
}

// Close releases all resources held by dependencies.
func (d *Dependencies) Close() error {
	if d.redisClient != nil {
		return d.redisClient.Close()
	}
	return nil
}
