package configuration

import (
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// AuthType is implemented by all authentication mode configurations.
type AuthType interface {
	GetName() string
}

// ErrMultipleAuthTypes is returned when multiple authentication types are configured simultaneously.
var ErrMultipleAuthTypes = errors.New("multiple authentication types configured")

// ErrInvalidAuthConfiguration is returned when an authentication configuration is invalid.
var ErrInvalidAuthConfiguration = errors.New("invalid authentication configuration")

// UpstreamAuthentication defines the authentication configuration for an upstream registry.
// Each upstream is configured for exactly one auth mode. Only one field should be set at a time;
// mutual exclusivity is enforced at validation.
type UpstreamAuthentication struct {
	// Anonymous configures unauthenticated access to the upstream registry.
	Anonymous *AnonymousAuthentication `koanf:"anonymous"`

	// Passthrough configures barnacle to forward client-provided credentials to the upstream registry.
	Passthrough *PassthroughAuthentication `koanf:"passthrough"`
}

// Validate checks that the authentication configuration is valid.
// At most one auth mode may be set. If none are set, a warning is logged
// and behavior defaults to anonymous.
func (u *UpstreamAuthentication) Validate(logger *zap.Logger) error {
	count := 0

	if u.Anonymous != nil {
		count++
	}

	if u.Passthrough != nil {
		count++
	}

	if count > 1 {
		return fmt.Errorf("%w: only one authentication mode may be configured per upstream", ErrMultipleAuthTypes)
	}

	if count == 0 {
		logger.Warn(
			"no authentication mode configured, defaulting to anonymous — set anonymous auth explicitly to suppress this warning",
		)
		return nil
	}

	if u.Anonymous != nil {
		return u.Anonymous.Validate()
	}

	return u.Passthrough.Validate()
}

// GetAuthType returns the configured authentication mode.
// If no mode is explicitly configured, it defaults to anonymous.
func (u *UpstreamAuthentication) GetAuthType() AuthType {
	if u.Passthrough != nil {
		return u.Passthrough
	}

	if u.Anonymous != nil {
		return u.Anonymous
	}

	return &AnonymousAuthentication{}
}

// AnonymousAuthentication configures unauthenticated access to the upstream registry.
type AnonymousAuthentication struct{}

// GetName returns the name of the anonymous authentication type.
func (a *AnonymousAuthentication) GetName() string {
	return "anonymous"
}

// Validate checks that the anonymous authentication configuration is valid.
func (a *AnonymousAuthentication) Validate() error {
	return nil
}

// PassthroughAuthentication configures barnacle to forward client-provided credentials
// directly to the upstream registry without inspection or caching.
type PassthroughAuthentication struct{}

// GetName returns the name of the passthrough authentication type.
func (p *PassthroughAuthentication) GetName() string {
	return "passthrough"
}

// Validate checks that the passthrough authentication configuration is valid.
func (p *PassthroughAuthentication) Validate() error {
	return nil
}
