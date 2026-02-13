package test

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// CreateTestLogger creates a zap logger suitable for use in unit tests.
// The logger integrates with Go's testing framework and outputs logs to the test output,
// which are visible when tests fail or when running with -v flag.
// This follows zap's best practices for testing as documented in zaptest package.
func CreateTestLogger(t *testing.T) *zap.Logger {
	return zaptest.NewLogger(t)
}
