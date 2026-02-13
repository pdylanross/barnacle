package registry

import (
	"errors"
	"fmt"

	"github.com/pdylanross/barnacle/internal/node"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/internal/registry/upstream"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ErrUnknownUpstream is returned when an upstream registry is not found in the configuration.
var ErrUnknownUpstream = httptk.ErrNameUnknown(errors.New("unknown upstream registry"))

// UpstreamRegistry manages a collection of upstream container registries.
// It provides methods to fetch image references and access configured upstream registries.
type UpstreamRegistry struct {
	upstreams map[string]upstream.Upstream
	blobCache coordinator.Cache
	logger    *zap.Logger
}

// NewUpstreamRegistry creates a new UpstreamRegistry from the given configuration.
// It initializes all configured upstream registries and returns an error if any upstream
// fails to initialize.
func NewUpstreamRegistry(
	config *configuration.Configuration,
	logger *zap.Logger,
	redisClient *redis.Client,
	nodeRegistry *node.Registry,
	inFlightTracker coordinator.InFlightTracker,
) (*UpstreamRegistry, error) {
	namedLogger := logger.Named("upstream")
	upstreams, blobCache, err := buildUpstreams(config, namedLogger, redisClient, nodeRegistry, inFlightTracker)
	if err != nil {
		return nil, err
	}

	return &UpstreamRegistry{
		upstreams: upstreams,
		blobCache: blobCache,
		logger:    namedLogger,
	}, nil
}

// ListUpstreams returns a list of all configured upstream registry aliases.
// The order of aliases in the returned slice is non-deterministic.
func (r *UpstreamRegistry) ListUpstreams() []string {
	upstreams := make([]string, 0, len(r.upstreams))
	for alias := range r.upstreams {
		upstreams = append(upstreams, alias)
	}
	return upstreams
}

// BlobCache returns the coordinator blob cache used for distributed cache management.
func (r *UpstreamRegistry) BlobCache() coordinator.Cache {
	return r.blobCache
}

// GetUpstream retrieves an upstream registry by its alias.
// Returns [ErrUnknownUpstream] if the alias is not configured.
func (r *UpstreamRegistry) GetUpstream(alias string) (upstream.Upstream, error) {
	up, ok := r.upstreams[alias]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownUpstream, alias)
	}
	return up, nil
}
