# Dependency Injection

## Overview

This project uses a simple, explicit dependency injection pattern through the `internal/dependencies` package. The pattern emphasizes:
- **Eager initialization** - All dependencies are created at startup, not lazily
- **Explicit wiring** - No reflection or magic; dependencies are manually constructed
- **Single container** - One `Dependencies` struct holds all application dependencies
- **Getter methods** - Dependencies are accessed via typed getter methods

## The Dependencies Container

The `Dependencies` struct in `internal/dependencies/dependencies.go` is the central DI container:

```go
type Dependencies struct {
    config           *configuration.Configuration
    logger           *zap.Logger
    taskRunner       *tasks.Runner
    upstreamRegistry *registry.UpstreamRegistry
    redisClient      *redis.Client
}
```

### Creating the Container

Use `NewDependencies()` to create and initialize all dependencies:

```go
deps, err := dependencies.NewDependencies(ctx, config, logger)
if err != nil {
    return err
}
defer deps.Close()
```

**Important**: The constructor:
- Takes a context for initialization operations (e.g., Redis ping)
- Returns an error if any dependency fails to initialize
- Constructs ALL dependencies eagerly during this call
- Cleans up partially-constructed dependencies on error

### Accessing Dependencies

Use typed getter methods to access dependencies:

```go
deps.Config()           // *configuration.Configuration
deps.Logger()           // *zap.Logger
deps.TaskRunner()       // *tasks.Runner
deps.UpstreamRegistry() // *registry.UpstreamRegistry
deps.RedisClient()      // *redis.Client
```

### Releasing Resources

Always call `Close()` when done to release resources:

```go
defer deps.Close()
```

## Adding New Dependencies

When adding a new dependency to the container:

1. **Add the field** to the `Dependencies` struct (unexported):
   ```go
   type Dependencies struct {
       // ... existing fields
       myNewDep *mypackage.MyDependency
   }
   ```

2. **Initialize in constructor** with proper error handling and cleanup:
   ```go
   func NewDependencies(...) (*Dependencies, error) {
       // ... existing initialization

       myNewDep, err := mypackage.New(config)
       if err != nil {
           // Clean up already-initialized dependencies
           _ = redisClient.Close()
           return nil, err
       }

       return &Dependencies{
           // ... existing fields
           myNewDep: myNewDep,
       }, nil
   }
   ```

3. **Add getter method**:
   ```go
   func (d *Dependencies) MyNewDep() *mypackage.MyDependency {
       return d.myNewDep
   }
   ```

4. **Update Close()** if the dependency needs cleanup:
   ```go
   func (d *Dependencies) Close() error {
       var errs []error
       if d.redisClient != nil {
           if err := d.redisClient.Close(); err != nil {
               errs = append(errs, err)
           }
       }
       if d.myNewDep != nil {
           if err := d.myNewDep.Close(); err != nil {
               errs = append(errs, err)
           }
       }
       return errors.Join(errs...)
   }
   ```

## Passing Dependencies to Components

### Route Handlers

Pass the entire `Dependencies` container to route registration:

```go
func RegisterRoutes(router *gin.Engine, deps *dependencies.Dependencies) {
    api := router.Group("/api")
    upstreamsapi.RegisterV1(api, deps)
}
```

Controllers extract what they need:

```go
func newController(deps *dependencies.Dependencies) *controller {
    return &controller{
        logger:   deps.Logger().Named("mycontroller"),
        registry: deps.UpstreamRegistry(),
    }
}
```

### Tasks

Tasks receive dependencies through the container:

```go
deps.TaskRunner().AddTask("my-task", NewMyTask(deps))
```

## Design Principles

### Why Eager Initialization?
- **Fail fast** - Configuration errors are caught at startup, not runtime
- **Predictable startup** - All dependencies are ready before serving requests
- **Simpler debugging** - No lazy initialization race conditions

### Why No Interfaces for the Container?
- The container is an implementation detail, not an abstraction
- Components depend on specific types, not interfaces
- Testing uses the real container with test configuration

### Why Getter Methods Instead of Public Fields?
- Encapsulation - internal representation can change
- Consistency - all access goes through methods
- Future flexibility - can add lazy initialization or caching if needed

## Testing with Dependencies

Tests create real `Dependencies` instances with test configuration:

```go
func TestMyComponent(t *testing.T) {
    logger := testutils.CreateTestLogger(t)
    config := configuration.Default()
    config.Cache.Disk.BasePath = t.TempDir()

    deps, err := dependencies.NewDependencies(context.Background(), config, logger)
    require.NoError(t, err)
    defer deps.Close()

    // Use deps in your test
}
```

For unit tests that don't need the full container, mock individual dependencies using the mocks in `test/mocks/`.

## Anti-Patterns to Avoid

### Don't Store Dependencies as Package-Level Variables
```go
// BAD
var globalDeps *dependencies.Dependencies

// GOOD - pass explicitly
func NewHandler(deps *dependencies.Dependencies) *Handler
```

### Don't Create Multiple Containers
```go
// BAD
deps1, _ := dependencies.NewDependencies(ctx, config, logger)
deps2, _ := dependencies.NewDependencies(ctx, config, logger)

// GOOD - create once, pass everywhere
deps, _ := dependencies.NewDependencies(ctx, config, logger)
```

### Don't Access Dependencies Before Checking Error
```go
// BAD
deps, err := dependencies.NewDependencies(ctx, config, logger)
deps.Logger().Info("starting") // deps might be nil!

// GOOD
deps, err := dependencies.NewDependencies(ctx, config, logger)
if err != nil {
    return err
}
deps.Logger().Info("starting")
```
