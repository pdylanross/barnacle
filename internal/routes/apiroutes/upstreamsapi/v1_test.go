package upstreamsapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/routes/apiroutes/upstreamsapi"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	upstreamdtos "github.com/pdylanross/barnacle/pkg/api/upstreamsapi"
	"github.com/pdylanross/barnacle/pkg/configuration"
	testutils "github.com/pdylanross/barnacle/test"
)

// newTestConfig creates a test configuration with proper defaults and the given upstreams.
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

func setupRouter(t *testing.T, config *configuration.Configuration) (*gin.Engine, func()) {
	t.Helper()

	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	logger := testutils.CreateTestLogger(t)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("failed to create dependencies: %v", err)
	}

	router := gin.New()
	router.Use(httptk.HTTPErrorHandler())
	group := router.Group("/api/v1/upstreams")
	upstreamsapi.RegisterV1(group, deps)

	return router, func() { deps.Close() }
}

func TestUpstreamsAPI_List_EmptyConfiguration(t *testing.T) {
	config := newTestConfig(t, nil)

	router, cleanup := setupRouter(t, config)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/upstreams/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result upstreamdtos.ListUpstreamsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(result.Upstreams) != 0 {
		t.Errorf("expected empty upstreams array, got %d items", len(result.Upstreams))
	}
}

func TestUpstreamsAPI_List_WithUpstreams(t *testing.T) {
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
		"quay": {
			Registry: "https://quay.io",
			Authentication: configuration.UpstreamAuthentication{
				Anonymous: &configuration.UpstreamAnonymousAuthentication{},
			},
		},
	})

	router, cleanup := setupRouter(t, config)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/upstreams/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result upstreamdtos.ListUpstreamsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(result.Upstreams) != 3 {
		t.Errorf("expected 3 upstreams, got %d", len(result.Upstreams))
	}

	// Convert to map for order-independent comparison
	upstreams := make(map[string]bool)
	for _, alias := range result.Upstreams {
		upstreams[alias] = true
	}

	expected := []string{"dockerio", "gcr", "quay"}
	for _, alias := range expected {
		if !upstreams[alias] {
			t.Errorf("expected upstream %q not found in response", alias)
		}
	}
}

func TestUpstreamsAPI_Get_ExistingUpstream(t *testing.T) {
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
				Basic: &configuration.UpstreamBasicAuthentication{
					Username: "user",
					Password: "pass",
				},
			},
		},
	})

	router, cleanup := setupRouter(t, config)
	defer cleanup()

	tests := []struct {
		name         string
		alias        string
		wantRegistry string
		wantAuthType string
	}{
		{
			name:         "get dockerio",
			alias:        "dockerio",
			wantRegistry: "https://registry-1.docker.io",
			wantAuthType: "anonymous",
		},
		{
			name:         "get gcr",
			alias:        "gcr",
			wantRegistry: "https://gcr.io",
			wantAuthType: "basic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/upstreams/"+tt.alias, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
			}

			var result upstreamdtos.GetUpstreamResponse
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if result.Alias != tt.alias {
				t.Errorf("expected alias %q, got %q", tt.alias, result.Alias)
			}

			if result.Registry != tt.wantRegistry {
				t.Errorf("expected registry %q, got %q", tt.wantRegistry, result.Registry)
			}

			if result.AuthType != tt.wantAuthType {
				t.Errorf("expected authType %q, got %q", tt.wantAuthType, result.AuthType)
			}
		})
	}
}

func TestUpstreamsAPI_Get_UnknownUpstream(t *testing.T) {
	config := newTestConfig(t, map[string]configuration.UpstreamConfiguration{
		"dockerio": {
			Registry: "https://registry-1.docker.io",
			Authentication: configuration.UpstreamAuthentication{
				Anonymous: &configuration.UpstreamAnonymousAuthentication{},
			},
		},
	})

	router, cleanup := setupRouter(t, config)
	defer cleanup()

	tests := []struct {
		name  string
		alias string
	}{
		{
			name:  "unknown upstream",
			alias: "unknown",
		},
		{
			name:  "nonexistent registry",
			alias: "nonexistent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/upstreams/"+tt.alias, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
			}
		})
	}
}

func TestUpstreamsAPI_Get_EmptyConfiguration(t *testing.T) {
	config := newTestConfig(t, nil)

	router, cleanup := setupRouter(t, config)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/upstreams/any", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestUpstreamsAPI_MethodNotAllowed(t *testing.T) {
	config := newTestConfig(t, map[string]configuration.UpstreamConfiguration{
		"dockerio": {
			Registry: "https://registry-1.docker.io",
			Authentication: configuration.UpstreamAuthentication{
				Anonymous: &configuration.UpstreamAnonymousAuthentication{},
			},
		},
	})

	router, cleanup := setupRouter(t, config)
	defer cleanup()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "POST to list",
			method: http.MethodPost,
			path:   "/api/v1/upstreams/",
		},
		{
			name:   "PUT to list",
			method: http.MethodPut,
			path:   "/api/v1/upstreams/",
		},
		{
			name:   "DELETE to list",
			method: http.MethodDelete,
			path:   "/api/v1/upstreams/",
		},
		{
			name:   "POST to get",
			method: http.MethodPost,
			path:   "/api/v1/upstreams/dockerio",
		},
		{
			name:   "PUT to get",
			method: http.MethodPut,
			path:   "/api/v1/upstreams/dockerio",
		},
		{
			name:   "DELETE to get",
			method: http.MethodDelete,
			path:   "/api/v1/upstreams/dockerio",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d or %d, got %d",
					http.StatusNotFound, http.StatusMethodNotAllowed, w.Code)
			}
		})
	}
}

func TestRegisterV1(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, nil)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("failed to create dependencies: %v", err)
	}
	defer deps.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/test")

	// Should not panic
	upstreamsapi.RegisterV1(group, deps)

	// Verify routes are registered by making a request
	req := httptest.NewRequest(http.MethodGet, "/test/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should get OK or other valid response, not 404
	if w.Code == http.StatusNotFound {
		t.Error("routes were not properly registered")
	}
}
