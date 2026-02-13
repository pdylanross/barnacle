package tk_test

import (
	"errors"
	"testing"

	"github.com/pdylanross/barnacle/internal/tk"
	testutils "github.com/pdylanross/barnacle/test"
)

func TestHandleDeferError_NoError(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	called := false

	fn := func() error {
		called = true
		return nil
	}

	// Should not panic and should call the function
	tk.HandleDeferError(fn, logger, "test operation")

	if !called {
		t.Error("expected function to be called")
	}
}

func TestHandleDeferError_WithError(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	testErr := errors.New("operation failed")
	called := false

	fn := func() error {
		called = true
		return testErr
	}

	// Should not panic and should log the error
	tk.HandleDeferError(fn, logger, "test operation")

	if !called {
		t.Error("expected function to be called")
	}
	// Note: We can't easily verify the log output in the test, but we ensure it doesn't panic
}

func TestHandleDeferError_DeferUsage(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	executed := false

	func() {
		defer tk.HandleDeferError(func() error {
			executed = true
			return nil
		}, logger, "deferred operation")
	}()

	if !executed {
		t.Error("expected deferred function to be executed")
	}
}

func TestHandleDeferError_DeferUsageWithError(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	testErr := errors.New("deferred operation failed")
	executed := false

	func() {
		defer tk.HandleDeferError(func() error {
			executed = true
			return testErr
		}, logger, "deferred operation")
	}()

	if !executed {
		t.Error("expected deferred function to be executed")
	}
}

func TestHandleDeferError_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		fn          func() error
		description string
		shouldCall  bool
	}{
		{
			name: "success case",
			fn: func() error {
				return nil
			},
			description: "successful operation",
			shouldCall:  true,
		},
		{
			name: "error case",
			fn: func() error {
				return errors.New("test error")
			},
			description: "failing operation",
			shouldCall:  true,
		},
		{
			name: "different description",
			fn: func() error {
				return errors.New("another error")
			},
			description: "closing database connection",
			shouldCall:  true,
		},
		{
			name: "empty description",
			fn: func() error {
				return errors.New("error with empty description")
			},
			description: "",
			shouldCall:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testutils.CreateTestLogger(t)
			called := false

			wrappedFn := func() error {
				called = true
				return tt.fn()
			}

			tk.HandleDeferError(wrappedFn, logger, tt.description)

			if tt.shouldCall && !called {
				t.Error("expected function to be called")
			}
		})
	}
}

func TestHandleDeferError_RealWorldScenarios(t *testing.T) {
	t.Run("simulated file close", func(t *testing.T) {
		logger := testutils.CreateTestLogger(t)
		closeError := errors.New("file already closed")

		simulatedClose := func() error {
			return closeError
		}

		// Should not panic
		tk.HandleDeferError(simulatedClose, logger, "closing file")
	})

	t.Run("simulated database close", func(t *testing.T) {
		logger := testutils.CreateTestLogger(t)

		simulatedClose := func() error {
			return nil
		}

		// Should not panic
		tk.HandleDeferError(simulatedClose, logger, "closing database connection")
	})

	t.Run("simulated resource cleanup", func(t *testing.T) {
		logger := testutils.CreateTestLogger(t)
		cleanupError := errors.New("cleanup failed")

		simulatedCleanup := func() error {
			return cleanupError
		}

		// Should not panic
		tk.HandleDeferError(simulatedCleanup, logger, "cleanup temporary resources")
	})
}

func TestHandleDeferError_MultipleDefers(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	executionOrder := make([]int, 0)

	func() {
		defer tk.HandleDeferError(func() error {
			executionOrder = append(executionOrder, 1)
			return nil
		}, logger, "first defer")

		defer tk.HandleDeferError(func() error {
			executionOrder = append(executionOrder, 2)
			return nil
		}, logger, "second defer")

		defer tk.HandleDeferError(func() error {
			executionOrder = append(executionOrder, 3)
			return errors.New("third defer error")
		}, logger, "third defer")
	}()

	// Defers execute in LIFO order
	if len(executionOrder) != 3 {
		t.Fatalf("expected 3 executions, got %d", len(executionOrder))
	}

	if executionOrder[0] != 3 || executionOrder[1] != 2 || executionOrder[2] != 1 {
		t.Errorf("expected execution order [3, 2, 1], got %v", executionOrder)
	}
}

func TestIgnoreDeferError_NoError(t *testing.T) {
	called := false

	fn := func() error {
		called = true
		return nil
	}

	// Should not panic and should call the function
	tk.IgnoreDeferError(fn)

	if !called {
		t.Error("expected function to be called")
	}
}

func TestIgnoreDeferError_WithError(t *testing.T) {
	testErr := errors.New("operation failed")
	called := false

	fn := func() error {
		called = true
		return testErr
	}

	// Should not panic and should silently ignore the error
	tk.IgnoreDeferError(fn)

	if !called {
		t.Error("expected function to be called")
	}
}

func TestIgnoreDeferError_DeferUsage(t *testing.T) {
	executed := false

	func() {
		defer tk.IgnoreDeferError(func() error {
			executed = true
			return nil
		})
	}()

	if !executed {
		t.Error("expected deferred function to be executed")
	}
}

func TestIgnoreDeferError_DeferUsageWithError(t *testing.T) {
	testErr := errors.New("deferred operation failed")
	executed := false

	func() {
		defer tk.IgnoreDeferError(func() error {
			executed = true
			return testErr
		})
	}()

	if !executed {
		t.Error("expected deferred function to be executed")
	}
}

func TestIgnoreDeferError_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		fn         func() error
		shouldCall bool
	}{
		{
			name: "success case",
			fn: func() error {
				return nil
			},
			shouldCall: true,
		},
		{
			name: "error case",
			fn: func() error {
				return errors.New("test error")
			},
			shouldCall: true,
		},
		{
			name: "logger sync simulation",
			fn: func() error {
				return errors.New("sync /dev/stderr: invalid argument")
			},
			shouldCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false

			wrappedFn := func() error {
				called = true
				return tt.fn()
			}

			tk.IgnoreDeferError(wrappedFn)

			if tt.shouldCall && !called {
				t.Error("expected function to be called")
			}
		})
	}
}
