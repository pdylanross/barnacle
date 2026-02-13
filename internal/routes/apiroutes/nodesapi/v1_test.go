package nodesapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/routes/apiroutes/nodesapi"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	nodedtos "github.com/pdylanross/barnacle/pkg/api/nodesapi"
	"github.com/pdylanross/barnacle/pkg/configuration"
	testutils "github.com/pdylanross/barnacle/test"
	"go.uber.org/zap"
)

func newTestConfig(t *testing.T, redisAddr string) *configuration.Configuration {
	t.Helper()
	cfg := configuration.Default()
	cfg.Upstreams = map[string]configuration.UpstreamConfiguration{}
	cfg.Redis.Addr = redisAddr
	cfg.Cache.Disk.Tiers = []configuration.DiskTierConfiguration{
		{Tier: 0, Path: t.TempDir()},
	}
	return cfg
}

func setupRouter(t *testing.T, config *configuration.Configuration) (*gin.Engine, *dependencies.Dependencies) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	logger := testutils.CreateTestLogger(t)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("failed to create dependencies: %v", err)
	}

	router := gin.New()
	router.Use(httptk.HTTPErrorHandler())
	group := router.Group("/api/v1/nodes")
	nodesapi.RegisterV1(group, deps)

	return router, deps
}

func TestNodesAPI_List_Empty(t *testing.T) {
	mr := miniredis.RunT(t)

	config := newTestConfig(t, mr.Addr())
	router, deps := setupRouter(t, config)
	defer deps.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result nodedtos.ListNodesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// No nodes have been synced to Redis, so list should be empty
	if len(result.Nodes) != 0 {
		t.Errorf("expected empty nodes array, got %d items", len(result.Nodes))
	}
}

func TestNodesAPI_List_WithNodes(t *testing.T) {
	mr := miniredis.RunT(t)

	config := newTestConfig(t, mr.Addr())
	router, deps := setupRouter(t, config)
	defer deps.Close()

	// Sync the current node so it appears in the list
	task := deps.NodeRegistry().MakeTask()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	go func() { _ = task.Run(ctx, zap.NewNop()) }()
	time.Sleep(30 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result nodedtos.ListNodesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(result.Nodes) < 1 {
		t.Fatalf("expected at least 1 node, got %d", len(result.Nodes))
	}

	// Verify node has expected fields populated
	found := false
	for _, n := range result.Nodes {
		if n.NodeID != "" {
			found = true
			if n.Status == "" {
				t.Error("expected status to be non-empty")
			}
		}
	}
	if !found {
		t.Error("expected at least one node with a non-empty nodeId")
	}
}

func TestNodesAPI_Me(t *testing.T) {
	mr := miniredis.RunT(t)

	config := newTestConfig(t, mr.Addr())
	router, deps := setupRouter(t, config)
	defer deps.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/me", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result nodedtos.NodeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// NodeID should match the node registry's node ID
	expectedNodeID := deps.NodeRegistry().NodeID()
	if result.NodeID != expectedNodeID {
		t.Errorf("expected nodeId %q, got %q", expectedNodeID, result.NodeID)
	}

	if result.Status == "" {
		t.Error("expected status to be non-empty")
	}

	if result.LastUpdated.IsZero() {
		t.Error("expected lastUpdated to be non-zero")
	}

	// Should have disk usage stats since we configured a tier
	if len(result.Stats.TierDiskUsage) != 1 {
		t.Errorf("expected 1 tier disk usage entry, got %d", len(result.Stats.TierDiskUsage))
	}
}

func TestNodesAPI_Get_ExistingNode(t *testing.T) {
	mr := miniredis.RunT(t)

	config := newTestConfig(t, mr.Addr())
	router, deps := setupRouter(t, config)
	defer deps.Close()

	// Sync the current node to Redis so it can be fetched
	task := deps.NodeRegistry().MakeTask()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	go func() { _ = task.Run(ctx, zap.NewNop()) }()
	time.Sleep(30 * time.Millisecond)

	nodeID := deps.NodeRegistry().NodeID()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/"+nodeID, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var result nodedtos.NodeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.NodeID != nodeID {
		t.Errorf("expected nodeId %q, got %q", nodeID, result.NodeID)
	}

	if result.Status != "healthy" {
		t.Errorf("expected status %q, got %q", "healthy", result.Status)
	}
}

func TestNodesAPI_Get_UnknownNode(t *testing.T) {
	mr := miniredis.RunT(t)

	config := newTestConfig(t, mr.Addr())
	router, deps := setupRouter(t, config)
	defer deps.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/nonexistent-node", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestNodesAPI_MethodNotAllowed(t *testing.T) {
	mr := miniredis.RunT(t)

	config := newTestConfig(t, mr.Addr())
	router, deps := setupRouter(t, config)
	defer deps.Close()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "POST to list",
			method: http.MethodPost,
			path:   "/api/v1/nodes/",
		},
		{
			name:   "PUT to list",
			method: http.MethodPut,
			path:   "/api/v1/nodes/",
		},
		{
			name:   "DELETE to list",
			method: http.MethodDelete,
			path:   "/api/v1/nodes/",
		},
		{
			name:   "POST to me",
			method: http.MethodPost,
			path:   "/api/v1/nodes/me",
		},
		{
			name:   "PUT to me",
			method: http.MethodPut,
			path:   "/api/v1/nodes/me",
		},
		{
			name:   "DELETE to me",
			method: http.MethodDelete,
			path:   "/api/v1/nodes/me",
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
	mr := miniredis.RunT(t)

	logger := testutils.CreateTestLogger(t)
	config := newTestConfig(t, mr.Addr())

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("failed to create dependencies: %v", err)
	}
	defer deps.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/test")

	// Should not panic
	nodesapi.RegisterV1(group, deps)

	// Verify routes are registered by making a request
	req := httptest.NewRequest(http.MethodGet, "/test/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should get OK or other valid response, not 404
	if w.Code == http.StatusNotFound {
		t.Error("routes were not properly registered")
	}
}
