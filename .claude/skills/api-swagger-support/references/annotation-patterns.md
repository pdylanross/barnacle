# Annotation Patterns Reference

Complete annotation examples matching this project's conventions.

## General API Info Block

This goes in the main entry point file (e.g., `cmd/barnacle/main.go`):

```go
// @title           Barnacle API
// @version         1.0
// @description     Pull-through caching proxy for OCI container registries.
// @description
// @description     Barnacle provides a distributed caching layer in front of upstream
// @description     container registries, with multi-node coordination via Redis.

// @contact.name    Barnacle Support
// @contact.url     https://github.com/pdylanross/barnacle/issues

// @license.name    MIT
// @license.url     https://opensource.org/licenses/MIT

// @host            localhost:8080
// @BasePath        /api/v1
// @schemes         http

// @accept          json
// @produce         json

// @tag.name        nodes
// @tag.description Node lifecycle and health management
// @tag.name        upstreams
// @tag.description Upstream container registry configuration
// @tag.name        blobs
// @tag.description Blob cache inspection and management

// @securityDefinitions.apikey  ApiKeyAuth
// @in                          header
// @name                        Authorization
// @description                 API key authentication
```

Multi-line descriptions use repeated `@description` annotations.

## Handler Patterns

### List Endpoint (GET collection)

```go
// @Summary      List all blobs on a node
// @Description  Returns information about all cached blobs on the specified node,
// @Description  including size, disk path, cache tier, and 5-minute access count.
// @Tags         blobs
// @Produce      json
// @Param        nodeId  path      string  true  "Node identifier"
// @Success      200     {object}  blobsapi.ListBlobsResponse
// @Failure      404     {object}  httptk.ErrorsList  "Node not found"
// @Failure      500     {object}  httptk.ErrorsList  "Internal server error"
// @Router       /nodes/{nodeId}/blobs [get]
func (c *blobsControllerV1) List(ctx *gin.Context) {
```

### Get Single Resource (GET by ID)

```go
// @Summary      Get blob details
// @Description  Returns detailed information about a single cached blob identified by its digest.
// @Tags         blobs
// @Produce      json
// @Param        nodeId  path      string  true  "Node identifier"
// @Param        digest  path      string  true  "Blob digest (e.g., sha256:abc123...)"
// @Success      200     {object}  blobsapi.BlobResponse
// @Failure      404     {object}  httptk.ErrorsList  "Blob or node not found"
// @Failure      500     {object}  httptk.ErrorsList  "Internal server error"
// @Router       /nodes/{nodeId}/blobs/{digest} [get]
func (c *blobsControllerV1) Get(ctx *gin.Context) {
```

### Delete Resource (DELETE)

```go
// @Summary      Delete a blob from a node
// @Description  Removes a blob from all local disk tiers. Redis metadata is only cleaned
// @Description  if Redis indicates the blob is registered to this node.
// @Tags         blobs
// @Param        nodeId  path  string  true  "Node identifier"
// @Param        digest  path  string  true  "Blob digest (e.g., sha256:abc123...)"
// @Success      204     "No Content"
// @Failure      404     {object}  httptk.ErrorsList  "Node not found"
// @Failure      500     {object}  httptk.ErrorsList  "Internal server error"
// @Router       /nodes/{nodeId}/blobs/{digest} [delete]
func (c *blobsControllerV1) Delete(ctx *gin.Context) {
```

### Create Resource (POST with body)

```go
// @Summary      Register a new upstream
// @Description  Adds a new upstream container registry configuration.
// @Tags         upstreams
// @Accept       json
// @Produce      json
// @Param        request  body      upstreamsapi.CreateUpstreamRequest  true  "Upstream configuration"
// @Success      201      {object}  upstreamsapi.GetUpstreamResponse
// @Failure      400      {object}  httptk.ErrorsList  "Invalid request body"
// @Failure      409      {object}  httptk.ErrorsList  "Upstream already exists"
// @Failure      500      {object}  httptk.ErrorsList  "Internal server error"
// @Router       /upstreams [post]
func (c *upstreamsControllerV1) Create(ctx *gin.Context) {
```

### Update Resource (PUT with body)

```go
// @Summary      Update an upstream
// @Description  Updates the configuration for an existing upstream registry.
// @Tags         upstreams
// @Accept       json
// @Produce      json
// @Param        alias    path      string                               true  "Upstream alias"
// @Param        request  body      upstreamsapi.UpdateUpstreamRequest   true  "Updated configuration"
// @Success      200      {object}  upstreamsapi.GetUpstreamResponse
// @Failure      400      {object}  httptk.ErrorsList  "Invalid request body"
// @Failure      404      {object}  httptk.ErrorsList  "Upstream not found"
// @Failure      500      {object}  httptk.ErrorsList  "Internal server error"
// @Router       /upstreams/{alias} [put]
func (c *upstreamsControllerV1) Update(ctx *gin.Context) {
```

