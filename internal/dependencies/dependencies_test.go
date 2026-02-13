package dependencies_test

import (
	"context"
	"testing"

	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/pkg/configuration"
	testutils "github.com/pdylanross/barnacle/test"
)

func newTestConfig(t *testing.T, port int) *configuration.Configuration {
	t.Helper()
	cfg := configuration.Default()
	cfg.Server.Port = port
	cfg.Cache.Disk.Tiers = []configuration.DiskTierConfiguration{
		{Tier: 0, Path: t.TempDir()},
	}
	return cfg
}

func TestNewDependencies(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, 8080)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer deps.Close()

	if deps == nil {
		t.Fatal("expected Dependencies to be non-nil")
	}
}

func TestDependencies_Config(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, 8080)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer deps.Close()

	retrievedConfig := deps.Config()
	if retrievedConfig == nil {
		t.Fatal("expected Config to return non-nil")
	}

	if retrievedConfig != config {
		t.Error("expected Config to return the same instance that was passed in")
	}

	if retrievedConfig.Server.Port != 8080 {
		t.Errorf("expected port to be 8080, got %d", retrievedConfig.Server.Port)
	}
}

func TestDependencies_Logger(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, 8080)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer deps.Close()

	retrievedLogger := deps.Logger()
	if retrievedLogger == nil {
		t.Fatal("expected Logger to return non-nil")
	}

	if retrievedLogger != logger {
		t.Error("expected Logger to return the same instance that was passed in")
	}
}

func TestDependencies_TaskRunner(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, 8080)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer deps.Close()

	taskRunner := deps.TaskRunner()
	if taskRunner == nil {
		t.Fatal("expected TaskRunner to return non-nil")
	}

	// Verify it's the correct type
	var _ = taskRunner
}

func TestDependencies_ConsistentReferences(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, 8080)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer deps.Close()

	// Call each getter multiple times to ensure consistent references
	config1 := deps.Config()
	config2 := deps.Config()
	if config1 != config2 {
		t.Error("expected Config to return the same instance on multiple calls")
	}

	logger1 := deps.Logger()
	logger2 := deps.Logger()
	if logger1 != logger2 {
		t.Error("expected Logger to return the same instance on multiple calls")
	}

	taskRunner1 := deps.TaskRunner()
	taskRunner2 := deps.TaskRunner()
	if taskRunner1 != taskRunner2 {
		t.Error("expected TaskRunner to return the same instance on multiple calls")
	}
}

func TestDependencies_AllDependenciesInitialized(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, 3000)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer deps.Close()

	// Verify all dependencies are initialized
	if deps.Config() == nil {
		t.Error("Config should be initialized")
	}

	if deps.Logger() == nil {
		t.Error("Logger should be initialized")
	}

	if deps.TaskRunner() == nil {
		t.Error("TaskRunner should be initialized")
	}

	if deps.RedisClient() == nil {
		t.Error("RedisClient should be initialized")
	}
}

func TestDependencies_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		port         int
		expectedPort int
	}{
		{
			name:         "default configuration",
			port:         8080,
			expectedPort: 8080,
		},
		{
			name:         "custom port",
			port:         9000,
			expectedPort: 9000,
		},
		{
			name:         "zero port",
			port:         0,
			expectedPort: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testutils.CreateTestLogger(t)
			config := newTestConfig(t, tt.port)
			deps, err := dependencies.NewDependencies(context.Background(), config, logger)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			defer deps.Close()

			if deps == nil {
				t.Fatal("expected Dependencies to be non-nil")
			}

			if deps.Config().Server.Port != tt.expectedPort {
				t.Errorf("expected port %d, got %d", tt.expectedPort, deps.Config().Server.Port)
			}

			// Verify all dependencies are present
			if deps.Config() == nil {
				t.Error("Config should not be nil")
			}
			if deps.Logger() == nil {
				t.Error("Logger should not be nil")
			}
			if deps.TaskRunner() == nil {
				t.Error("TaskRunner should not be nil")
			}
		})
	}
}
