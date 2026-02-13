package tasks_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/pdylanross/barnacle/internal/tasks"
	"github.com/pdylanross/barnacle/pkg/configuration"
	testutils "github.com/pdylanross/barnacle/test"
	"github.com/pdylanross/barnacle/test/mocks"
)

func TestRunner_NoTasks(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{}

	runner := tasks.NewRunner(config, logger)

	// No tasks added, so cancel immediately and wait
	runner.Cancel()
	err := runner.Wait()
	if err != nil {
		t.Errorf("expected no error with no tasks, got %v", err)
	}
}

func TestRunner_SingleSuccessfulTask(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{}

	runner := tasks.NewRunner(config, logger)
	runner.AddTask("test-task", mocks.NewSuccessfulTask())

	err := runner.Wait()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRunner_MultipleSuccessfulTasks(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{}

	runner := tasks.NewRunner(config, logger)
	runner.AddTask("task1", mocks.NewSuccessfulTask())
	runner.AddTask("task2", mocks.NewSuccessfulTask())
	runner.AddTask("task3", mocks.NewSuccessfulTask())

	err := runner.Wait()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRunner_TaskFailure(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{}

	testErr := errors.New("task failed")
	runner := tasks.NewRunner(config, logger)
	runner.AddTask("failing-task", mocks.NewFailingTask(testErr))
	runner.AddTask("blocking-task", mocks.NewBlockingTask())

	err := runner.Wait()

	if err == nil {
		t.Fatal("expected error when task fails, got nil")
	}

	if !errors.Is(err, tasks.ErrTaskFailed) {
		t.Errorf("expected ErrTaskFailed, got %v", err)
	}

	if !errors.Is(err, testErr) {
		t.Errorf("expected wrapped error to contain original error %v, got %v", testErr, err)
	}
}

func TestRunner_ContextCancellation(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{}

	runner := tasks.NewRunner(config, logger)
	runner.AddTask("blocking-task", mocks.NewBlockingTask())

	// Cancel runner's context after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		runner.Cancel()
	}()

	err := runner.Wait()
	if err != nil {
		t.Errorf("expected no error when context is cancelled gracefully, got %v", err)
	}
}

func TestRunner_Shutdown(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{}

	runner := tasks.NewRunner(config, logger)
	runner.AddTask("blocking-task", mocks.NewBlockingTask())

	// Trigger shutdown after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		runner.Shutdown()
	}()

	err := runner.Wait()
	if err != nil {
		t.Errorf("expected no error when shutdown is called gracefully, got %v", err)
	}
}

func TestRunner_AddTask(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{}

	runner := tasks.NewRunner(config, logger)

	// Add both tasks - they start immediately
	runner.AddTask("task1", mocks.NewSuccessfulTask())
	runner.AddTask("task2", mocks.NewFailingTask(errors.New("new task")))

	err := runner.Wait()

	// Should fail because one of the tasks fails
	if err == nil {
		t.Fatal("expected error from failing task, got nil")
	}

	if !errors.Is(err, tasks.ErrTaskFailed) {
		t.Errorf("expected ErrTaskFailed, got %v", err)
	}
}

func TestRunner_ConcurrentTaskExecution(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{}

	executionOrder := make(chan string, 4)

	task1 := &mocks.Task{
		RunFunc: func(_ context.Context, _ *zap.Logger) error {
			executionOrder <- "task1-start"
			time.Sleep(50 * time.Millisecond)
			executionOrder <- "task1-end"
			return nil
		},
	}

	task2 := &mocks.Task{
		RunFunc: func(_ context.Context, _ *zap.Logger) error {
			executionOrder <- "task2-start"
			time.Sleep(50 * time.Millisecond)
			executionOrder <- "task2-end"
			return nil
		},
	}

	runner := tasks.NewRunner(config, logger)
	runner.AddTask("task1", task1)
	runner.AddTask("task2", task2)

	err := runner.Wait()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	close(executionOrder)

	// Verify tasks ran concurrently by checking we got both starts before both ends
	events := make([]string, 0)
	for event := range executionOrder {
		events = append(events, event)
	}

	if len(events) != 4 {
		t.Errorf("expected 4 events, got %d", len(events))
	}

	// Both tasks should have started before either completed (proving concurrency)
	startsBeforeFirstEnd := 0
	for i, event := range events {
		if event == "task1-end" || event == "task2-end" {
			startsBeforeFirstEnd = i
			break
		}
	}

	if startsBeforeFirstEnd < 2 {
		t.Errorf("tasks did not run concurrently: %v", events)
	}
}