### Endpoint with Query Parameters

```go
// @Summary      Search blobs
// @Description  Search for cached blobs with optional filtering and pagination.
// @Tags         blobs
// @Produce      json
// @Param        nodeId     path   string  true   "Node identifier"
// @Param        mediaType  query  string  false  "Filter by media type"
// @Param        tier       query  int     false  "Filter by cache tier"    minimum(0)
// @Param        minSize    query  int     false  "Minimum blob size"       minimum(0)
// @Param        page       query  int     false  "Page number"             default(1) minimum(1)
// @Param        limit      query  int     false  "Results per page"        default(50) minimum(1) maximum(500)
// @Success      200  {object}  blobsapi.ListBlobsResponse
// @Failure      400  {object}  httptk.ErrorsList  "Invalid query parameters"
// @Failure      404  {object}  httptk.ErrorsList  "Node not found"
// @Router       /nodes/{nodeId}/blobs [get]
func (c *blobsControllerV1) Search(ctx *gin.Context) {
```

### Endpoint with Header Parameters

```go
// @Summary      Pull a blob
// @Description  Retrieves blob content from the cache or upstream registry.
// @Tags         distribution
// @Produce      application/octet-stream
// @Param        name            path    string  true   "Repository name"
// @Param        digest          path    string  true   "Blob digest"
// @Param        Accept          header  string  false  "Accepted media types"
// @Param        Range           header  string  false  "Byte range (RFC 7233)"
// @Success      200  {file}    binary
// @Success      206  {file}    binary  "Partial content"
// @Failure      404  {object}  httptk.ErrorsList
// @Header       200  {string}  Content-Type          "Media type of the blob"
// @Header       200  {string}  Docker-Content-Digest "Blob digest"
// @Header       200  {integer} Content-Length         "Blob size in bytes"
// @Router       /v2/{name}/blobs/{digest} [get]
func (c *registryController) GetBlob(ctx *gin.Context) {
```

### Health Check (simple response)

```go
// @Summary      Health check
// @Description  Returns the health status of this node.
// @Tags         system
// @Produce      json
// @Success      200  {object}  map[string]string  "Healthy"
// @Failure      503  {object}  map[string]string  "Unhealthy"
// @Router       /healthz [get]
func (c *healthController) HealthCheck(ctx *gin.Context) {
```

### Deprecated Endpoint

```go
// @Summary      Get upstream (deprecated)
// @Description  Use GET /api/v2/upstreams/{alias} instead.
// @Tags         upstreams
// @Deprecated
// @Produce      json
// @Param        alias  path      string  true  "Upstream alias"
// @Success      200    {object}  upstreamsapi.GetUpstreamResponse
// @Failure      404    {object}  httptk.ErrorsList
// @Router       /upstreams/{alias} [get]
func (c *upstreamsControllerV1) GetDeprecated(ctx *gin.Context) {
```

## Parameter Constraint Annotations

Append after the description string:

```go
// Integer constraints
// @Param  tier   query  int  false  "Cache tier"  minimum(0) maximum(10)
// @Param  limit  query  int  false  "Page size"   default(20) minimum(1) maximum(100)

// String constraints
// @Param  email  query  string  false  "Email"  Format(email)
// @Param  name   query  string  false  "Name"   minLength(1) maxLength(255)

// Enum constraint
// @Param  status  query  string  false  "Status filter"  Enums(active, inactive, pending)

// Collection format for array params
// @Param  ids  query  []int  false  "Filter by IDs"  collectionFormat(csv)
```

## Response Headers

Declare response headers separately from the body:

```go
// @Success  200     {object}  blobsapi.ListBlobsResponse
// @Header   200     {integer} X-Total-Count   "Total number of results"
// @Header   200     {string}  X-Request-Id    "Request trace identifier"
// @Header   200     {string}  Cache-Control   "Caching directive"
```

## Multiple Routers

A handler can serve multiple paths:

```go
// @Router  /nodes/me/blobs [get]
// @Router  /nodes/{nodeId}/blobs [get]
func (c *blobsControllerV1) List(ctx *gin.Context) {
```

## Security Per-Operation

```go
// Public endpoint (no security annotation)
// @Router  /healthz [get]

// Requires API key
// @Security  ApiKeyAuth
// @Router    /nodes [get]

// Requires one of multiple schemes
// @Security  ApiKeyAuth
// @Security  BasicAuth
// @Router    /admin/config [put]

// OAuth2 with specific scopes
// @Security  OAuth2[read, write]
// @Router    /upstreams [post]
```

## Comment Formatting Rules

- One blank comment line (`//`) between annotation groups for readability (optional but encouraged)
- Align values within annotation groups when practical
- `@Description` can span multiple lines using repeated annotations
- `@Param` fields are space-separated, not tab-separated
- Path parameters in `@Router` use `{name}`, not `:name` (Swagger format, not Gin format)
- The `@Router` annotation should always be last
