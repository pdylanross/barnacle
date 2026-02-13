package configuration

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConfiguration contains Redis connection settings.
type RedisConfiguration struct {
	// Addr is the Redis server address in "host:port" format.
	Addr string `koanf:"addr"`

	// Password is the optional password for Redis authentication.
	Password string `koanf:"password"`

	// DB is the Redis database number to use.
	DB int `koanf:"db"`

	// DialTimeout is the timeout for establishing new connections.
	DialTimeout time.Duration `koanf:"dialTimeout"`

	// ReadTimeout is the timeout for socket reads.
	ReadTimeout time.Duration `koanf:"readTimeout"`

	// WriteTimeout is the timeout for socket writes.
	WriteTimeout time.Duration `koanf:"writeTimeout"`

	// PoolSize is the maximum number of socket connections.
	PoolSize int `koanf:"poolSize"`
}

// Validate checks that the Redis configuration is valid.
func (r *RedisConfiguration) Validate() error {
	if r.Addr == "" {
		return fmt.Errorf("%w: redis addr cannot be empty", ErrInvalidConfiguration)
	}
	if r.DB < 0 {
		return fmt.Errorf("%w: redis db must be non-negative, got %d", ErrInvalidConfiguration, r.DB)
	}
	return nil
}

// BuildClient creates a new Redis client from the configuration.
func (r *RedisConfiguration) BuildClient() *redis.Client {
	opts := &redis.Options{
		Addr:     r.Addr,
		Password: r.Password,
		DB:       r.DB,
	}

	if r.DialTimeout > 0 {
		opts.DialTimeout = r.DialTimeout
	}
	if r.ReadTimeout > 0 {
		opts.ReadTimeout = r.ReadTimeout
	}
	if r.WriteTimeout > 0 {
		opts.WriteTimeout = r.WriteTimeout
	}
	if r.PoolSize > 0 {
		opts.PoolSize = r.PoolSize
	}

	return redis.NewClient(opts)
}

// BuildClientAndPing creates a new Redis client and verifies connectivity.
// Returns an error if the connection cannot be established.
func (r *RedisConfiguration) BuildClientAndPing(ctx context.Context) (*redis.Client, error) {
	client := r.BuildClient()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return client, nil
}
