# Gin Middleware Patterns Reference

Complete guide to middleware patterns in Gin.

## Middleware Basics

Middleware in Gin are functions that have access to the request/response cycle. They can:
- Execute code before/after the handler
- Modify the request or response
- End the request-response cycle
- Call the next middleware in the chain

### Basic Structure

```go
func MyMiddleware() gin.HandlerFunc {
    // Setup code (runs once at startup)
    return func(c *gin.Context) {
        // Before request
        
        c.Next()  // Call next handler
        
        // After request
    }
}
```

## Applying Middleware

### Global Middleware
```go
r := gin.New()
r.Use(gin.Logger())
r.Use(gin.Recovery())
r.Use(MyMiddleware())
```

### Route-Specific Middleware
```go
r.GET("/admin", AuthMiddleware(), adminHandler)
r.POST("/upload", RateLimitMiddleware(), uploadHandler)
```

### Group Middleware
```go
api := r.Group("/api")
api.Use(AuthMiddleware())
{
    api.GET("/users", listUsers)
    api.POST("/users", createUser)
}

// Nested groups
admin := api.Group("/admin")
admin.Use(AdminOnlyMiddleware())
{
    admin.DELETE("/users/:id", deleteUser)
}
```

## Common Middleware Patterns

### Authentication Middleware

```go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        if token == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "Authorization header required",
            })
            return
        }
        
        // Validate token
        user, err := validateToken(token)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "Invalid token",
            })
            return
        }
        
        // Store user in context for handlers
        c.Set("user", user)
        c.Next()
    }
}

// In handler:
func profileHandler(c *gin.Context) {
    user := c.MustGet("user").(*User)
    c.JSON(http.StatusOK, user)
}
```

### Request Logging Middleware

```go
func RequestLogger() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        query := c.Request.URL.RawQuery
        
        c.Next()
        
        latency := time.Since(start)
        status := c.Writer.Status()
        clientIP := c.ClientIP()
        method := c.Request.Method
        
        log.Printf("[%d] %s %s %s %v %s",
            status, method, path, query, latency, clientIP)
        
        // Log errors if any
        if len(c.Errors) > 0 {
            log.Printf("Errors: %v", c.Errors.String())
        }
    }
}
```

### Rate Limiting Middleware

```go
func RateLimiter(maxRequests int, window time.Duration) gin.HandlerFunc {
    // Simple in-memory rate limiter (use Redis for production)
    requests := make(map[string][]time.Time)
    var mu sync.Mutex
    
    return func(c *gin.Context) {
        clientIP := c.ClientIP()
        now := time.Now()
        
        mu.Lock()
        
        // Clean old requests
        var valid []time.Time
        for _, t := range requests[clientIP] {
            if now.Sub(t) < window {
                valid = append(valid, t)
            }
        }
        requests[clientIP] = valid
        
        if len(requests[clientIP]) >= maxRequests {
            mu.Unlock()
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
                "error": "Rate limit exceeded",
            })
            return
        }
        
        requests[clientIP] = append(requests[clientIP], now)
        mu.Unlock()
        
        c.Next()
    }
}
```

### CORS Middleware

```go
func CORSMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "*")
        c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
        c.Header("Access-Control-Max-Age", "86400")
        
        if c.Request.Method == "OPTIONS" {
            c.AbortWithStatus(http.StatusNoContent)
            return
        }
        
        c.Next()
    }
}
```

### Request ID Middleware

```go
func RequestID() gin.HandlerFunc {
    return func(c *gin.Context) {
        requestID := c.GetHeader("X-Request-ID")
        if requestID == "" {
            requestID = uuid.New().String()
        }
        
        c.Set("request_id", requestID)
        c.Header("X-Request-ID", requestID)
        
        c.Next()
    }
}
```

### Timeout Middleware

```go
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
        defer cancel()
        
        c.Request = c.Request.WithContext(ctx)
        
        finished := make(chan struct{})
        go func() {
            c.Next()
            close(finished)
        }()
        
        select {
        case <-finished:
            // Request completed normally
        case <-ctx.Done():
            c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
                "error": "Request timeout",
            })
        }
    }
}
```

## Error Handling Middleware

### Centralized Error Handler

```go
// Custom error type
type AppError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Details any    `json:"details,omitempty"`
}

func (e *AppError) Error() string {
    return e.Message
}

func NewAppError(code int, message string) *AppError {
    return &AppError{Code: code, Message: message}
}

// Error handling middleware
func ErrorHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()
        
        if len(c.Errors) == 0 {
            return
        }
        
        err := c.Errors.Last().Err
        
        // Check for custom error type
        var appErr *AppError
        if errors.As(err, &appErr) {
            c.JSON(appErr.Code, appErr)
            return
        }
        
        // Default to 500
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Internal server error",
        })
    }
}

// Usage in handler
func handler(c *gin.Context) {
    user, err := findUser(id)
    if err != nil {
        c.Error(NewAppError(http.StatusNotFound, "User not found"))
        return
    }
    c.JSON(http.StatusOK, user)
}
```

