# Environment Dependencies

This covers the management of external development tools required to build, test, and develop the Barnacle project. All required tools must be documented here and installed via `make tools`.

## Required Tools

The following tools are required for development and are installed by `make tools`:

| Tool | Purpose | Install Command |
|------|---------|-----------------|
| `swag` | Swagger/OpenAPI documentation generation | `go install github.com/swaggo/swag/cmd/swag@latest` |
| `goimports` | Code formatting with import organization | `go install golang.org/x/tools/cmd/goimports@latest` |
| `golangci-lint` | Linting and static analysis (v2 required) | `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` |

## Adding a New Tool

When adding a new development tool dependency:

1. **Add to Makefile `tools` recipe**: Update the `tools` target in `Makefile` to include the `go install` command for the new tool.

2. **Update this document**: Add a row to the "Required Tools" table above with:
   - Tool name
   - Purpose/what it's used for
   - The exact install command

3. **Document usage**: If the tool is used by a Makefile target, ensure that target has a comment explaining what it does.

### Example: Adding a New Tool

If you need to add a new tool like `mockgen` for generating mocks:

```makefile
# In Makefile tools target, add:
tools:
	@echo "Installing development tools..."
	go install github.com/swaggo/swag/cmd/swag@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install go.uber.org/mock/mockgen@latest  # <-- new tool
	@echo "All tools installed successfully"
```

Then update this document to include mockgen in the Required Tools table.

## First-Time Setup

New developers should run:

```bash
make tools
```

This installs all required development tools. Run this command again if tools are updated or new tools are added.

## Version Pinning

Currently, tools are installed at `@latest`. If version pinning becomes necessary for reproducibility, update the install commands to use specific versions:

```makefile
go install github.com/swaggo/swag/cmd/swag@v1.16.6
```

Document any pinned versions and the reason for pinning in this file.
