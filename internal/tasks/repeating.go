package tasks

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Repeating is a task wrapper that executes a function repeatedly at a fixed interval.
// It respects context cancellation and stops immediately when the context is cancelled.
type Repeating struct {
	interval time.Duration
	fn       func(ctx context.Context) error
}

// NewRepeating creates a new repeating task that executes fn at the specified interval.
// The function fn will be called immediately on first run, then repeatedly at the interval.
func NewRepeating(interval time.Duration, fn func(ctx context.Context) error) *Repeating {
	return &Repeating{
		interval: interval,
		fn:       fn,
	}
}

// Run executes the repeating task until the context is cancelled.
// The function is called immediately on first run, then at each interval thereafter.
func (r *Repeating) Run(ctx context.Context, logger *zap.Logger) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// Execute immediately on first run
	if err := r.fn(ctx); err != nil {
		logger.Error("Repeating task failed", zap.Error(err))
		return err
	}

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Repeating task stopped due to context cancellation")
			return nil
		case <-ticker.C:
			if err := r.fn(ctx); err != nil {
				// Check if error is due to context cancellation (graceful shutdown)
				if ctx.Err() != nil {
					logger.Debug("Repeating task stopped due to context cancellation")
					return nil
				}
				logger.Error("Repeating task failed", zap.Error(err))
				return err
			}
		}
	}
}
