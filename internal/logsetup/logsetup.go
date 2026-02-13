package logsetup

import (
	"github.com/pdylanross/barnacle/internal/tk"
	"go.uber.org/zap"
)

// InitializeLogger creates and returns a zap logger based on the DEVELOPMENT environment variable
// If DEVELOPMENT is set to "true" (case-insensitive), it returns a development logger
// Otherwise, it returns a production logger.
func InitializeLogger() (*zap.Logger, error) {
	isDevelopment := tk.IsDevelopment()

	var logger *zap.Logger
	var err error

	if isDevelopment {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}

	if err != nil {
		return nil, err
	}

	return logger, nil
}
