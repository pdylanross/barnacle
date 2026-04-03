package configuration

import (
	"fmt"

	"go.uber.org/zap"
)

// UpstreamConfiguration defines the configuration for an upstream container registry.
// It specifies how to connect to and authenticate with the upstream registry.
// The registry alias is stored as the key in the Configuration.Upstreams map.
type UpstreamConfiguration struct {
	// Registry is the URL or hostname of the upstream registry.
	Registry string `koanf:"registry"`

	// Authentication specifies the authentication method to use with the upstream registry.
	Authentication UpstreamAuthentication `koanf:"authentication"`
}

// Validate checks that the upstream configuration is valid.
// Returns an error if the configuration is invalid.
func (u *UpstreamConfiguration) Validate(logger *zap.Logger) error {
	if u.Registry == "" {
		return fmt.Errorf("%w: registry cannot be empty", ErrInvalidAuthConfiguration)
	}

	return u.Authentication.Validate(logger)
}
