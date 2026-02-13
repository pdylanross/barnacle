package tasks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/pdylanross/barnacle/pkg/configuration"
)

// Task represents a long-running task that can be managed by the Runner.
// Tasks should respect context cancellation and exit gracefully when the context is cancelled.
type Task interface {
	// Run executes the task. It should block until the task completes or the context is cancelled.
	// Tasks must check ctx.Err() for cancellation and return promptly when cancelled.
	// The logger parameter is a named logger specific to this task instance.
	Run(ctx context.Context, logger *zap.Logger) error
}

// Runner manages a collection of long-running tasks and handles graceful shutdown.
// The Runner starts in a running state and tasks are started immediately when added.
//
// Note: The Runner stores gCtx (errgroup's derived context) to pass to tasks.
// This is necessary for the errgroup pattern where all tasks share a cancellable context.
type Runner struct {
	logger *zap.Logger
	config *configuration.ServerConfiguration
	cancel context.CancelFunc
	g      *errgroup.Group
	gCtx   context.Context
	mu     sync.Mutex
}

// ErrTaskFailed is returned when a task fails during execution.
var ErrTaskFailed = errors.New("task failed during execution")

// NewRunner creates a new task runner with the given configuration and logger.
// The runner starts immediately in a running state with a background context.
func NewRunner(config *configuration.ServerConfiguration, logger *zap.Logger) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	g, gCtx := errgroup.WithContext(ctx)

	return &Runner{
		logger: logger.Named("taskrunner"),
		config: config,
		cancel: cancel,
		g:      g,
		gCtx:   gCtx,
	}
}

// AddTask adds a task to the runner and starts it immediately.
// The task will be given a named logger based on the provided name.
func (r *Runner) AddTask(name string, task Task) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("Starting task", zap.String("name", name))

	taskName := name
	taskImpl := task
	taskLogger := r.logger.With(zap.String("task", taskName))

	r.g.Go(func() error {
		err := taskImpl.Run(r.gCtx, taskLogger)
		if err != nil {
			r.logger.Error("Task failed", zap.String("name", taskName), zap.Error(err))
			return fmt.Errorf("%w: task '%s': %w", ErrTaskFailed, taskName, err)
		}
		r.logger.Info("Task completed", zap.String("name", taskName))
		return nil
	})
}

// Cancel cancels the runner's context, signaling all tasks to stop.
func (r *Runner) Cancel() {
	r.cancel()
}

// Shutdown initiates a graceful shutdown of all running tasks.
// It cancels the runner's context, signaling all tasks to stop.
// Call Wait() after Shutdown() to wait for all tasks to complete.
func (r *Runner) Shutdown() {
	r.logger.Info("Shutdown requested, signaling all tasks to stop")
	r.cancel()
}

// Wait blocks until either:
// - All tasks complete successfully
// - An OS signal (SIGTERM/SIGINT) is received
// - Any task returns an error
//
// When shutdown is triggered, Wait will:
// 1. Cancel the context to signal all tasks to stop
// 2. Wait for all tasks to complete
// 3. If a task fails, return ErrTaskFailed with the underlying error.
func (r *Runner) Wait() error {
	r.logger.Info("Waiting for tasks to complete")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigChan)

	// Wait for completion or signal
	errChan := make(chan error, 1)
	go func() {
		errChan <- r.g.Wait()
	}()

	select {
	case sig := <-sigChan:
		r.logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
		r.cancel() // Signal all tasks to stop

		// Wait for tasks to complete after cancellation
		if err := <-errChan; err != nil {
			r.logger.Error("Task error during shutdown", zap.Error(err))
			return err
		}
		r.logger.Info("All tasks completed successfully after shutdown signal")
		return nil

	case err := <-errChan:
		if err != nil {
			r.logger.Error("Task error triggered shutdown", zap.Error(err))
			return err
		}

		r.logger.Info("All tasks completed successfully")
		return nil
	}
}
