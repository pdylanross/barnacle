package tasks

import (
	"context"

	"go.uber.org/zap"
)

// OneShot is a task wrapper that executes a function once.
// It respects context cancellation and will not execute if the context is already cancelled.
type OneShot struct {
	fn func(ctx context.Context) error
}

// NewOneShot creates a new one-shot task that executes fn once.
// The function fn will be called immediately when Run is invoked.
func NewOneShot(fn func(ctx context.Context) error) *OneShot {
	return &OneShot{
		fn: fn,
	}
}

// Run executes the one-shot task once and returns.
// If the context is cancelled before execution, it returns immediately without running the function.
func (o *OneShot) Run(ctx context.Context, logger *zap.Logger) error {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		logger.Debug("OneShot task cancelled before execution")
		return nil
	default:
	}

	// Execute the function
	if err := o.fn(ctx); err != nil {
		logger.Error("OneShot task failed", zap.Error(err))
		return err
	}

	logger.Debug("OneShot task completed successfully")
	return nil
}
