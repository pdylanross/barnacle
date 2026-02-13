package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/pdylanross/barnacle/internal/server"
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

func TestNewServer(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, 8080)

	srv, err := server.NewServer(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer srv.Close()

	if srv == nil {
		t.Fatal("expected Server to be non-nil")
	}
}

func TestNewServer_WithDifferentPorts(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"default port", 8080},
		{"custom port", 9000},
		{"zero port", 0},
		{"high port", 65535},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testutils.CreateTestLogger(t)
			config := newTestConfig(t, tt.port)

			srv, err := server.NewServer(context.Background(), config, logger)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			defer srv.Close()

			if srv == nil {
				t.Fatal("expected Server to be non-nil")
			}
		})
	}
}

func TestServer_Run(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, 0) // Use dynamic port to avoid conflicts

	srv, err := server.NewServer(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("expected no error creating server, got %v", err)
	}
	defer srv.Close()

	// Run the server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Run()
	}()

	// Let the server run for 2 seconds
	time.Sleep(2 * time.Second)

	// Trigger shutdown
	srv.Shutdown()

	// Wait for the server to complete
	err = <-errChan
	if err != nil {
		t.Errorf("expected no error from Run after shutdown, got %v", err)
	}
}
