# Contributing to Barnacle

Thank you for your interest in contributing to Barnacle! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Submitting Changes](#submitting-changes)
- [Style Guidelines](#style-guidelines)
- [Reporting Issues](#reporting-issues)

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment. Be considerate of others and focus on constructive collaboration.

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/barnacle.git
   cd barnacle
   ```
3. Add the upstream repository as a remote:
   ```bash
   git remote add upstream https://github.com/pdylanross/barnacle.git
   ```

## Development Setup

### Prerequisites

- Go 1.25.3 or later
- Docker and Docker Compose
- Make

### Installing Development Tools

Install the required development tools:

```bash
make tools
```

This installs:
- `swag` - Swagger documentation generator
- `goimports` - Import formatting
- `golangci-lint` - Linter

### Local Development Environment

Start the local development dependencies (Redis):

```bash
make local-up
```

To stop the dependencies:

```bash
make local-down
```

### Building

```bash
make build
```

### Running Locally

```bash
make run serve
```

## Making Changes

1. Create a new branch for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. Make your changes, following the [style guidelines](#style-guidelines)

3. Run formatting:
   ```bash
   make fmt
   ```

4. Run the linter:
   ```bash
   make lint
   ```

5. Run tests:
   ```bash
   make test
   ```

6. Commit your changes with a descriptive commit message

## Testing

### Unit Tests

Run unit tests with race detection:

```bash
make test
```

### End-to-End Tests

For e2e tests, first start the local cluster:

```bash
make local-cluster-build
make local-cluster-up
```

Then run the e2e tests:

```bash
make e2e
```

Clean up after testing:

```bash
make local-cluster-down
```

### Writing Tests

- Place unit tests in `*_test.go` files alongside the code they test
- Place e2e tests in `test/e2e/`
- Use table-driven tests where appropriate
- Use `github.com/stretchr/testify` for assertions
- Use `github.com/alicebob/miniredis/v2` for Redis mocking in unit tests

## Submitting Changes

### Pull Request Process

1. Ensure all tests pass and the linter reports no issues
2. Update documentation if you're changing behavior
3. Push your branch to your fork:
   ```bash
   git push origin feature/your-feature-name
   ```
4. Open a pull request against the `main` branch
5. Fill out the pull request template with relevant information
6. Wait for review and address any feedback

### Commit Messages

Write clear, concise commit messages that explain the "why" behind changes:

- Use the imperative mood ("Add feature" not "Added feature")
- Keep the first line under 72 characters
- Reference issues when applicable (e.g., "Fix #123")

Good examples:
```
Add manifest cache TTL configuration

Allow users to configure how long manifests are cached before
revalidation. Defaults to 5 minutes for tags.

Fixes #42
```

```
Fix race condition in blob streaming

The blob reader could be closed while still in use during
concurrent requests. Add mutex to protect reader lifecycle.
```

## Style Guidelines

### Code Style

- Follow standard Go conventions and idioms
- Run `make fmt` before committing (uses `goimports` and `gofmt -s`)
- Run `make lint` to check for issues (uses `golangci-lint`)

### Documentation

- Add godoc comments to all exported functions, types, and packages
- Keep comments concise and focused on "why" rather than "what"
- Update README.md if adding user-facing features

### API Changes

- Follow OCI Distribution Specification for registry endpoints
- Use proper HTTP status codes and error responses
- Add Swagger annotations for new API endpoints:
  ```bash
  make swagger
  ```

## Reporting Issues

### Bug Reports

When reporting bugs, please include:

- Go version (`go version`)
- Operating system and architecture
- Steps to reproduce the issue
- Expected behavior
- Actual behavior
- Relevant logs or error messages

### Feature Requests

For feature requests, please describe:

- The problem you're trying to solve
- Your proposed solution
- Any alternatives you've considered

## Areas of Interest

We especially welcome contributions in these areas:

- Performance testing at scale
- Additional upstream registry integrations
- Metrics and Grafana dashboards
- Documentation improvements
- Bug reports and fixes

## Questions?

If you have questions, feel free to:

- Open an issue for discussion
- Check existing issues for similar topics

Thank you for contributing to Barnacle!