func TestRepeating_ImmediateExecution(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	callCount := 0

	fn := func(_ context.Context) error {
		callCount++
		return nil
	}

	repeating := tasks.NewRepeating(100*time.Millisecond, fn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the task in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- repeating.Run(ctx, logger)
	}()

	// Give it a moment to execute immediately
	time.Sleep(10 * time.Millisecond)

	// Cancel to stop the task
	cancel()
	<-done

	// Should have been called at least once (immediately)
	if callCount < 1 {
		t.Errorf("expected function to be called immediately, got %d calls", callCount)
	}
}

func TestRepeating_RepeatsAtInterval(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	callTimes := make([]time.Time, 0)
	var mu sync.Mutex

	fn := func(_ context.Context) error {
		mu.Lock()
		callTimes = append(callTimes, time.Now())
		mu.Unlock()
		return nil
	}

	interval := 50 * time.Millisecond
	repeating := tasks.NewRepeating(interval, fn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the task
	done := make(chan error, 1)
	go func() {
		done <- repeating.Run(ctx, logger)
	}()

	// Let it run for enough time to get multiple executions
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	count := len(callTimes)
	mu.Unlock()

	// Should have been called at least 3 times (immediate + 2 intervals in 200ms with 50ms interval)
	if count < 3 {
		t.Errorf("expected at least 3 calls, got %d", count)
	}

	// Verify intervals are approximately correct (within 20ms tolerance)
	mu.Lock()
	for i := 1; i < len(callTimes); i++ {
		actualInterval := callTimes[i].Sub(callTimes[i-1])
		diff := actualInterval - interval

		if diff < 0 {
			diff = -diff
		}

		if diff > 20*time.Millisecond {
			t.Errorf("interval %d was %v, expected ~%v", i, actualInterval, interval)
		}
	}
	mu.Unlock()
}

func TestRepeating_StopsOnContextCancellation(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	callCount := 0
	var mu sync.Mutex

	fn := func(_ context.Context) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	}

	repeating := tasks.NewRepeating(50*time.Millisecond, fn)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- repeating.Run(ctx, logger)
	}()

	// Let it run for a bit
	time.Sleep(100 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait for completion
	err := <-done
	if err != nil {
		t.Errorf("expected no error on cancellation, got %v", err)
	}

	mu.Lock()
	countAfterCancel := callCount
	mu.Unlock()

	// Wait a bit more to ensure no more calls happen
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	if finalCount != countAfterCancel {
		t.Errorf("expected no more calls after cancellation, but count increased from %d to %d",
			countAfterCancel, finalCount)
	}
}

func TestRepeating_ReturnsErrorOnFailure(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	testErr := errors.New("task execution failed")
	callCount := 0

	fn := func(_ context.Context) error {
		callCount++
		if callCount == 2 {
			return testErr
		}
		return nil
	}

	repeating := tasks.NewRepeating(50*time.Millisecond, fn)

	ctx := context.Background()
	err := repeating.Run(ctx, logger)

	if err == nil {
		t.Fatal("expected error from failing task, got nil")
	}

	if !errors.Is(err, testErr) {
		t.Errorf("expected error to be %v, got %v", testErr, err)
	}

	// Should have been called exactly twice (immediate + one interval before failing)
	if callCount != 2 {
		t.Errorf("expected 2 calls before failure, got %d", callCount)
	}
}

func TestRepeating_FailsOnFirstExecution(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	testErr := errors.New("immediate failure")

	fn := func(_ context.Context) error {
		return testErr
	}

	repeating := tasks.NewRepeating(50*time.Millisecond, fn)

	ctx := context.Background()
	err := repeating.Run(ctx, logger)

	if err == nil {
		t.Fatal("expected error from failing task, got nil")
	}

	if !errors.Is(err, testErr) {
		t.Errorf("expected error to be %v, got %v", testErr, err)
	}
}

func TestRepeating_IntegrationWithRunner(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := &configuration.ServerConfiguration{}
	callCount := 0
	var mu sync.Mutex

	fn := func(_ context.Context) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	}

	repeating := tasks.NewRepeating(50*time.Millisecond, fn)

	runner := tasks.NewRunner(config, logger)
	runner.AddTask("repeating-task", repeating)

	// Let it run for a bit
	time.Sleep(150 * time.Millisecond)

	// Cancel to stop
	runner.Cancel()
	err := runner.Wait()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	// Should have been called at least 2 times (immediate + 1-2 intervals)
	if finalCount < 2 {
		t.Errorf("expected at least 2 calls, got %d", finalCount)
	}
}
