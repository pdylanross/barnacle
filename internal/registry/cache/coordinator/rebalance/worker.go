package rebalance

import (
	"context"
	"errors"

	"github.com/pdylanross/barnacle/internal/tasks"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"go.uber.org/zap"
)

// Worker consumes rebalance decisions from the queue and executes transfers.
type Worker struct {
	nodeID    string
	queue     *QueueManager
	transfer  *Transferrer
	config    *configuration.RebalanceConfiguration
	logger    *zap.Logger
	semaphore chan struct{}
}

// NewWorker creates a new Worker.
func NewWorker(
	nodeID string,
	queue *QueueManager,
	transfer *Transferrer,
	config *configuration.RebalanceConfiguration,
	logger *zap.Logger,
) *Worker {
	maxConcurrent := config.GetMaxConcurrentTransfers()

	return &Worker{
		nodeID:    nodeID,
		queue:     queue,
		transfer:  transfer,
		config:    config,
		logger:    logger.Named("rebalance-worker"),
		semaphore: make(chan struct{}, maxConcurrent),
	}
}

// MakeTask returns a one-shot task that runs the worker loop.
func (w *Worker) MakeTask() tasks.Task {
	return tasks.NewOneShot(w.run)
}

// run is the main worker loop.
func (w *Worker) run(ctx context.Context) error {
	w.logger.Info("rebalance worker started",
		zap.String("nodeID", w.nodeID),
		zap.Int("maxConcurrent", w.config.GetMaxConcurrentTransfers()))

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("rebalance worker stopping")
			return nil
		default:
		}

		// Block waiting for a decision from the queue
		pollInterval := w.config.GetQueuePollInterval()
		decision, err := w.queue.Dequeue(ctx, w.nodeID, pollInterval)
		if errors.Is(err, ErrQueueEmpty) {
			// Queue is empty, loop back and wait again
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr // intentionally return nil on context cancellation
			}
			w.logger.Error("failed to dequeue decision",
				zap.Error(err))
			continue
		}

		// Process the decision with concurrency limiting
		w.processDecision(ctx, decision)
	}
}

// processDecision executes a transfer for the given decision.
// It uses a semaphore to limit concurrent transfers.
func (w *Worker) processDecision(ctx context.Context, decision *Decision) {
	// Acquire semaphore slot (blocks if at max concurrency)
	select {
	case <-ctx.Done():
		w.logger.Debug("context cancelled while waiting for semaphore",
			zap.String("decisionID", decision.ID))
		return
	case w.semaphore <- struct{}{}:
	}

	// Execute the transfer in a goroutine so we can continue processing queue
	go func() {
		defer func() {
			// Release semaphore slot
			<-w.semaphore
		}()

		w.executeTransfer(ctx, decision)
	}()
}

// executeTransfer performs the actual blob transfer.
func (w *Worker) executeTransfer(ctx context.Context, decision *Decision) {
	w.logger.Debug("executing transfer",
		zap.String("decisionID", decision.ID),
		zap.String("digest", decision.Digest),
		zap.String("targetNode", decision.TargetNodeID))

	err := w.transfer.Execute(ctx, decision)
	if err != nil {
		w.logger.Error("transfer failed",
			zap.String("decisionID", decision.ID),
			zap.String("digest", decision.Digest),
			zap.Error(err))
		return
	}

	w.logger.Info("transfer completed successfully",
		zap.String("decisionID", decision.ID),
		zap.String("digest", decision.Digest),
		zap.String("targetNode", decision.TargetNodeID))
}
