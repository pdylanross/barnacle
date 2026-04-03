# Model Documentation Reference

Struct tag patterns, enums, composition, generics, and advanced model documentation for swag.

## Struct Tags Quick Reference

| Tag              | Purpose                                | Example                                      |
|------------------|----------------------------------------|----------------------------------------------|
| `json`           | JSON field name (required)             | `` `json:"digest"` ``                        |
| `example`        | Example value shown in docs            | `` `example:"sha256:abc123..."` ``           |
| `description`    | Field description                      | `` `description:"Blob size in bytes"` ``     |
| `format`         | Data format hint                       | `` `format:"date-time"` ``                   |
| `default`        | Default value                          | `` `default:"active"` ``                     |
| `enums`          | Comma-separated allowed values         | `` `enums:"active,inactive"` ``              |
| `required`       | Mark field as required                 | `` `required:"true"` ``                      |
| `minimum`        | Minimum numeric value                  | `` `minimum:"0"` ``                          |
| `maximum`        | Maximum numeric value                  | `` `maximum:"100"` ``                        |
| `minLength`      | Minimum string length                  | `` `minLength:"1"` ``                        |
| `maxLength`      | Maximum string length                  | `` `maxLength:"255"` ``                      |
| `pattern`        | Regex pattern for validation           | `` `pattern:"^[a-z]+$"` ``                   |
| `swaggertype`    | Override Go type for Swagger           | `` `swaggertype:"string"` ``                 |
| `swaggerignore`  | Exclude field from docs                | `` `swaggerignore:"true"` ``                 |
| `extensions`     | Vendor extensions (x-* prefixed)       | `` `extensions:"x-nullable,x-order=1"` ``   |

## DTO Examples (Project Conventions)

DTOs in this project live in `pkg/api/<group>/` and use JSON tags plus documentation tags:

```go
type BlobResponse struct {
    Digest        string `json:"digest"        example:"sha256:abc123..."  description:"Content-addressable digest"`
    Size          int64  `json:"size"          example:"1048576"           description:"Blob size in bytes"`
    MediaType     string `json:"mediaType"     example:"application/vnd.oci.image.layer.v1.tar+gzip"`
    DiskPath      string `json:"diskPath"      example:"/var/cache/barnacle/hot/sha256/abc123"`
    Tier          int    `json:"tier"          example:"0"                 description:"Cache tier number"`
    AccessCount5m int64  `json:"accessCount5m" example:"42"                description:"Access count in last 5 minutes"`
}

type ListBlobsResponse struct {
    Blobs []BlobResponse `json:"blobs"`
}
```

## Model-Level Documentation

Use `@Description` annotations on struct type comments:

```go
// UpstreamConfig represents an upstream container registry.
// @Description Upstream container registry configuration including
// @Description authentication, caching policy, and health check settings.
type UpstreamConfig struct {
    // Alias is the unique identifier for this upstream
    Alias    string `json:"alias"    example:"dockerhub" description:"Unique upstream alias"`
    // Host is the registry hostname
    Host     string `json:"host"     example:"registry-1.docker.io"`
    Insecure bool   `json:"insecure" default:"false"     description:"Allow HTTP connections"`
}
```

- `// @Description` lines on the struct become the schema description in Swagger
- Inline field comments (both `//` above and `//` trailing) appear as field descriptions
- The `description` struct tag takes precedence over inline comments

## Type Overrides with swaggertype

Use `swaggertype` when Go types don't map cleanly to Swagger primitives:

```go
type Event struct {
    // time.Time auto-maps, but you can be explicit:
    CreatedAt time.Time `json:"createdAt" swaggertype:"string" format:"date-time"`

    // Custom type mapped to a primitive
    Status AppStatus `json:"status" swaggertype:"string" enums:"running,stopped,error"`

    // sql.NullInt64 mapped to integer
    RetryCount sql.NullInt64 `json:"retryCount" swaggertype:"integer"`

    // big.Float mapped to number
    Score big.Float `json:"score" swaggertype:"number"`

    // Custom type mapped to primitive,subtype
    Timestamp TimestampTime `json:"timestamp" swaggertype:"primitive,integer"`

    // Slice of custom types
    Coefficients []big.Float `json:"coefficients" swaggertype:"array,number"`

    // Map types
    Labels map[string]string `json:"labels" swaggertype:"object,string,string"`
}
```

### swaggertype Formats

| Go Type                | swaggertype Value         | Swagger Result      |
|------------------------|---------------------------|---------------------|
| `sql.NullInt64`        | `"integer"`               | `integer`           |
| `sql.NullString`       | `"string"`                | `string`            |
| `big.Float`            | `"number"`                | `number`            |
| `[]big.Float`          | `"array,number"`          | `array` of `number` |
| Custom timestamp       | `"primitive,integer"`     | `integer`           |
| `map[string]string`    | `"object,string,string"`  | `object`            |

## Excluding Fields

Use `swaggerignore` to hide internal fields:

```go
type NodeInfo struct {
    ID       string `json:"id"`
    Hostname string `json:"hostname"`

    // Internal fields excluded from API docs
    InternalIP string `json:"-"           swaggerignore:"true"`
    DebugData  []byte `json:"debug,omitempty" swaggerignore:"true"`
}
```

Fields with `json:"-"` are already excluded from JSON marshaling, but `swaggerignore:"true"` explicitly excludes them from the Swagger schema too.

## Enums

### From Struct Tags

```go
type SearchRequest struct {
    Query string `json:"query"  example:"nginx"`
    // Sort order for results
    Order string `json:"order"  enums:"asc,desc"      default:"asc"`
    // Cache tier filter
    Tier  int    `json:"tier"   enums:"0,1,2"          description:"Cache tier number"`
}
```

### From Go Constants

