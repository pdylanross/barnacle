package configuration

import (
	"errors"
	"fmt"
)

// ErrMultipleAuthTypes is returned when multiple authentication types are configured simultaneously.
var ErrMultipleAuthTypes = errors.New("multiple authentication types configured")

// ErrInvalidAuthConfiguration is returned when an authentication configuration is invalid.
var ErrInvalidAuthConfiguration = errors.New("invalid authentication configuration")

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
func (u *UpstreamConfiguration) Validate() error {
	if u.Registry == "" {
		return fmt.Errorf("%w: registry cannot be empty", ErrInvalidAuthConfiguration)
	}

	return u.Authentication.Validate()
}

// UpstreamAuthentication defines the authentication configuration for an upstream registry.
// Only one authentication type can be set at a time. If none are set, anonymous authentication is used.
type UpstreamAuthentication struct {
	// Anonymous indicates anonymous (unauthenticated) access to the registry.
	Anonymous *UpstreamAnonymousAuthentication `koanf:"anonymous"`

	// Basic specifies HTTP Basic authentication credentials.
	Basic *UpstreamBasicAuthentication `koanf:"basic"`

	// Bearer specifies bearer token authentication.
	Bearer *UpstreamBearerAuthentication `koanf:"bearer"`
}

// Validate ensures that only one authentication type is configured.
// If no authentication is configured, it defaults to anonymous.
// Returns an error if multiple authentication types are set or if the configured type is invalid.
func (u *UpstreamAuthentication) Validate() error {
	setCount := 0

	if u.Anonymous != nil {
		setCount++
	}
	if u.Basic != nil {
		setCount++
	}
	if u.Bearer != nil {
		setCount++
	}

	if setCount > 1 {
		return fmt.Errorf("%w: only one authentication type can be set", ErrMultipleAuthTypes)
	}

	// Default to anonymous if nothing is set
	if setCount == 0 {
		u.Anonymous = &UpstreamAnonymousAuthentication{}
	}

	// Validate the specific authentication type
	if u.Anonymous != nil {
		return u.Anonymous.Validate()
	}
	if u.Basic != nil {
		return u.Basic.Validate()
	}
	if u.Bearer != nil {
		return u.Bearer.Validate()
	}

	return nil
}

// UpstreamAnonymousAuthentication represents anonymous (unauthenticated) access to a registry.
// This is used when no credentials are required.
type UpstreamAnonymousAuthentication struct{}

// Validate checks that the anonymous authentication configuration is valid.
// Anonymous authentication has no fields to validate, so this always returns nil.
func (u *UpstreamAnonymousAuthentication) Validate() error {
	return nil
}

// UpstreamBasicAuthentication represents HTTP Basic authentication credentials.
type UpstreamBasicAuthentication struct {
	// Username is the username for basic authentication.
	Username string `koanf:"username"`

	// Password is the password for basic authentication.
	Password string `koanf:"password"`
}

// Validate checks that the basic authentication configuration is valid.
// Returns an error if username or password is empty.
func (u *UpstreamBasicAuthentication) Validate() error {
	if u.Username == "" {
		return fmt.Errorf("%w: basic auth username cannot be empty", ErrInvalidAuthConfiguration)
	}

	if u.Password == "" {
		return fmt.Errorf("%w: basic auth password cannot be empty", ErrInvalidAuthConfiguration)
	}

	return nil
}

// UpstreamBearerAuthentication represents bearer token authentication.
type UpstreamBearerAuthentication struct {
	// Token is the bearer token to use for authentication.
	Token string `koanf:"token"`
}

// Validate checks that the bearer authentication configuration is valid.
// Returns an error if the token is empty.
func (u *UpstreamBearerAuthentication) Validate() error {
	if u.Token == "" {
		return fmt.Errorf("%w: bearer token cannot be empty", ErrInvalidAuthConfiguration)
	}

	return nil
}
