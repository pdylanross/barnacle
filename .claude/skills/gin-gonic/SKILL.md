---
name: gin-gonic
description: Comprehensive guide for building web APIs and applications with Gin, a high-performance HTTP web framework for Go. Use when creating REST APIs, implementing middleware, handling request binding/validation, rendering responses (JSON/XML/HTML), managing routes, file uploads, authentication, graceful shutdown, or any Go web development task using Gin.
---

# Gin Web Framework Reference

Gin is a high-performance HTTP web framework for Go, providing a Martini-like API with up to 40x better performance thanks to httprouter.

## Quick Start

```go
package main

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

func main() {
    r := gin.Default() // Includes Logger and Recovery middleware
    
    r.GET("/ping", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"message": "pong"})
    })
    
    r.Run() // Default :8080, or r.Run(":3000")
}
```

## Core Concepts

### Engine Initialization
```go
r := gin.Default()  // With Logger + Recovery middleware
r := gin.New()      // Blank, no middleware
```

### HTTP Methods
```go
r.GET("/path", handler)
r.POST("/path", handler)
r.PUT("/path", handler)
r.DELETE("/path", handler)
r.PATCH("/path", handler)
r.HEAD("/path", handler)
r.OPTIONS("/path", handler)
r.Any("/path", handler)  // All methods
```

### The Context (`*gin.Context`)
The most important type in Gin - carries request details, validates/serializes data, manages middleware flow.

## Routing

### Path Parameters
```go
// Matches /user/john but not /user/ or /user
r.GET("/user/:name", func(c *gin.Context) {
    name := c.Param("name")
    c.String(http.StatusOK, "Hello %s", name)
})

// Wildcard: matches /user/john/send, /user/john/profile, etc.
r.GET("/user/:name/*action", func(c *gin.Context) {
    name := c.Param("name")
    action := c.Param("action")
})
```

### Query Parameters
```go
// /welcome?firstname=Jane&lastname=Doe
r.GET("/welcome", func(c *gin.Context) {
    firstname := c.DefaultQuery("firstname", "Guest")
    lastname := c.Query("lastname")
})
```

### Route Groups
```go
v1 := r.Group("/v1")
{
    v1.POST("/login", loginHandler)
    v1.POST("/submit", submitHandler)
}

v2 := r.Group("/v2")
v2.Use(AuthMiddleware()) // Group-specific middleware
{
    v2.POST("/login", loginHandlerV2)
}
```

## Request Binding & Validation

Gin uses `go-playground/validator/v10` for validation.

### Binding Methods

| Type | Must Bind (aborts on error) | Should Bind (returns error) |
|------|----------------------------|----------------------------|
| Auto | `Bind` | `ShouldBind` |
| JSON | `BindJSON` | `ShouldBindJSON` |
| XML | `BindXML` | `ShouldBindXML` |
| Query | `BindQuery` | `ShouldBindQuery` |
| URI | `BindUri` | `ShouldBindUri` |
| Header | `BindHeader` | `ShouldBindHeader` |
| YAML | `BindYAML` | `ShouldBindYAML` |

**Prefer `ShouldBind*` methods** - they return errors for you to handle.

### Example Struct with Validation
```go
type Login struct {
    User     string `form:"user" json:"user" binding:"required"`
    Password string `form:"password" json:"password" binding:"required,min=6"`
    Email    string `form:"email" json:"email" binding:"required,email"`
}

r.POST("/login", func(c *gin.Context) {
    var login Login
    if err := c.ShouldBindJSON(&login); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    // Process login...
    c.JSON(http.StatusOK, gin.H{"status": "logged in"})
})
```

### URI Binding
```go
type Person struct {
    ID   string `uri:"id" binding:"required,uuid"`
    Name string `uri:"name" binding:"required"`
}

r.GET("/:name/:id", func(c *gin.Context) {
    var person Person
    if err := c.ShouldBindUri(&person); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, person)
})
```

### Default Values
```go
type Request struct {
    Name string `form:"name,default=Guest"`
    Page int    `form:"page,default=1"`
}
```

## Response Rendering

### JSON
```go
c.JSON(http.StatusOK, gin.H{"message": "hey", "status": http.StatusOK})
c.JSON(http.StatusOK, myStruct)
c.IndentedJSON(http.StatusOK, data)  // Pretty-printed
c.SecureJSON(http.StatusOK, data)    // Prevents hijacking (prepends while(1);)
c.PureJSON(http.StatusOK, data)      // Literal chars (no unicode escape)
c.AsciiJSON(http.StatusOK, data)     // ASCII only, escapes non-ASCII
```

### Other Formats
```go
c.XML(http.StatusOK, data)
c.YAML(http.StatusOK, data)
c.TOML(http.StatusOK, data)
c.ProtoBuf(http.StatusOK, protoData)
c.String(http.StatusOK, "Hello %s", name)
c.Data(http.StatusOK, "text/plain", []byte("raw data"))
```

### HTML Templates
```go
r.LoadHTMLGlob("templates/*")
// or r.LoadHTMLFiles("templates/index.html", "templates/about.html")

r.GET("/", func(c *gin.Context) {
    c.HTML(http.StatusOK, "index.tmpl", gin.H{"title": "Main"})
})
```

### File Responses
```go
c.File("local/file.txt")
c.FileAttachment("local/file.txt", "download-name.txt")
c.DataFromReader(http.StatusOK, contentLength, contentType, reader, extraHeaders)
```

## Middleware

### Using Middleware
```go
r := gin.New()
r.Use(gin.Logger())      // Global
r.Use(gin.Recovery())    // Global

r.GET("/benchmark", MyMiddleware(), handler)  // Per-route

authorized := r.Group("/auth")
authorized.Use(AuthRequired())  // Per-group
```

