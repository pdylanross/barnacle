package configuration_test

import (
	"testing"

	"github.com/pdylanross/barnacle/pkg/configuration"
)

func TestAnonymousAuthentication_GetName(t *testing.T) {
	auth := &configuration.AnonymousAuthentication{}
	if got := auth.GetName(); got != "anonymous" {
		t.Errorf("GetName() = %q, want %q", got, "anonymous")
	}
}

func TestPassthroughAuthentication_GetName(t *testing.T) {
	auth := &configuration.PassthroughAuthentication{}
	if got := auth.GetName(); got != "passthrough" {
		t.Errorf("GetName() = %q, want %q", got, "passthrough")
	}
}

func TestUpstreamAuthentication_GetAuthType(t *testing.T) {
	tests := []struct {
		name     string
		auth     configuration.UpstreamAuthentication
		wantName string
	}{
		{
			name:     "no auth configured defaults to anonymous",
			auth:     configuration.UpstreamAuthentication{},
			wantName: "anonymous",
		},
		{
			name: "anonymous explicitly set",
			auth: configuration.UpstreamAuthentication{
				Anonymous: &configuration.AnonymousAuthentication{},
			},
			wantName: "anonymous",
		},
		{
			name: "passthrough explicitly set",
			auth: configuration.UpstreamAuthentication{
				Passthrough: &configuration.PassthroughAuthentication{},
			},
			wantName: "passthrough",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.auth.GetAuthType()
			if got.GetName() != tt.wantName {
				t.Errorf("GetAuthType().GetName() = %q, want %q", got.GetName(), tt.wantName)
			}
		})
	}
}
