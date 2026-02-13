package configuration

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrInvalidPort is returned when the server port is invalid.
var ErrInvalidPort = errors.New("invalid port")

// ErrInvalidConfiguration is returned when a configuration value is invalid.
var ErrInvalidConfiguration = errors.New("invalid configuration")

// Default configuration values.
const (
	DefaultServerPort            = 8080
	DefaultRedisAddr             = "localhost:6379"
	DefaultTagLimit              = 10000
	DefaultManifestMemoryLimitMi = 100
	DefaultTagTTL                = 5 * time.Minute
)

// Default returns a new Configuration with all default values set.
func Default() *Configuration {
	return &Configuration{
		Server: ServerConfiguration{
			Port: DefaultServerPort,
		},
		Redis: RedisConfiguration{
			Addr: DefaultRedisAddr,
		},
		Cache: CacheConfiguration{
			Memory: MemoryCacheConfiguration{
				TagLimit:              DefaultTagLimit,
				ManifestMemoryLimitMi: DefaultManifestMemoryLimitMi,
				TagTTL:                DefaultTagTTL,
			},
			Disk: DiskCacheConfiguration{
				Tiers: []DiskTierConfiguration{
					{Tier: 0, Path: DefaultDiskTier0Path},
				},
				DescriptorLimit: DefaultDiskDescriptorLimit,
			},
		},
		Upstreams: nil,
		NodeHealth: NodeHealthConfig{
			SyncInterval: DefaultNodeHealthSyncInterval,
		},
	}
}

// Configuration represents the complete application configuration.
// It contains all settings required for the application to run.
type Configuration struct {
	// Server contains server-specific configuration settings.
	Server ServerConfiguration `koanf:"server"`

	// Redis contains Redis connection settings.
	Redis RedisConfiguration `koanf:"redis"`

	// Cache contains caching layer settings.
	Cache CacheConfiguration `koanf:"cache"`

	// Upstreams defines a list of upstream container registry configurations for connecting and authenticating.
	Upstreams map[string]UpstreamConfiguration `koanf:"upstreams"`

	// NodeHealth contains settings for node health reporting in distributed mode.
	NodeHealth NodeHealthConfig `koanf:"nodeHealth"`

	// Rebalance contains settings for the blob rebalancing system.
	Rebalance RebalanceConfiguration `koanf:"rebalance"`
}

// Validate checks that the configuration is valid.
// Returns an error if any configuration value is invalid.
func (c *Configuration) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return err
	}

	if err := c.Redis.Validate(); err != nil {
		return err
	}

	if err := c.Cache.Validate(); err != nil {
		return err
	}

	// Validate each upstream configuration
	for alias, upstream := range c.Upstreams {
		if alias == "" {
			return fmt.Errorf("%w: upstream alias cannot be empty", ErrInvalidConfiguration)
		}
		if err := upstream.Validate(); err != nil {
			return fmt.Errorf("upstream %q: %w", alias, err)
		}
	}

	if err := c.NodeHealth.Validate(); err != nil {
		return err
	}

	if err := c.Rebalance.Validate(); err != nil {
		return err
	}

	return nil
}

// ServerConfiguration contains server-specific settings.
// It defines how the HTTP server should be configured.
type ServerConfiguration struct {
	// Port is the TCP port the server listens on.
	// Valid ports are 0-65535. Port 0 means the OS will assign a random available port.
	Port int `koanf:"port"`
}

// Validate checks that the server configuration is valid.
// Returns an error if the port is out of valid range (0-65535).
func (s *ServerConfiguration) Validate() error {
	if s.Port < 0 || s.Port > 65535 {
		return fmt.Errorf("%w: port must be between 0 and 65535, got %d", ErrInvalidPort, s.Port)
	}
	return nil
}

// ListenAddr returns the server's listening address based on the configured port.
// The address is in the format ":port", suitable for use with net/http.
func (s *ServerConfiguration) ListenAddr() string {
	return fmt.Sprintf(":%d", s.Port)
}

// BuildHTTP creates and returns a new [http.Server] instance configured with the server's listening address.
// The server is configured with a ReadHeaderTimeout to prevent Slowloris attacks.
func (s *ServerConfiguration) BuildHTTP() *http.Server {
	return &http.Server{
		Addr:              s.ListenAddr(),
		ReadHeaderTimeout: 10 * time.Second, //nolint:mnd // 10 seconds is a standard timeout for header reads
	}
}
