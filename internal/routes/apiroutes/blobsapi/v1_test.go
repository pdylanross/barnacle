package blobsapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/internal/routes/apiroutes/blobsapi"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	blobdtos "github.com/pdylanross/barnacle/pkg/api/blobsapi"
	"github.com/pdylanross/barnacle/pkg/configuration"
	testutils "github.com/pdylanross/barnacle/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDigest(hex string) string {
	if len(hex) < 64 {
		hex += strings.Repeat("a", 64-len(hex))
	}
	return "sha256:" + hex[:64]
}

func newTestConfig(t *testing.T, mr *miniredis.Miniredis) *configuration.Configuration {
	t.Helper()
	cfg := configuration.Default()
	cfg.Upstreams = map[string]configuration.UpstreamConfiguration{}
	cfg.Cache.Disk.Tiers = []configuration.DiskTierConfiguration{
		{Tier: 0, Path: t.TempDir()},
	}
	cfg.Redis.Addr = mr.Addr()
	return cfg
}

func setupRouter(
	t *testing.T,
	config *configuration.Configuration,
) (*gin.Engine, *dependencies.Dependencies, func()) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	logger := testutils.CreateTestLogger(t)

	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("failed to create dependencies: %v", err)
	}

	router := gin.New()
	router.Use(httptk.HTTPErrorHandler())
	group := router.Group("/api/v1/nodes/:nodeId/blobs")
	blobsapi.RegisterV1(group, deps)

	return router, deps, func() { deps.Close() }
}

// putBlobOnDisk writes a blob directly to the coordinator cache.
func putBlobOnDisk(
	t *testing.T,
	deps *dependencies.Dependencies,
	digest string,
	content []byte,
) {
	t.Helper()
	blobCache := deps.UpstreamRegistry().BlobCache()

	h, err := v1.NewHash(digest)
	require.NoError(t, err)

	desc := &v1.Descriptor{
		Digest:    h,
		Size:      int64(len(content)),
		MediaType: types.DockerLayer,
	}

	// Store directly in tier 0
	err = blobCache.Put(context.Background(), "", "", digest, desc, bytes.NewReader(content),
		&coordinator.CacheLocationDecision{Local: true, Tier: 0})
	require.NoError(t, err)
}

func TestBlobsAPI_List_EmptyCache(t *testing.T) {
	mr := miniredis.RunT(t)
	config := newTestConfig(t, mr)

	router, deps, cleanup := setupRouter(t, config)
	defer cleanup()

	nodeID := deps.NodeRegistry().NodeID()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/"+nodeID+"/blobs/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var result blobdtos.ListBlobsResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Empty(t, result.Blobs)
}

func TestBlobsAPI_List_WithBlobs(t *testing.T) {
	mr := miniredis.RunT(t)
	config := newTestConfig(t, mr)

	router, deps, cleanup := setupRouter(t, config)
	defer cleanup()

	nodeID := deps.NodeRegistry().NodeID()

	// Store some blobs
	digest1 := testDigest("b10b111100000001")
	digest2 := testDigest("b10b222200000002")
	putBlobOnDisk(t, deps, digest1, []byte("content one"))
	putBlobOnDisk(t, deps, digest2, []byte("content two"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/"+nodeID+"/blobs/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var result blobdtos.ListBlobsResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Len(t, result.Blobs, 2)

	foundDigests := make(map[string]bool)
	for _, b := range result.Blobs {
		foundDigests[b.Digest] = true
		assert.Positive(t, b.Size)
		assert.NotEmpty(t, b.DiskPath)
		assert.Equal(t, 0, b.Tier)
	}

	assert.True(t, foundDigests[digest1], "digest1 not found")
	assert.True(t, foundDigests[digest2], "digest2 not found")
}

func TestBlobsAPI_Get_ExistingBlob(t *testing.T) {
	mr := miniredis.RunT(t)
	config := newTestConfig(t, mr)

	router, deps, cleanup := setupRouter(t, config)
	defer cleanup()

	nodeID := deps.NodeRegistry().NodeID()

	digest := testDigest("ae1b10b00000111")
	content := []byte("get blob test content")
	putBlobOnDisk(t, deps, digest, content)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/"+nodeID+"/blobs/"+digest, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var result blobdtos.BlobResponse
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, digest, result.Digest)
	assert.Equal(t, int64(len(content)), result.Size)
	assert.NotEmpty(t, result.DiskPath)
	assert.Equal(t, 0, result.Tier)
}

func TestBlobsAPI_Get_NonExistentBlob(t *testing.T) {
	mr := miniredis.RunT(t)
	config := newTestConfig(t, mr)

	router, deps, cleanup := setupRouter(t, config)
	defer cleanup()

	nodeID := deps.NodeRegistry().NodeID()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/"+nodeID+"/blobs/"+testDigest("00000000e1"), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBlobsAPI_Delete_ExistingBlob(t *testing.T) {
	mr := miniredis.RunT(t)
	config := newTestConfig(t, mr)

	router, deps, cleanup := setupRouter(t, config)
	defer cleanup()

	nodeID := deps.NodeRegistry().NodeID()

	digest := testDigest("de1b10b00000111")
	putBlobOnDisk(t, deps, digest, []byte("delete me"))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/nodes/"+nodeID+"/blobs/"+digest, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify blob is gone by trying to get it
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/"+nodeID+"/blobs/"+digest, nil)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	assert.Equal(t, http.StatusNotFound, getW.Code)
}

func TestBlobsAPI_Delete_NonExistentBlob(t *testing.T) {
	mr := miniredis.RunT(t)
	config := newTestConfig(t, mr)

	router, deps, cleanup := setupRouter(t, config)
	defer cleanup()

	nodeID := deps.NodeRegistry().NodeID()

	// Delete should succeed even for non-existent blobs
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/nodes/"+nodeID+"/blobs/"+testDigest("0000000000e1"), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestBlobsAPI_WrongNodeID(t *testing.T) {
	mr := miniredis.RunT(t)
	config := newTestConfig(t, mr)

	router, _, cleanup := setupRouter(t, config)
	defer cleanup()

	wrongNodeID := "wrong-node-id"

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "List with wrong node",
			method: http.MethodGet,
			path:   "/api/v1/nodes/" + wrongNodeID + "/blobs/",
		},
		{
			name:   "Get with wrong node",
			method: http.MethodGet,
			path:   "/api/v1/nodes/" + wrongNodeID + "/blobs/" + testDigest("any"),
		},
		{
			name:   "Delete with wrong node",
			method: http.MethodDelete,
			path:   "/api/v1/nodes/" + wrongNodeID + "/blobs/" + testDigest("any"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNotFound, w.Code)
		})
	}
}

func TestBlobsAPI_RegisterV1(t *testing.T) {
	mr := miniredis.RunT(t)
	config := newTestConfig(t, mr)

	logger := testutils.CreateTestLogger(t)
	deps, err := dependencies.NewDependencies(context.Background(), config, logger)
	require.NoError(t, err)
	defer deps.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/test/:nodeId/blobs")

	// Should not panic
	blobsapi.RegisterV1(group, deps)

	// Verify routes are registered
	nodeID := deps.NodeRegistry().NodeID()
	req := httptest.NewRequest(http.MethodGet, "/test/"+nodeID+"/blobs/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should get OK (empty list), not 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}