Define enums as typed constants. Swag auto-discovers them:

```go
type CacheTier string

const (
    TierHot  CacheTier = "hot"    // Hot tier — fastest access
    TierWarm CacheTier = "warm"   // Warm tier — moderate access
    TierCold CacheTier = "cold"   // Cold tier — archival
)

type BlobFilter struct {
    Tier CacheTier `json:"tier"` // Swag picks up the enum values automatically
}
```

Override the display name of an enum variant with `@name`:

```go
const (
    StatusActive   Status = "active"
    StatusInactive Status = "inactive" // @name Disabled
)
```

## Field Validation Constraints

Combine multiple constraints on a single field:

```go
type PaginationParams struct {
    Page  int `json:"page"  minimum:"1" default:"1"                      description:"Page number"`
    Limit int `json:"limit" minimum:"1" maximum:"500" default:"50"       description:"Results per page"`
}

type CreateUpstreamRequest struct {
    Alias string `json:"alias" minLength:"1" maxLength:"63" pattern:"^[a-z0-9-]+$" description:"URL-safe alias"`
    Host  string `json:"host"  example:"registry-1.docker.io"                       description:"Registry hostname"`
}
```

## Array Fields

```go
type MultiTagFilter struct {
    // Comma-separated list of tags
    Tags []string `json:"tags" example:"latest,v1.0" collectionFormat:"csv"`

    // Array of integers
    TierIDs []int `json:"tierIds" example:"0,1"`
}
```

For array example values, use comma-separated strings in the `example` tag.

## Composition and Embedding

### Embedded Structs

Go struct embedding produces `allOf` in the Swagger output:

```go
type BaseResponse struct {
    RequestID string `json:"requestId" example:"req-abc123"`
    Timestamp string `json:"timestamp" format:"date-time"`
}

type BlobDetailResponse struct {
    BaseResponse               // Embedded — produces allOf in Swagger
    Digest   string `json:"digest"`
    Size     int64  `json:"size"`
}
```

### Inline Composition in Annotations

Override fields of a generic wrapper in `@Success` annotations:

```go
// Single object inside a wrapper
// @Success  200  {object}  jsonresult.JSONResult{data=blobsapi.BlobResponse}  "OK"

// Array inside a wrapper
// @Success  200  {object}  jsonresult.JSONResult{data=[]blobsapi.BlobResponse}  "OK"

// Primitive type inside a wrapper
// @Success  200  {object}  jsonresult.JSONResult{data=string}  "OK"

// Multiple field overrides
// @Success  200  {object}  jsonresult.JSONResult{data=blobsapi.BlobResponse,count=int}  "OK"

// Deep nesting
// @Success  200  {object}  jsonresult.JSONResult{data=blobsapi.BlobResponse{nested=blobsapi.Inner}}  "OK"
```

## Generics Support

Swag supports Go generics (1.18+) in annotations using bracket syntax:

```go
type PagedResponse[T any] struct {
    Items      []T `json:"items"`
    TotalCount int `json:"totalCount"`
    Page       int `json:"page"`
}

// Reference generic types in annotations with square brackets:
// @Success  200  {object}  PagedResponse[blobsapi.BlobResponse]
func (c *blobsControllerV1) List(ctx *gin.Context) {
```

Multiple type parameters:

```go
// @Success  200  {object}  web.Response[types.Post, types.Meta]
```

Nested generics:

```go
// @Success  200  {object}  web.Response[web.Inner[types.Post]]
```

## Model Naming

Override how a struct appears in the Swagger `definitions` section:

```go
type internalBlobDetailResponse struct {
    Digest    string `json:"digest"`
    Size      int64  `json:"size"`
    MediaType string `json:"mediaType"`
} // @name BlobDetail
```

This renames `internalBlobDetailResponse` to `BlobDetail` in the generated spec. Useful when internal Go names don't match the desired public API naming.

## Function-Scoped Structs

Define request/response types inside handler functions. Reference them as `<package>.<function>.<struct>`:

```go
// @Param    request  body      blobsapi.Search.searchRequest  true  "Search parameters"
// @Success  200      {object}  blobsapi.Search.searchResponse
// @Router   /nodes/{nodeId}/blobs/search [post]
func (c *blobsControllerV1) Search(ctx *gin.Context) {
    type searchRequest struct {
        Query     string `json:"query"`
        MediaType string `json:"mediaType"`
    }
    type searchResponse struct {
        Results []BlobResponse `json:"results"`
        Total   int            `json:"total"`
    }
    // ...
}
```

## Global Type Overrides (.swaggo file)

Create a `.swaggo` file in the project root to replace or skip types globally:

```
// Replace sql.NullInt64 with int everywhere
replace database/sql.NullInt64 int

// Replace sql.NullString with string everywhere
replace database/sql.NullString string

// Skip a type entirely (excluded from docs)
skip internal/secret.Credentials
```

Use with: `swag init --overridesFile .swaggo`

## Vendor Extensions

Add custom `x-*` extensions to fields:

```go
type Account struct {
    ID     string `json:"id"     extensions:"x-nullable"`
    Name   string `json:"name"   extensions:"x-order=1"`
    Email  string `json:"email"  extensions:"x-nullable,x-order=2,!x-omitempty"`
}
```

- `x-key` — sets extension to `true`
- `x-key=value` — sets extension to the given value
- `!x-key` — sets extension to `false`

## Project Conventions for Models

1. **DTOs in `pkg/api/<group>/`** — public types referenced in annotations
2. **Always include `json` tags** — required for field name mapping
3. **Use `example` tags** on key fields — improves Swagger UI try-it-out experience
4. **Use `description` tags** for non-obvious fields
5. **Error responses use `httptk.ErrorsList`** — the project-wide OCI error format
6. **Align struct tags** for readability when practical
