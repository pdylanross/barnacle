package mocks

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/pdylanross/barnacle/internal/tasks"
)

// Task is a mock implementation of tasks.Task for testing.
type Task struct {
	RunFunc func(ctx context.Context, logger *zap.Logger) error
}

// NewSuccessfulTask creates a task that completes immediately with no error.
func NewSuccessfulTask() *Task {
	return &Task{
		RunFunc: func(_ context.Context, _ *zap.Logger) error {
			return nil
		},
	}
}

// NewBlockingTask creates a task that blocks until context is cancelled.
// This mock intentionally blocks on ctx.Done() to simulate a long-running task.
func NewBlockingTask() *Task {
	return &Task{
		RunFunc: func(ctx context.Context, _ *zap.Logger) error {
			<-ctx.Done()
			return nil
		},
	}
}

// NewFailingTask creates a task that returns an error immediately.
func NewFailingTask(err error) *Task {
	return &Task{
		RunFunc: func(_ context.Context, _ *zap.Logger) error {
			return err
		},
	}
}

// NewErrorTask creates a task that returns the specified error message.
func NewErrorTask(message string) *Task {
	return NewFailingTask(errors.New(message))
}

// Run executes the mock task's RunFunc if set, otherwise returns nil.
func (m *Task) Run(ctx context.Context, logger *zap.Logger) error {
	if m.RunFunc != nil {
		return m.RunFunc(ctx, logger)
	}
	return nil
}

// Verify interface compliance.
var _ tasks.Task = (*Task)(nil)