### Recovery with Custom Handler

```go
func CustomRecovery() gin.HandlerFunc {
    return gin.CustomRecoveryWithWriter(gin.DefaultErrorWriter, func(c *gin.Context, err any) {
        // Log the panic
        log.Printf("Panic recovered: %v\n%s", err, debug.Stack())
        
        // Notify monitoring service
        notifyError(err)
        
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
            "error": "An unexpected error occurred",
        })
    })
}
```

## Middleware Flow Control

### c.Next() vs c.Abort()

```go
func Middleware1() gin.HandlerFunc {
    return func(c *gin.Context) {
        log.Println("M1: before")
        c.Next()                      // Continue to next handler
        log.Println("M1: after")
    }
}

func Middleware2() gin.HandlerFunc {
    return func(c *gin.Context) {
        log.Println("M2: before")
        c.Abort()                     // Stop chain, remaining handlers skipped
        log.Println("M2: after")      // This still runs
    }
}

// Output: M1: before -> M2: before -> M2: after -> M1: after
```

### Conditional Middleware

```go
func ConditionalAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Skip auth for certain paths
        if c.Request.URL.Path == "/health" {
            c.Next()
            return
        }
        
        // Apply auth for everything else
        token := c.GetHeader("Authorization")
        if token == "" {
            c.AbortWithStatus(http.StatusUnauthorized)
            return
        }
        
        c.Next()
    }
}
```

## Context Value Passing

### Setting and Getting Values

```go
func ContextMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Set values
        c.Set("user_id", "12345")
        c.Set("request_time", time.Now())
        
        c.Next()
    }
}

func handler(c *gin.Context) {
    // Get with type assertion
    userID := c.GetString("user_id")
    
    // Get with default
    count := c.GetInt("count")  // Returns 0 if not set
    
    // MustGet panics if key doesn't exist
    requestTime := c.MustGet("request_time").(time.Time)
    
    // Check if exists
    if val, exists := c.Get("optional_key"); exists {
        // Use val
    }
}
```

## Popular gin-contrib Middleware

Commonly used middleware from the official collection:

```go
import (
    "github.com/gin-contrib/cors"
    "github.com/gin-contrib/gzip"
    "github.com/gin-contrib/sessions"
    "github.com/gin-contrib/timeout"
    "github.com/gin-contrib/cache"
)

// CORS
r.Use(cors.Default())
r.Use(cors.New(cors.Config{
    AllowOrigins:     []string{"https://example.com"},
    AllowMethods:     []string{"GET", "POST"},
    AllowHeaders:     []string{"Origin", "Content-Type"},
    ExposeHeaders:    []string{"Content-Length"},
    AllowCredentials: true,
    MaxAge:           12 * time.Hour,
}))

// Gzip compression
r.Use(gzip.Gzip(gzip.DefaultCompression))

// Sessions
store := cookie.NewStore([]byte("secret"))
r.Use(sessions.Sessions("mysession", store))

// Response caching
r.Use(cache.CachePage(store, time.Minute, func(c *gin.Context) {
    // Generate page
}))
```

## Middleware Testing

```go
func TestAuthMiddleware(t *testing.T) {
    gin.SetMode(gin.TestMode)
    
    r := gin.New()
    r.Use(AuthMiddleware())
    r.GET("/protected", func(c *gin.Context) {
        c.String(http.StatusOK, "success")
    })
    
    // Test without token
    w := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/protected", nil)
    r.ServeHTTP(w, req)
    assert.Equal(t, http.StatusUnauthorized, w.Code)
    
    // Test with valid token
    w = httptest.NewRecorder()
    req, _ = http.NewRequest("GET", "/protected", nil)
    req.Header.Set("Authorization", "valid-token")
    r.ServeHTTP(w, req)
    assert.Equal(t, http.StatusOK, w.Code)
}
```

## Best Practices

1. **Order matters**: Middleware executes in order of registration
2. **Always call `c.Next()` or `c.Abort()`**: Forgetting can cause unexpected behavior
3. **Copy context for goroutines**: Use `c.Copy()` when passing context to goroutines
4. **Keep middleware focused**: Each middleware should do one thing well
5. **Use `ShouldBind*` over `Bind*`**: Better error handling control
6. **Set appropriate timeouts**: Prevent hanging requests
7. **Log carefully**: Don't log sensitive data (passwords, tokens)
8. **Test middleware in isolation**: Use httptest for unit testing