# Error Handling

## Sentinel Errors
- **NEVER use inline `fmt.Errorf()` calls in function returns** - Always use global error variables
- Define global error variables at package level using `errors.New()`:
  ```go
  var (
      ErrLoadYAMLFiles = errors.New("failed to load YAML files")
      ErrReadFile      = errors.New("failed to read file")
  )
  ```
- Wrap errors using `fmt.Errorf()` with the global error variable and `%w`:
  ```go
  return fmt.Errorf("%w: %w", ErrLoadYAMLFiles, err)
  // Or with context:
  return fmt.Errorf("%w %s: %w", ErrReadFile, filename, err)
  ```
- **Why this pattern?** Global error variables enable callers to use `errors.Is()` and `errors.As()` to check for specific error conditions, making error handling more robust and testable
- Always wrap underlying errors with `%w` to preserve the error chain
- Include relevant context (filenames, paths, etc.) between the sentinel error and wrapped error

## HTTP API Errors
- **ALWAYS use `internal/tk/httptk` error factories for HTTP API errors** - These conform to the OCI distribution specification
- Use pre-defined factory methods instead of constructing errors manually:
  ```go
  // GOOD - Use factory methods:
  return httptk.ErrManifestUnknown(nil)
  return httptk.ErrBlobUnknown(map[string]string{"digest": digest})
  return httptk.ErrUnauthorized(nil)

  // BAD - Don't construct manually:
  return httptk.NewHTTPError(404, "MANIFEST_UNKNOWN", "manifest unknown", nil)
  ```
- Available factory methods (see `internal/tk/httptk/errors.go`):
    - `ErrBlobUnknown`, `ErrBlobUploadInvalid`, `ErrBlobUploadUnknown`
    - `ErrDigestInvalid`, `ErrManifestBlobUnknown`, `ErrManifestInvalid`
    - `ErrManifestUnknown`, `ErrManifestUnverified`, `ErrNameInvalid`
    - `ErrNameUnknown`, `ErrPaginationNumberInvalid`, `ErrRangeInvalid`
    - `ErrSizeInvalid`, `ErrTagInvalid`, `ErrUnauthorized`, `ErrDenied`
    - `ErrUnsupported`, `ErrTooManyRequests`
- Pass contextual details via the `detail` parameter when helpful for debugging
- Use `NewHTTPError()` only for custom errors not covered by the OCI spec
- **ALWAYS discard the return value from `c.Error()`** when attaching errors to a Gin context:
  ```go
  // GOOD:
  _ = c.Error(httptk.ErrManifestUnknown(nil))

  // BAD:
  c.Error(httptk.ErrManifestUnknown(nil))  // Linter will warn about unchecked error
   ```
