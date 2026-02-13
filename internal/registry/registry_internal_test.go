package registry

import (
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/pkg/configuration"
	testutils "github.com/pdylanross/barnacle/test"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// setupTestRedis creates a miniredis instance and returns a client and cleanup function.
func setupTestRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return client, func() {
		client.Close()
		mr.Close()
	}
}

// newTestConfig creates a test configuration with proper cache settings.
// If upstreams is nil, an empty map is used.
func newTestConfig(
	t *testing.T,
	upstreams map[string]configuration.UpstreamConfiguration,
) *configuration.Configuration {
	t.Helper()
	cfg := configuration.Default()
	if upstreams == nil {
		cfg.Upstreams = map[string]configuration.UpstreamConfiguration{}
	} else {
		cfg.Upstreams = upstreams
	}
	cfg.Cache.Disk.Tiers = []configuration.DiskTierConfiguration{
		{Tier: 0, Path: t.TempDir()},
	}
	return cfg
}

// newTestNodeRegistry creates a node.Registry backed by the given Redis client for testing.
func newTestNodeRegistry(t *testing.T, redisClient *redis.Client) *node.Registry {
	t.Helper()
	cfg := &configuration.NodeHealthConfig{NodeID: "test-node"}
	cacheCfg := &configuration.CacheConfiguration{}
	nr, err := node.NewRegistry(cfg, cacheCfg, zap.NewNop(), redisClient)
	if err != nil {
		t.Fatalf("failed to create test node registry: %v", err)
	}
	return nr
}

