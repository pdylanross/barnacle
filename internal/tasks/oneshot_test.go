package tasks_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pdylanross/barnacle/internal/tasks"
	"github.com/pdylanross/barnacle/pkg/configuration"
	testutils "github.com/pdylanross/barnacle/test"
)

func TestOneShot_ExecutesOnce(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	ctx := context.Background()

	var execCount atomic.Int32
	task := tasks.NewOneShot(func(_ context.Context) error {
		execCount.Add(1)
		return nil
	})

	err := task.Run(ctx, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if execCount.Load() != 1 {
		t.Errorf("expected function to be called exactly once, got %d", execCount.Load())
	}
}

func TestOneShot_ReturnsError(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	ctx := context.Background()

	expectedErr := errors.New("task failed")
	task := tasks.NewOneShot(func(_ context.Context) error {
		return expectedErr
	})

	err := task.Run(ctx, logger)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestOneShot_RespectsContextCancellation(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before execution

	var executed atomic.Bool
	task := tasks.NewOneShot(func(_ context.Context) error {
		executed.Store(true)
		return nil
	})

	err := task.Run(ctx, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if executed.Load() {
		t.Error("expected function not to execute when context is cancelled")
	}
}

func TestOneShot_ContextCancelledDuringExecution(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	task := tasks.NewOneShot(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})

	errChan := make(chan error, 1)
	go func() {
		errChan <- task.Run(ctx, logger)
	}()

	<-started // Wait for task to start
	cancel()  // Cancel context during execution
	err := <-errChan

	if err == nil {
		t.Fatal("expected error from context cancellation, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestOneShot_IntegrationWithRunner(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{
		Port: 8080,
	}
	runner := tasks.NewRunner(config, logger)

	var executed atomic.Bool
	task := tasks.NewOneShot(func(_ context.Context) error {
		executed.Store(true)
		return nil
	})

	runner.AddTask("oneshot-task", task)

	err := runner.Wait()
	if err != nil {
		t.Fatalf("expected no error from runner, got %v", err)
	}

	if !executed.Load() {
		t.Error("expected task to be executed")
	}
}

func TestOneShot_MultipleTasksWithRunner(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{
		Port: 8080,
	}
	runner := tasks.NewRunner(config, logger)

	var count atomic.Int32
	for range 5 {
		task := tasks.NewOneShot(func(_ context.Context) error {
			count.Add(1)
			return nil
		})
		runner.AddTask("oneshot-task", task)
	}

	err := runner.Wait()
	if err != nil {
		t.Fatalf("expected no error from runner, got %v", err)
	}

	if count.Load() != 5 {
		t.Errorf("expected 5 executions, got %d", count.Load())
	}
}

func TestOneShot_FailureStopsRunner(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{
		Port: 8080,
	}
	runner := tasks.NewRunner(config, logger)

	expectedErr := errors.New("task failure")
	failTask := tasks.NewOneShot(func(_ context.Context) error {
		return expectedErr
	})

	blockTask := tasks.NewOneShot(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})

	runner.AddTask("fail-task", failTask)
	runner.AddTask("block-task", blockTask)

	err := runner.Wait()
	if err == nil {
		t.Fatal("expected error from runner, got nil")
	}

	if !errors.Is(err, tasks.ErrTaskFailed) {
		t.Errorf("expected ErrTaskFailed, got %v", err)
	}
}

func TestOneShot_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		fn          func(ctx context.Context) error
		ctxCanceled bool
		expectError bool
		expectExec  bool
	}{
		{
			name: "success",
			fn: func(_ context.Context) error {
				return nil
			},
			ctxCanceled: false,
			expectError: false,
			expectExec:  true,
		},
		{
			name: "error",
			fn: func(_ context.Context) error {
				return errors.New("failed")
			},
			ctxCanceled: false,
			expectError: true,
			expectExec:  true,
		},
		{
			name: "context cancelled before execution",
			fn: func(_ context.Context) error {
				return nil
			},
			ctxCanceled: true,
			expectError: false,
			expectExec:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testutils.CreateTestLogger(t)
			ctx, cancel := context.WithCancel(context.Background())
			if tt.ctxCanceled {
				cancel()
			} else {
				defer cancel()
			}

			var executed atomic.Bool
			task := tasks.NewOneShot(func(ctx context.Context) error {
				executed.Store(true)
				return tt.fn(ctx)
			})

			err := task.Run(ctx, logger)

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if tt.expectExec && !executed.Load() {
				t.Error("expected function to be executed")
			}

			if !tt.expectExec && executed.Load() {
				t.Error("expected function not to be executed")
			}
		})
	}
}

func TestOneShot_ConcurrentExecution(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{
		Port: 8080,
	}
	runner := tasks.NewRunner(config, logger)

	const numTasks = 10
	var count atomic.Int32

	for range numTasks {
		task := tasks.NewOneShot(func(_ context.Context) error {
			time.Sleep(10 * time.Millisecond)
			count.Add(1)
			return nil
		})
		runner.AddTask("concurrent-task", task)
	}

	start := time.Now()
	err := runner.Wait()
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("expected no error from runner, got %v", err)
	}

	if count.Load() != numTasks {
		t.Errorf("expected %d executions, got %d", numTasks, count.Load())
	}

	// Tasks should run concurrently, so total time should be less than sequential execution
	if duration > 50*time.Millisecond {
		t.Logf("tasks may not have run concurrently, took %v", duration)
	}
}
