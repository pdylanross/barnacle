package configuration_test

import (
	"errors"
	"testing"

	"github.com/pdylanross/barnacle/pkg/configuration"
	testutils "github.com/pdylanross/barnacle/test"
)

func TestUpstreamConfiguration_Validate(t *testing.T) {
	logger := testutils.CreateTestLogger(t)

	tests := []struct {
		name    string
		config  configuration.UpstreamConfiguration
		wantErr bool
		errType error
	}{
		{
			name: "valid configuration",
			config: configuration.UpstreamConfiguration{
				Registry: "https://registry-1.docker.io",
			},
			wantErr: false,
		},
		{
			name: "empty registry",
			config: configuration.UpstreamConfiguration{
				Registry: "",
			},
			wantErr: true,
			errType: configuration.ErrInvalidAuthConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate(logger)

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
	logger := testutils.CreateTestLogger(t)

	tests := []struct {
		name    string
		auth    configuration.UpstreamAuthentication
		wantErr bool
		errType error
	}{
		{
			name:    "no auth mode set defaults to anonymous",
			auth:    configuration.UpstreamAuthentication{},
			wantErr: false,
		},
		{
			name: "anonymous explicitly set",
			auth: configuration.UpstreamAuthentication{
				Anonymous: &configuration.AnonymousAuthentication{},
			},
			wantErr: false,
		},
		{
			name: "passthrough explicitly set",
			auth: configuration.UpstreamAuthentication{
				Passthrough: &configuration.PassthroughAuthentication{},
			},
			wantErr: false,
		},
		{
			name: "multiple auth modes set",
			auth: configuration.UpstreamAuthentication{
				Anonymous:   &configuration.AnonymousAuthentication{},
				Passthrough: &configuration.PassthroughAuthentication{},
			},
			wantErr: true,
			errType: configuration.ErrMultipleAuthTypes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate(logger)

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
