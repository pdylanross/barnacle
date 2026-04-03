# Barnacle - Claude Code Instructions

## Skills

- **Always** load the `code-standards` skill when working with code in this repo. It covers context usage, dependency injection, development workflow, environment tools, error handling, API swagger support, internal API standards, and unit testing patterns.
- Load the `gin-gonic` skill when working with HTTP handlers, routes, middleware, or Gin-related components.
- Load the `oci-distribution-spec` skill when working with registry protocol, OCI endpoints, or container distribution code.

## Build Commands
- `make` - Format and build project
- `make fmt` - Format code using goimports
- `make test` - Run all unit tests (requires local Redis)
- `make e2e` - Run end-to-end tests
- `make lint` - Run golangci-lint
- `make run <args>` - Run the barnacle CLI with arguments (e.g., `make run serve` or `make run -- serve --configDir /tmp/test`)
- `make local-up` - Start local development dependencies (Redis)
- `make local-down` - Stop local development dependencies

## Running the Application
- **ALWAYS prefer `make run` when testing code changes**
- Use `make run serve` to start the server with default config
- Use `make run -- <command> <flags>` when passing flags (the `--` separator is required before flags)
- Examples:
    - `make run serve` - Run with default configuration
    - `make run -- serve --configDir /custom/path` - Run with custom config directory

## Test Commands
- **ALWAYS use `make test` to run unit tests** - This is the standard way to run tests in this project
- **Local Redis must be running for unit tests** - Start with `make local-up`, stop with `make local-down`
- `go test -v -run=TestName ./...` - Run a specific test by name (only when targeting specific tests)
- `make e2e` - Run end-to-end tests

## Project Structure

### Core Application
- `cmd/barnacle` - CLI entrypoint and commands (serve, root)
- `cmd/e2e-imagegen` - E2E test image generator tool
- `pkg/configuration` - Public configuration structs (importable by external packages)
- `pkg/api` - Public API DTOs (blobsapi, nodesapi, rebalanceapi, upstreamsapi)
- `internal/configloader` - Config loading logic (YAML + envsubst + koanf with BARNACLE_ env prefix)
- `internal/dependencies` - Dependency injection container with eager initialization
- `internal/logsetup` - Logger initialization and configuration

### Server & Routing
- `internal/server` - Server lifecycle management and initialization
- `internal/server/httpserver` - HTTP server implementation with graceful shutdown
- `internal/routes` - Route registration and health endpoints
- `internal/routes/apiroutes` - API route group (management endpoints)
- `internal/routes/apiroutes/upstreamsapi` - Upstream registry management API (v1)
- `internal/routes/apiroutes/nodesapi` - Node lifecycle and health API (v1)
- `internal/routes/apiroutes/blobsapi` - Blob cache inspection API (v1)
- `internal/routes/apiroutes/rebalanceapi` - Rebalance operations API (v1)
- `internal/routes/distributionroutes` - OCI Distribution API routes
- `internal/routes/distributionroutes/registry` - Registry controller for OCI endpoints

### Node Management
- `internal/node` - Node management (disk usage, registry coordination)

### Registry & Caching
- `internal/registry` - Upstream registry management and factory
- `internal/registry/upstream` - Upstream interface and caching wrapper
- `internal/registry/upstream/standard` - Standard upstream implementation (go-containerregistry)
- `internal/registry/data` - Data types for manifests and responses
- `internal/registry/cache` - Cache interfaces (BlobCache, ManifestCache)
- `internal/registry/cache/memory` - In-memory manifest cache with TTL
- `internal/registry/cache/disk` - Disk-based blob cache with descriptor persistence
- `internal/registry/cache/coordinator` - Redis-coordinated distributed blob cache
- `internal/registry/cache/coordinator/rebalance` - Blob rebalance planner, transfer, and worker

### Infrastructure
- `internal/tasks` - Task runner system for managing long-running concurrent tasks
- `internal/tk` - Toolkit utilities (defer helpers, development utilities)
- `internal/tk/httptk` - HTTP toolkit (OCI-compliant error handling, responses)

### Testing
- `test/` - Test utilities shared across all tests
- `test/mocks` - Shared mock implementations (BlobCache, NodeRegistry, Task)
- `test/e2e` - End-to-end tests
- `hack/local` - Local development configuration (docker-compose for Redis)
- `hack/local-clustered` - Multi-node clustered local dev environment
- `hack/e2e` - E2E test infrastructure (minikube setup, manifests, config)

## Task System
- `internal/tasks` provides a concurrent task runner with graceful shutdown
- Tasks start immediately when added via `AddTask()` and receive a named logger
- Use `Wait()` to block until all tasks complete or a signal is received
- `OneShot` task wrapper executes a function once with context cancellation support
- `Repeating` task wrapper executes functions at fixed intervals

## Toolkit (tk)
- `internal/tk` provides common utility functions
- `HandleDeferError(fn, logger, description)` - handles defer statements that return errors
    - Logs errors with context if the deferred function fails
    - Example: `defer tk.HandleDeferError(file.Close, logger, "closing file")`
- `IgnoreDeferError(fn)` - silently ignores errors from deferred functions
    - Example: `defer tk.IgnoreDeferError(logger.Sync)`
- `internal/tk/httptk` provides HTTP utilities for OCI-compliant error handling
