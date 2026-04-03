---
name: code-standards
description: Project code standards covering context usage, DI, workflow, errors, testing, API design, and Swagger docs. Always load when working with code in this repo.
---

# Barnacle Code Standards

This project follows consistent patterns for Go development. Load the relevant reference documents below based on the area you're working in.

## References

| Reference | Load when... |
|---|---|
| [context-standards](references/context-standards.md) | Working with `context.Context` parameters or propagation |
| [dependency-injection](references/dependency-injection.md) | Adding, modifying, or wiring dependencies via `internal/dependencies` |
| [development-workflow](references/development-workflow.md) | Creating branches, commits, or PRs |
| [environment-dependencies](references/environment-dependencies.md) | Adding dev tools or updating `make tools` |
| [error-handling](references/error-handling.md) | Handling errors, defining sentinel errors, or returning HTTP errors |
| [api-swagger-support](references/api-swagger-support.md) | Adding or updating Swagger/OpenAPI annotations |
| [swagger-annotation-patterns](references/swagger-annotation-patterns.md) | Writing handler-level swagger comment blocks |
| [swagger-model-documentation](references/swagger-model-documentation.md) | Documenting DTOs with struct tags, enums, or generics for swagger |
| [internal-api-standards](references/internal-api-standards.md) | Building internal management API endpoints or DTOs |
| [unit-testing-patterns](references/unit-testing-patterns.md) | Writing or modifying unit tests |