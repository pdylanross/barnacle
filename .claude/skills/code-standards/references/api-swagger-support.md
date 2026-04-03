# API Swagger Support (swag + gin-swagger)

This project uses [swag](https://github.com/swaggo/swag) to generate Swagger 2.0 documentation from Go annotations, and [gin-swagger](https://github.com/swaggo/gin-swagger) to serve the interactive Swagger UI.

## Setup

### Installation

```bash
# Install the swag CLI
go install github.com/swaggo/swag/cmd/swag@latest

# Project dependencies
go get -u github.com/swaggo/swag
go get -u github.com/swaggo/gin-swagger
go get -u github.com/swaggo/files
```

### Generate Documentation

```bash
# From project root — generates docs/ folder with docs.go, swagger.json, swagger.yaml
swag init

# If the general API info is not in main.go
swag init -g cmd/barnacle/main.go

# Parse multiple directories
swag init -g cmd/barnacle/main.go -d ./,./internal/routes/

# Format swagger comments
swag fmt
```

### Serve Swagger UI in Gin

```go
import (
    swaggerFiles "github.com/swaggo/files"
    ginSwagger "github.com/swaggo/gin-swagger"
    _ "github.com/pdylanross/barnacle/docs" // Generated docs
)

// Register the swagger route
router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
```

Access at: `http://localhost:8080/swagger/index.html`

## Annotation Workflow

1. Add **general API info** annotations to the main entry point (or file passed to `-g`)
2. Add **operation annotations** above each handler function
3. Use **struct tags** on DTOs to control field documentation
4. Run `swag init` to regenerate
5. Run `swag fmt` to auto-format comment alignment

## General API Info

Place these annotations in the main file (or the file specified with `-g`). They define the top-level Swagger metadata.

```go
// @title           Barnacle API
// @version         1.0
// @description     Pull-through caching proxy for OCI container registries.

// @contact.name    API Support
// @contact.url     https://github.com/pdylanross/barnacle

// @license.name    MIT
// @license.url     https://opensource.org/licenses/MIT

// @host            localhost:8080
// @BasePath        /api/v1

// @schemes         http https
// @accept          json
// @produce         json

// @tag.name        nodes
// @tag.description Node management endpoints
// @tag.name        upstreams
// @tag.description Upstream registry management
// @tag.name        blobs
// @tag.description Blob cache management
```

### Dynamic Configuration

Override generated values at runtime:

```go
import "github.com/pdylanross/barnacle/docs"

docs.SwaggerInfo.Title = "Barnacle API"
docs.SwaggerInfo.Host = actualHost
docs.SwaggerInfo.BasePath = "/api/v1"
docs.SwaggerInfo.Schemes = []string{"http", "https"}
```

## Operation Annotations

Place directly above handler functions. Every handler that should appear in the docs needs at minimum `@Summary`, `@Router`, and response annotations.

```go
// @Summary      List all blobs on a node
// @Description  Returns all cached blobs with size, disk path, tier, and 5-minute access count
// @Tags         blobs
// @Accept       json
// @Produce      json
// @Param        nodeId  path  string  true  "Node ID"
// @Success      200  {object}  blobsapi.ListBlobsResponse
// @Failure      404  {object}  httptk.ErrorsList
// @Router       /nodes/{nodeId}/blobs [get]
func (c *blobsControllerV1) List(ctx *gin.Context) {
```

### Annotation Quick Reference

| Annotation       | Purpose                          | Example                                              |
|------------------|----------------------------------|------------------------------------------------------|
| `@Summary`       | Short one-line description       | `// @Summary List blobs`                             |
| `@Description`   | Detailed explanation             | `// @Description Returns all cached blobs`           |
| `@Tags`          | Group operations by category     | `// @Tags blobs`                                     |
| `@Accept`        | Request content types            | `// @Accept json`                                    |
| `@Produce`       | Response content types           | `// @Produce json`                                   |
| `@Param`         | Declare a parameter              | `// @Param id path string true "Node ID"`            |
| `@Success`       | Success response                 | `// @Success 200 {object} Model`                     |
| `@Failure`       | Error response                   | `// @Failure 404 {object} ErrorModel`                |
| `@Router`        | Path and HTTP method             | `// @Router /nodes/{id} [get]`                       |
| `@Security`      | Auth requirement                 | `// @Security ApiKeyAuth`                            |
| `@Deprecated`    | Mark endpoint deprecated         | `// @Deprecated`                                     |
| `@Header`        | Response header                  | `// @Header 200 {string} X-Request-Id "Request ID"`  |

## Parameters

Format: `@Param name location type required "description" annotations`

### Locations

| Location   | Usage                                   |
|------------|-----------------------------------------|
| `path`     | URL path segment: `/nodes/{nodeId}`     |
| `query`    | Query string: `?page=1&limit=10`        |
| `header`   | HTTP header                             |
| `body`     | Request body (one per operation)        |
| `formData` | Form field or file upload               |

### Examples

```go
// Path parameter
// @Param  nodeId  path  string  true  "Node identifier"

// Query parameters with defaults and constraints
// @Param  page   query  int     false  "Page number"     default(1)  minimum(1)
// @Param  limit  query  int     false  "Results per page" default(20) minimum(1) maximum(100)
// @Param  q      query  string  false  "Search query"

// Header parameter
// @Param  Authorization  header  string  true  "Bearer token"

// Body parameter (references a struct)
// @Param  request  body  CreateNodeRequest  true  "Node creation payload"

// File upload
// @Param  file  formData  file  true  "Upload file"

// Array query parameter
// @Param  tags  query  []string  false  "Filter by tags"  collectionFormat(csv)
```

## Responses

Format: `@Success/@Failure statusCode {type} Model "description"`

```go
// Single object
// @Success  200  {object}  blobsapi.BlobResponse

// Array
// @Success  200  {array}  blobsapi.BlobResponse

// No body (e.g. 204)
// @Success  204  "No Content"

// String response
// @Success  200  {string}  string  "OK"

// Error with OCI error format
// @Failure  400  {object}  httptk.ErrorsList
// @Failure  404  {object}  httptk.ErrorsList
// @Failure  500  {object}  httptk.ErrorsList
```

## Struct Tags for Models

Control how DTO fields appear in generated documentation:

```go
type BlobResponse struct {
    Digest        string `json:"digest"        example:"sha256:abc123..."  description:"Content-addressable digest"`
    Size          int64  `json:"size"          example:"1048576"           description:"Blob size in bytes"`
    MediaType     string `json:"mediaType"     example:"application/vnd.oci.image.layer.v1.tar+gzip"`
    DiskPath      string `json:"diskPath"      example:"/var/cache/barnacle/hot/sha256/abc123"`
    Tier          int    `json:"tier"          example:"0"                 description:"Cache tier number"`
    AccessCount5m int64  `json:"accessCount5m" example:"42"                description:"Access count in last 5 minutes"`
}
```

### Supported Struct Tags

| Tag              | Purpose                         | Example                              |
|------------------|---------------------------------|--------------------------------------|
| `json`           | JSON field name (required)      | `` `json:"digest"` ``                |
| `example`        | Example value for docs          | `` `example:"sha256:abc..."` ``      |
| `description`    | Field description               | `` `description:"Blob size"` ``      |
| `format`         | Data format hint                | `` `format:"date-time"` ``           |
| `default`        | Default value                   | `` `default:"active"` ``             |
| `enums`          | Comma-separated allowed values  | `` `enums:"active,inactive"` ``      |
| `required`       | Mark field as required          | `` `required:"true"` ``              |
| `swaggertype`    | Override Go type for swagger    | `` `swaggertype:"string"` ``         |
| `swaggerignore`  | Exclude field from docs         | `` `swaggerignore:"true"` ``         |
| `extensions`     | Vendor extensions               | `` `extensions:"x-nullable"` ``      |

### Type Overrides

When Go types don't map cleanly to Swagger types:

```go
type Event struct {
    // time.Time auto-maps, but you can be explicit:
    CreatedAt time.Time `json:"createdAt" swaggertype:"string" format:"date-time"`

    // Custom type -> primitive
    Status AppStatus `json:"status" swaggertype:"string" enums:"running,stopped"`

    // Map types
    Labels map[string]string `json:"labels" swaggertype:"object,string,string"`
}
```

### Model Naming

Rename a struct in the Swagger output:

```go
type InternalNodeResponse struct {
    NodeID string `json:"nodeId"`
} // @name NodeResponse
```

## Security Definitions

Define at the API level, apply per-operation:

```go
// In main file:
// @securityDefinitions.basic   BasicAuth
// @securityDefinitions.apikey  ApiKeyAuth
// @in                          header
// @name                        Authorization
// @description                 Bearer token (prefix with "Bearer ")

// On a handler:
// @Security  ApiKeyAuth
// @Security  BasicAuth
```

OAuth2 variants:

```go
// @securityDefinitions.oauth2.implicit      OAuth2Implicit
// @authorizationUrl                         https://example.com/oauth/authorize
// @scope.read                               Read access
// @scope.write                              Write access

// @securityDefinitions.oauth2.password      OAuth2Password
// @tokenUrl                                 https://example.com/oauth/token

// @securityDefinitions.oauth2.accessCode    OAuth2AccessCode
// @authorizationUrl                         https://example.com/oauth/authorize
// @tokenUrl                                 https://example.com/oauth/token
```

## swag init Flags

| Flag                          | Default      | Purpose                                         |
|-------------------------------|--------------|-------------------------------------------------|
| `-g, --generalInfo`           | `main.go`    | File containing general API info annotations    |
| `-d, --dir`                   | `./`         | Comma-separated directories to parse            |
| `-o, --output`                | `./docs`     | Output directory for generated files            |
| `--exclude`                   | —            | Directories to exclude from parsing             |
| `-p, --propertyStrategy`      | `camelcase`  | Field naming: `camelcase`, `snakecase`, `pascalcase` |
| `--outputTypes`               | `go,json,yaml` | File types to generate                       |
| `--parseDependency, --pd`     | `false`      | Parse external dependencies for models          |
| `--parseDependencyLevel, --pdl` | `0`        | Depth: 0=off, 1=models, 2=ops, 3=all           |
| `--parseInternal`             | `false`      | Parse `internal/` packages                      |
| `--parseDepth`                | `100`        | Maximum struct parse depth                      |
| `--requiredByDefault`         | `false`      | Treat all fields as required unless tagged       |
| `--instanceName`              | `swagger`    | Name for multiple Swagger instances             |
| `--overridesFile`             | `.swaggo`    | Global type override file                       |
| `--collectionFormat, --cf`    | `csv`        | Array param format: csv, multi, pipes, tsv, ssv |
| `-t, --tags`                  | —            | Only include operations with these tags         |

### Typical Command for This Project

```bash
swag init \
  -g cmd/barnacle/main.go \
  -d ./cmd/barnacle/,./internal/routes/,./pkg/api/,./internal/tk/httptk/ \
  --parseInternal \
  -o docs
```

This parses:
- Main entry point for general API info
- Route handlers for operation annotations
- DTO packages for model definitions
- Error types for response models

## gin-swagger Configuration

```go
router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler,
    ginSwagger.URL("/swagger/doc.json"),             // API definition URL
    ginSwagger.DefaultModelsExpandDepth(-1),          // -1 hides models section
    ginSwagger.DocExpansion("list"),                   // "list", "full", or "none"
    ginSwagger.DeepLinking(true),                      // Deep linking for tags/operations
    ginSwagger.PersistAuthorization(true),             // Remember auth across reloads
))
```

| Option                        | Type   | Default    | Purpose                                   |
|-------------------------------|--------|------------|-------------------------------------------|
| `URL`                         | string | `doc.json` | Path to the API definition JSON           |
| `DocExpansion`                | string | `list`     | Default expansion: `list`, `full`, `none` |
| `DeepLinking`                 | bool   | `true`     | Enable deep linking for tags/operations   |
| `DefaultModelsExpandDepth`    | int    | `1`        | Model section depth (-1 = hidden)         |
| `DefaultModelExpandDepth`     | int    | `1`        | Individual model depth                    |
| `DefaultModelRendering`       | string | `example`  | `example` or `model`                      |
| `PersistAuthorization`        | bool   | `false`    | Persist auth data across browser sessions |
| `InstanceName`                | string | `swagger`  | For multiple swagger instances            |

## Project Conventions

When adding swagger annotations to this codebase:

1. **DTOs live in `pkg/api/<group>/`** — these are the types referenced in `@Success` and `@Failure` annotations
2. **Error responses use `httptk.ErrorsList`** — the OCI-spec error format used project-wide
3. **Handlers live in `internal/routes/apiroutes/<group>/`** — annotate the exported handler methods
4. **Tags match route groups** — use `@Tags blobs`, `@Tags nodes`, `@Tags upstreams`
5. **Import the generated docs** with a blank import: `_ "github.com/pdylanross/barnacle/docs"`

## Detailed References

- See [swagger-annotation-patterns.md](swagger-annotation-patterns.md) for complete annotation examples matching this project's patterns
- See [swagger-model-documentation.md](swagger-model-documentation.md) for struct tag patterns, enums, composition, and generics

## External Resources

- swag: https://github.com/swaggo/swag
- gin-swagger: https://github.com/swaggo/gin-swagger
- Swagger 2.0 spec: https://swagger.io/specification/v2/
- OpenAPI migration guide: https://swagger.io/blog/news/whats-new-in-openapi-3-0/
