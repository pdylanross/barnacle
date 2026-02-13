// Package tk (toolkit) provides common utility functions for the application.
package tk

import (
	"go.uber.org/zap"
)

// HandleDeferError executes a function that returns an error and logs any errors that occur.
// This is useful for defer statements where you want to handle errors without returning them.
//
// Example usage:
//
//	defer utilities.HandleDeferError(file.Close, logger, "closing file")
//	defer utilities.HandleDeferError(db.Close, logger, "closing database connection")
func HandleDeferError(fn func() error, logger *zap.Logger, description string) {
	if err := fn(); err != nil {
		logger.Error("Deferred operation failed", zap.String("operation", description), zap.Error(err))
	}
}

// IgnoreDeferError executes a function that returns an error and silently ignores any errors.
// This is useful for defer statements where errors are expected and harmless (e.g., syncing logger to stderr).
//
// Example usage:
//
//	defer utilities.IgnoreDeferError(logger.Sync)
func IgnoreDeferError(fn func() error) {
	_ = fn()
}