### Writing Custom Middleware
```go
func Logger() gin.HandlerFunc {
    return func(c *gin.Context) {
        t := time.Now()
        
        c.Set("example", "12345")  // Set values for handlers
        
        c.Next()  // Process request
        
        latency := time.Since(t)
        status := c.Writer.Status()
        log.Printf("Status: %d, Latency: %v", status, latency)
    }
}
```

### Flow Control
```go
c.Next()                           // Continue to next handler
c.Abort()                          // Stop chain, don't write response
c.AbortWithStatus(http.StatusUnauthorized)
c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
```

### Error Handling Middleware
```go
func ErrorHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()
        
        if len(c.Errors) > 0 {
            err := c.Errors.Last().Err
            c.JSON(http.StatusInternalServerError, gin.H{
                "success": false,
                "error":   err.Error(),
            })
        }
    }
}

// In handlers, use c.Error(err) to add errors
```

### Recovery with Custom Handler
```go
r.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
    if err, ok := recovered.(string); ok {
        c.String(http.StatusInternalServerError, "Error: %s", err)
    }
    c.AbortWithStatus(http.StatusInternalServerError)
}))
```

## File Uploads

### Single File
```go
r.MaxMultipartMemory = 8 << 20  // 8 MiB (default 32 MiB)

r.POST("/upload", func(c *gin.Context) {
    file, _ := c.FormFile("file")
    // WARNING: file.Filename should NOT be trusted
    c.SaveUploadedFile(file, "/safe/path/"+filepath.Base(file.Filename))
    c.String(http.StatusOK, "Uploaded: %s", file.Filename)
})
```

### Multiple Files
```go
r.POST("/upload", func(c *gin.Context) {
    form, _ := c.MultipartForm()
    files := form.File["upload[]"]
    
    for _, file := range files {
        c.SaveUploadedFile(file, dst)
    }
    c.String(http.StatusOK, "%d files uploaded", len(files))
})
```

## Static Files
```go
r.Static("/assets", "./assets")
r.StaticFS("/static", http.Dir("my_file_system"))
r.StaticFile("/favicon.ico", "./resources/favicon.ico")
```

## Authentication

### BasicAuth
```go
authorized := r.Group("/admin", gin.BasicAuth(gin.Accounts{
    "admin": "secret",
    "user":  "password",
}))

authorized.GET("/secrets", func(c *gin.Context) {
    user := c.MustGet(gin.AuthUserKey).(string)
    c.JSON(http.StatusOK, gin.H{"user": user})
})
```

## Cookies
```go
// Set
c.SetCookie("name", "value", 3600, "/", "localhost", false, true)

// Get
cookie, err := c.Cookie("name")
```

## Redirects
```go
c.Redirect(http.StatusMovedPermanently, "http://example.com/")
c.Redirect(http.StatusFound, "/new-path")  // For POST redirects

// Internal redirect (same router)
c.Request.URL.Path = "/new-path"
r.HandleContext(c)
```

## Graceful Shutdown
```go
srv := &http.Server{Addr: ":8080", Handler: router}

go func() {
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("listen: %s\n", err)
    }
}()

quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := srv.Shutdown(ctx); err != nil {
    log.Fatal("Server forced to shutdown:", err)
}
```

## Production Configuration

### Release Mode
```go
gin.SetMode(gin.ReleaseMode)
// Or: export GIN_MODE=release
```

### Trusted Proxies
```go
// Trust specific proxies
r.SetTrustedProxies([]string{"192.168.1.2"})

// Disable proxy trust (use direct client IP)
r.SetTrustedProxies(nil)

// CDN platforms
r.TrustedPlatform = gin.PlatformCloudflare
r.TrustedPlatform = gin.PlatformGoogleAppEngine
```

### Custom HTTP Server
```go
s := &http.Server{
    Addr:           ":8080",
    Handler:        router,
    ReadTimeout:    10 * time.Second,
    WriteTimeout:   10 * time.Second,
    MaxHeaderBytes: 1 << 20,
}
s.ListenAndServe()
```

## Testing
```go
func setupRouter() *gin.Engine {
    r := gin.Default()
    r.GET("/ping", func(c *gin.Context) {
        c.String(http.StatusOK, "pong")
    })
    return r
}

func TestPingRoute(t *testing.T) {
    router := setupRouter()
    
    w := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/ping", nil)
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusOK, w.Code)
    assert.Equal(t, "pong", w.Body.String())
}
```

## Goroutines in Handlers

**Always copy context when using goroutines:**
```go
r.GET("/async", func(c *gin.Context) {
    cCp := c.Copy()  // IMPORTANT: Copy context
    go func() {
        time.Sleep(5 * time.Second)
        log.Println("Done: " + cCp.Request.URL.Path)
    }()
})
```

## Build Tags

```bash
# Alternative JSON libraries
go build -tags=jsoniter .    # json-iterator
go build -tags=go_json .     # go-json  
go build -tags=sonic .       # sonic (fastest)

# Disable MsgPack
go build -tags=nomsgpack .
```

## Detailed References

For comprehensive examples and patterns:
- See [references/binding-patterns.md](references/binding-patterns.md) for advanced binding
- See [references/middleware-patterns.md](references/middleware-patterns.md) for middleware patterns

## Official Resources

- Documentation: https://gin-gonic.com/en/docs/
- GitHub: https://github.com/gin-gonic/gin
- Examples: https://github.com/gin-gonic/examples
- Middleware collection: https://github.com/gin-contrib