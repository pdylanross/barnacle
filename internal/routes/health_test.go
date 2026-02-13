package routes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/routes"
	"github.com/pdylanross/barnacle/pkg/configuration"
	testutils "github.com/pdylanross/barnacle/test"
)

func newTestConfig(t *testing.T) *configuration.Configuration {
	t.Helper()
	cfg := configuration.Default()
	cfg.Upstreams = map[string]configuration.UpstreamConfiguration{}
	cfg.Cache.Disk.Tiers = []configuration.DiskTierConfiguration{
		{Tier: 0, Path: t.TempDir()},
	}
	return cfg
}

func setupHealthRouter(t *testing.T) (*gin.Engine, func()) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("failed to create dependencies: %v", err)
	}

	router := gin.New()
	controller := routes.NewHealthController(deps)
	controller.RegisterRoutes(router)

	return router, func() { deps.Close() }
}

func TestHealthController_HealthCheck(t *testing.T) {
	router, cleanup := setupHealthRouter(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	expectedStatus := "ok"
	if status, ok := response["status"]; !ok {
		t.Error("response missing 'status' field")
	} else if status != expectedStatus {
		t.Errorf("expected status %q, got %q", expectedStatus, status)
	}
}

func TestHealthController_HealthCheck_ContentType(t *testing.T) {
	router, cleanup := setupHealthRouter(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	expectedContentType := "application/json; charset=utf-8"
	if contentType != expectedContentType {
		t.Errorf("expected Content-Type %q, got %q", expectedContentType, contentType)
	}
}

func TestHealthController_HealthCheck_MethodNotAllowed(t *testing.T) {
	router, cleanup := setupHealthRouter(t)
	defer cleanup()

	tests := []struct {
		name   string
		method string
	}{
		{
			name:   "POST not allowed",
			method: http.MethodPost,
		},
		{
			name:   "PUT not allowed",
			method: http.MethodPut,
		},
		{
			name:   "DELETE not allowed",
			method: http.MethodDelete,
		},
		{
			name:   "PATCH not allowed",
			method: http.MethodPatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/health", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d or %d, got %d",
					http.StatusNotFound, http.StatusMethodNotAllowed, w.Code)
			}
		})
	}
}

func TestNewHealthController(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("failed to create dependencies: %v", err)
	}
	defer deps.Close()

	controller := routes.NewHealthController(deps)
	if controller == nil {
		t.Fatal("expected non-nil controller")
	}
}

func TestHealthController_RegisterRoutes(t *testing.T) {
	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("failed to create dependencies: %v", err)
	}
	defer deps.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := routes.NewHealthController(deps)

	// Should not panic
	controller.RegisterRoutes(router)

	// Verify route is registered
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Error("health route was not properly registered")
	}
}

func TestHealthController_MultipleCalls(t *testing.T) {
	router, cleanup := setupHealthRouter(t)
	defer cleanup()

	// Call the health endpoint multiple times
	for i := range 5 {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("call %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}

		var response map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("call %d: failed to unmarshal response: %v", i+1, err)
		}

		if response["status"] != "ok" {
			t.Errorf("call %d: expected status 'ok', got %q", i+1, response["status"])
		}
	}
}
