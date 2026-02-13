package configuration_test

import (
	"errors"
	"testing"

	"github.com/pdylanross/barnacle/pkg/configuration"
)

func TestUpstreamConfiguration_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  configuration.UpstreamConfiguration
		wantErr bool
		errType error
	}{
		{
			name: "valid configuration with anonymous auth",
			config: configuration.UpstreamConfiguration{
				Registry: "https://registry-1.docker.io",
				Authentication: configuration.UpstreamAuthentication{
					Anonymous: &configuration.UpstreamAnonymousAuthentication{},
				},
			},
			wantErr: false,
		},
		{
			name: "valid configuration with basic auth",
			config: configuration.UpstreamConfiguration{
				Registry: "https://registry-1.docker.io",
				Authentication: configuration.UpstreamAuthentication{
					Basic: &configuration.UpstreamBasicAuthentication{
						Username: "user",
						Password: "pass",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid configuration with bearer auth",
			config: configuration.UpstreamConfiguration{
				Registry: "https://registry-1.docker.io",
				Authentication: configuration.UpstreamAuthentication{
					Bearer: &configuration.UpstreamBearerAuthentication{
						Token: "token123",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty registry",
			config: configuration.UpstreamConfiguration{
				Registry: "",
				Authentication: configuration.UpstreamAuthentication{
					Anonymous: &configuration.UpstreamAnonymousAuthentication{},
				},
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
		{
			name: "invalid authentication",
			config: configuration.UpstreamConfiguration{
				Registry: "https://registry-1.docker.io",
				Authentication: configuration.UpstreamAuthentication{
					Basic: &configuration.UpstreamBasicAuthentication{
						Username: "",
						Password: "pass",
					},
				},
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if tt.wantErr && tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("expected error type %v, got %v", tt.errType, err)
			}
		})
	}
}

func TestUpstreamAuthentication_Validate(t *testing.T) {
	tests := []struct {
		name    string
		auth    configuration.UpstreamAuthentication
		wantErr bool
		errType error
	}{
		{
			name: "only anonymous set",
			auth: configuration.UpstreamAuthentication{
				Anonymous: &configuration.UpstreamAnonymousAuthentication{},
			},
			wantErr: false,
		},
		{
			name: "only basic set - valid",
			auth: configuration.UpstreamAuthentication{
				Basic: &configuration.UpstreamBasicAuthentication{
					Username: "user",
					Password: "pass",
				},
			},
			wantErr: false,
		},
		{
			name: "only bearer set - valid",
			auth: configuration.UpstreamAuthentication{
				Bearer: &configuration.UpstreamBearerAuthentication{
					Token: "token123",
				},
			},
			wantErr: false,
		},
		{
			name:    "no auth set - defaults to anonymous",
			auth:    configuration.UpstreamAuthentication{},
			wantErr: false,
		},
		{
			name: "multiple auth types - anonymous and basic",
			auth: configuration.UpstreamAuthentication{
				Anonymous: &configuration.UpstreamAnonymousAuthentication{},
				Basic: &configuration.UpstreamBasicAuthentication{
					Username: "user",
					Password: "pass",
				},
			},
			wantErr: true,
			errType: configuration.ErrMultipleAuthTypes,
		},
		{
			name: "multiple auth types - anonymous and bearer",
			auth: configuration.UpstreamAuthentication{
				Anonymous: &configuration.UpstreamAnonymousAuthentication{},
				Bearer: &configuration.UpstreamBearerAuthentication{
					Token: "token123",
				},
			},
			wantErr: true,
			errType: configuration.ErrMultipleAuthTypes,
		},
		{
			name: "multiple auth types - basic and bearer",
			auth: configuration.UpstreamAuthentication{
				Basic: &configuration.UpstreamBasicAuthentication{
					Username: "user",
					Password: "pass",
				},
				Bearer: &configuration.UpstreamBearerAuthentication{
					Token: "token123",
				},
			},
			wantErr: true,
			errType: configuration.ErrMultipleAuthTypes,
		},
		{
			name: "all auth types set",
			auth: configuration.UpstreamAuthentication{
				Anonymous: &configuration.UpstreamAnonymousAuthentication{},
				Basic: &configuration.UpstreamBasicAuthentication{
					Username: "user",
					Password: "pass",
				},
				Bearer: &configuration.UpstreamBearerAuthentication{
					Token: "token123",
				},
			},
			wantErr: true,
			errType: configuration.ErrMultipleAuthTypes,
		},
		{
			name: "invalid basic auth",
			auth: configuration.UpstreamAuthentication{
				Basic: &configuration.UpstreamBasicAuthentication{
					Username: "",
					Password: "pass",
				},
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
		{
			name: "invalid bearer auth",
			auth: configuration.UpstreamAuthentication{
				Bearer: &configuration.UpstreamBearerAuthentication{
					Token: "",
				},
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate()

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if tt.wantErr && tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("expected error type %v, got %v", tt.errType, err)
			}

			// Verify that no auth defaults to anonymous
			if tt.name == "no auth set - defaults to anonymous" && tt.auth.Anonymous == nil {
				t.Error("expected Anonymous to be set when no auth is configured")
			}
		})
	}
}

func TestUpstreamAnonymousAuthentication_Validate(t *testing.T) {
	auth := configuration.UpstreamAnonymousAuthentication{}
	err := auth.Validate()

	if err != nil {
		t.Errorf("expected no error for anonymous auth, got %v", err)
	}
}

func TestUpstreamBasicAuthentication_Validate(t *testing.T) {
	tests := []struct {
		name    string
		auth    configuration.UpstreamBasicAuthentication
		wantErr bool
		errType error
	}{
		{
			name: "valid credentials",
			auth: configuration.UpstreamBasicAuthentication{
				Username: "user",
				Password: "pass",
			},
			wantErr: false,
		},
		{
			name: "empty username",
			auth: configuration.UpstreamBasicAuthentication{
				Username: "",
				Password: "pass",
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
		{
			name: "empty password",
			auth: configuration.UpstreamBasicAuthentication{
				Username: "user",
				Password: "",
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
		{
			name: "both empty",
			auth: configuration.UpstreamBasicAuthentication{
				Username: "",
				Password: "",
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate()

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if tt.wantErr && tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("expected error type %v, got %v", tt.errType, err)
			}
		})
	}
}

func TestUpstreamBearerAuthentication_Validate(t *testing.T) {
	tests := []struct {
		name    string
		auth    configuration.UpstreamBearerAuthentication
		wantErr bool
		errType error
	}{
		{
			name: "valid token",
			auth: configuration.UpstreamBearerAuthentication{
				Token: "token123",
			},
			wantErr: false,
		},
		{
			name: "empty token",
			auth: configuration.UpstreamBearerAuthentication{
				Token: "",
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate()

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if tt.wantErr && tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("expected error type %v, got %v", tt.errType, err)
			}
		})
	}
}

func TestUpstreamAuthentication_DefaultsToAnonymous(t *testing.T) {
	auth := configuration.UpstreamAuthentication{}

	if auth.Anonymous != nil {
		t.Error("expected Anonymous to be nil before validation")
	}

	err := auth.Validate()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if auth.Anonymous == nil {
		t.Error("expected Anonymous to be set after validation")
	}
}