func TestNewUpstreamRegistry(t *testing.T) {
	tests := []struct {
		name      string
		upstreams map[string]configuration.UpstreamConfiguration
		wantErr   bool
	}{
		{
			name:      "empty configuration",
			upstreams: nil,
			wantErr:   false,
		},
		{
			name: "single upstream",
			upstreams: map[string]configuration.UpstreamConfiguration{
				"dockerio": {
					Registry: "https://registry-1.docker.io",
					Authentication: configuration.UpstreamAuthentication{
						Anonymous: &configuration.UpstreamAnonymousAuthentication{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple upstreams",
			upstreams: map[string]configuration.UpstreamConfiguration{
				"dockerio": {
					Registry: "https://registry-1.docker.io",
					Authentication: configuration.UpstreamAuthentication{
						Anonymous: &configuration.UpstreamAnonymousAuthentication{},
					},
				},
				"gcr": {
					Registry: "https://gcr.io",
					Authentication: configuration.UpstreamAuthentication{
						Basic: &configuration.UpstreamBasicAuthentication{
							Username: "user",
							Password: "pass",
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testutils.CreateTestLogger(t)
			config := newTestConfig(t, tt.upstreams)
			redisClient, cleanup := setupTestRedis(t)
			defer cleanup()

			registry, err := NewUpstreamRegistry(config, logger, redisClient, newTestNodeRegistry(t, redisClient), nil)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if registry == nil {
				t.Fatal("expected non-nil registry")
			}

			if registry.upstreams == nil {
				t.Error("expected non-nil upstreams map")
			}

			if registry.logger == nil {
				t.Error("expected non-nil logger")
			}

			expectedCount := len(config.Upstreams)
			if len(registry.upstreams) != expectedCount {
				t.Errorf("expected %d upstreams, got %d", expectedCount, len(registry.upstreams))
			}
		})
	}
}

func TestUpstreamRegistry_ListUpstreams(t *testing.T) {
	tests := []struct {
		name      string
		upstreams map[string]configuration.UpstreamConfiguration
		wantList  []string
	}{
		{
			name:      "empty configuration",
			upstreams: nil,
			wantList:  []string{},
		},
		{
			name: "single upstream",
			upstreams: map[string]configuration.UpstreamConfiguration{
				"dockerio": {
					Registry: "https://registry-1.docker.io",
					Authentication: configuration.UpstreamAuthentication{
						Anonymous: &configuration.UpstreamAnonymousAuthentication{},
					},
				},
			},
			wantList: []string{"dockerio"},
		},
		{
			name: "multiple upstreams",
			upstreams: map[string]configuration.UpstreamConfiguration{
				"dockerio": {
					Registry: "https://registry-1.docker.io",
					Authentication: configuration.UpstreamAuthentication{
						Anonymous: &configuration.UpstreamAnonymousAuthentication{},
					},
				},
				"gcr": {
					Registry: "https://gcr.io",
					Authentication: configuration.UpstreamAuthentication{
						Anonymous: &configuration.UpstreamAnonymousAuthentication{},
					},
				},
				"quay": {
					Registry: "https://quay.io",
					Authentication: configuration.UpstreamAuthentication{
						Anonymous: &configuration.UpstreamAnonymousAuthentication{},
					},
				},
			},
			wantList: []string{"dockerio", "gcr", "quay"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testutils.CreateTestLogger(t)
			config := newTestConfig(t, tt.upstreams)
			redisClient, cleanup := setupTestRedis(t)
			defer cleanup()

			registry, err := NewUpstreamRegistry(config, logger, redisClient, newTestNodeRegistry(t, redisClient), nil)
			if err != nil {
				t.Fatalf("failed to create registry: %v", err)
			}

			list := registry.ListUpstreams()

			if len(list) != len(tt.wantList) {
				t.Errorf("expected %d upstreams, got %d", len(tt.wantList), len(list))
			}

			// Convert to map for order-independent comparison
			listMap := make(map[string]bool)
			for _, alias := range list {
				listMap[alias] = true
			}

			for _, want := range tt.wantList {
				if !listMap[want] {
					t.Errorf("expected upstream %q not found in list", want)
				}
			}
		})
	}
}

func TestUpstreamRegistry_GetUpstream(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, map[string]configuration.UpstreamConfiguration{
		"dockerio": {
			Registry: "https://registry-1.docker.io",
			Authentication: configuration.UpstreamAuthentication{
				Anonymous: &configuration.UpstreamAnonymousAuthentication{},
			},
		},
		"gcr": {
			Registry: "https://gcr.io",
			Authentication: configuration.UpstreamAuthentication{
				Anonymous: &configuration.UpstreamAnonymousAuthentication{},
			},
		},
	})
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	registry, err := NewUpstreamRegistry(config, logger, redisClient, newTestNodeRegistry(t, redisClient), nil)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	tests := []struct {
		name            string
		alias           string
		wantErr         bool
		wantErrType     error
		wantErrContains string
	}{
		{
			name:    "valid upstream - dockerio",
			alias:   "dockerio",
			wantErr: false,
		},
		{
			name:    "valid upstream - gcr",
			alias:   "gcr",
			wantErr: false,
		},
		{
			name:            "unknown upstream",
			alias:           "unknown",
			wantErr:         true,
			wantErrType:     ErrUnknownUpstream,
			wantErrContains: "unknown",
		},
		{
			name:            "empty alias",
			alias:           "",
			wantErr:         true,
			wantErrType:     ErrUnknownUpstream,
			wantErrContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream, getErr := registry.GetUpstream(tt.alias)

			if tt.wantErr {
				if getErr == nil {
					t.Error("expected error, got nil")
					return
				}

				if tt.wantErrType != nil && !errors.Is(getErr, tt.wantErrType) {
					t.Errorf("expected error type %v, got %v", tt.wantErrType, getErr)
				}

				if tt.wantErrContains != "" && !contains(getErr.Error(), tt.wantErrContains) {
					t.Errorf("expected error to contain %q, got %q", tt.wantErrContains, getErr.Error())
				}

				return
			}

			if getErr != nil {
				t.Errorf("unexpected error: %v", getErr)
				return
			}

			if upstream == nil {
				t.Fatal("expected non-nil upstream")
			}
		})
	}
}

func TestUpstreamRegistry_EmptyConfiguration(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, nil)
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	registry, err := NewUpstreamRegistry(config, logger, redisClient, newTestNodeRegistry(t, redisClient), nil)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Test that ListUpstreams returns empty slice
	list := registry.ListUpstreams()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}

	// Test that GetUpstream returns error
	_, err = registry.GetUpstream("any")
	if err == nil {
		t.Error("expected error for any upstream in empty registry")
	}
	if !errors.Is(err, ErrUnknownUpstream) {
		t.Errorf("expected ErrUnknownUpstream, got %v", err)
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	if substr == "" {
		return true
	}
	return len(s) >= len(substr) && indexOfString(s, substr) >= 0
}

// indexOfString returns the index of the first instance of substr in s, or -1 if not present.
func indexOfString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
