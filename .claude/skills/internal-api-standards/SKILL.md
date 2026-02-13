---
name: internal-api-standards
description: Standards for internal management API endpoints, including DTO patterns and response type organization
---

## Internal API Standards

Internal APIs are the management endpoints served under `/api/`. These are distinct from the OCI Distribution API (`/v2/`) and have their own conventions for request/response types.

### DTO Requirement

**ALWAYS define dedicated DTOs (Data Transfer Objects) for API request and response types.** Never return types from `internal/` packages directly in API responses.

- API responses must use types defined in `pkg/api/`
- Controllers must map from internal domain types to DTOs before responding
- Request bodies must bind to dedicated DTO types, then be mapped to internal types

**Why?** Internal types carry implementation details (unexported fields, validation methods, internal tags) that should not leak into the API contract. DTOs provide a stable, versioned API surface decoupled from internal refactoring.

### DTO Package Organization

DTOs live in `pkg/api/` and mirror the route structure under `internal/routes/apiroutes/`:

```
internal/routes/apiroutes/
├── group.go
└── upstreamsapi/
    └── v1.go                    # Controller logic

pkg/api/
└── upstreamsapi/
    └── v1.go                    # DTOs for upstreams API v1
```

Each DTO file corresponds to a controller and version. Name the file to match the version (e.g., `v1.go` for v1 endpoints).

### Defining DTOs

DTOs are plain structs with JSON tags. They do not have `Validate()` methods or `koanf` tags - those belong to configuration and domain types.

```go
package upstreamsapi

// ListUpstreamsResponse is the response body for GET /api/v1/upstreams.
type ListUpstreamsResponse struct {
    Upstreams []string `json:"upstreams"`
}

// GetUpstreamResponse is the response body for GET /api/v1/upstreams/:alias.
type GetUpstreamResponse struct {
    Alias    string `json:"alias"`
    Registry string `json:"registry"`
}
```

Naming conventions:
- Response types: `<Operation><Resource>Response` (e.g., `ListUpstreamsResponse`, `GetUpstreamResponse`)
- Request types: `<Operation><Resource>Request` (e.g., `CreateUpstreamRequest`, `UpdateUpstreamRequest`)

### Mapping in Controllers

Controllers are responsible for mapping between internal types and DTOs:

```go
// GOOD - Map to DTO before responding
func (c *upstreamControllerV1) List(ctx *gin.Context) {
    aliases := c.registry.ListUpstreams()
    ctx.JSON(http.StatusOK, upstreamdtos.ListUpstreamsResponse{
        Upstreams: aliases,
    })
}

// BAD - Returning internal types directly
func (c *upstreamControllerV1) List(ctx *gin.Context) {
    ctx.JSON(http.StatusOK, c.registry.ListUpstreams())
}
```

```go
// GOOD - Bind to DTO, then map to internal type
func (c *controller) Create(ctx *gin.Context) {
    var req dtos.CreateThingRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        _ = ctx.Error(err)
        return
    }
    // Map DTO -> internal type
    thing := mapCreateRequest(req)
    // ...
}

// BAD - Binding directly to internal/configuration types
func (c *controller) Create(ctx *gin.Context) {
    var cfg configuration.SomeConfig
    if err := ctx.ShouldBindJSON(&cfg); err != nil { /* ... */ }
}
```

### What Counts as an Internal Type

Never return any of the following directly in an API response:
- Types from `internal/` packages (e.g., `registry.UpstreamRegistry`, `node.Info`)
- Types from `pkg/configuration/` (e.g., `configuration.UpstreamConfiguration`)
- Domain/model types that carry methods, validation, or non-JSON tags

### Swagger Annotation Requirement

All internal API handler functions **must** have swagger annotations (`@Summary`, `@Tags`, `@Param`, `@Success`, `@Failure`, `@Router`). This is required for every handler that serves an internal API endpoint. The `/v2/` OCI distribution routes are exempt — they are intentionally hidden from swagger documentation.

DTOs should include `example` struct tags on fields to improve the interactive Swagger UI experience:

```go
type GetUpstreamResponse struct {
    Alias    string `json:"alias" example:"dockerhub"`
    Registry string `json:"registry" example:"registry-1.docker.io"`
    AuthType string `json:"authType" example:"anonymous"`
}
```

Regenerate docs after adding/changing annotations: `go generate ./cmd/barnacle/...`

### Versioning

DTO packages are versioned alongside their routes. When a new API version is introduced:
- Create a new DTO file (e.g., `v2.go`) in the same package, or a new subpackage if the surface area is large
- Old DTOs remain for backwards compatibility
- Mapping functions may need to handle version differences