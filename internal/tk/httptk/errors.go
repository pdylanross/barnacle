// Package httptk provides HTTP toolkit utilities for building API responses
// that conform to the OCI distribution specification.
package httptk

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

// OCI distribution specification error codes.
// See: https://distribution.github.io/distribution/spec/api/#errors-2
const (
	CodeBlobUnknown            = "BLOB_UNKNOWN"
	CodeBlobUploadInvalid      = "BLOB_UPLOAD_INVALID"
	CodeBlobUploadUnknown      = "BLOB_UPLOAD_UNKNOWN"
	CodeDigestInvalid          = "DIGEST_INVALID"
	CodeManifestBlobUnknown    = "MANIFEST_BLOB_UNKNOWN"
	CodeManifestInvalid        = "MANIFEST_INVALID"
	CodeManifestUnknown        = "MANIFEST_UNKNOWN"
	CodeManifestUnverified     = "MANIFEST_UNVERIFIED"
	CodeNameInvalid            = "NAME_INVALID"
	CodeNameUnknown            = "NAME_UNKNOWN"
	CodePaginationNumberInvald = "PAGINATION_NUMBER_INVALID"
	CodeRangeInvalid           = "RANGE_INVALID"
	CodeSizeInvalid            = "SIZE_INVALID"
	CodeTagInvalid             = "TAG_INVALID"
	CodeUnauthorized           = "UNAUTHORIZED"
	CodeDenied                 = "DENIED"
	CodeUnsupported            = "UNSUPPORTED"
	CodeTooManyRequests        = "TOOMANYREQUESTS"
	CodeUnknown                = "UNKNOWN"
)

// HTTPError is a general-purpose utility for constructing HTTP errors that conform
// to the OCI distribution specification error format. It implements the error interface
// and can be converted to an HTTPErrorDTO for JSON serialization.
//
// See: https://distribution.github.io/distribution/spec/api/#errors
type HTTPError struct {
	Code    string
	Message string
	Detail  any
	Status  int
}

// NewHTTPError creates a new HTTPError with the given HTTP status code, error code,
// message, and optional detail. The error code should be one of the codes defined
// in the OCI distribution specification (e.g., "BLOB_UNKNOWN", "MANIFEST_INVALID").
func NewHTTPError(status int, code string, message string, detail any) *HTTPError {
	return &HTTPError{
		Code:    code,
		Message: message,
		Detail:  detail,
		Status:  status,
	}
}

// ErrBlobUnknown returns an error indicating the blob is unknown to the registry.
// Used when a blob is requested that does not exist in the specified repository.
func ErrBlobUnknown(detail any) *HTTPError {
	return NewHTTPError(http.StatusNotFound, CodeBlobUnknown, "blob unknown to registry", detail)
}

// ErrBlobUploadInvalid returns an error indicating the blob upload is invalid.
// Used when an upload encountered an error and cannot proceed.
func ErrBlobUploadInvalid(detail any) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, CodeBlobUploadInvalid, "blob upload invalid", detail)
}

// ErrBlobUploadUnknown returns an error indicating the blob upload is unknown.
// Used when an upload was cancelled or was never started.
func ErrBlobUploadUnknown(detail any) *HTTPError {
	return NewHTTPError(http.StatusNotFound, CodeBlobUploadUnknown, "blob upload unknown to registry", detail)
}

// ErrDigestInvalid returns an error indicating the provided digest did not match.
// Used when the digest in the request does not match the uploaded content.
func ErrDigestInvalid(detail any) *HTTPError {
	return NewHTTPError(
		http.StatusBadRequest,
		CodeDigestInvalid,
		"provided digest did not match uploaded content",
		detail,
	)
}

// ErrManifestBlobUnknown returns an error indicating a manifest references an unknown blob.
// Used when a manifest upload references a blob that does not exist.
func ErrManifestBlobUnknown(detail any) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, CodeManifestBlobUnknown, "blob unknown to registry", detail)
}

// ErrManifestInvalid returns an error indicating the manifest is invalid.
// Used when validation of the manifest fails during upload.
func ErrManifestInvalid(detail any) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, CodeManifestInvalid, "manifest invalid", detail)
}

// ErrManifestUnknown returns an error indicating the manifest is unknown.
// Used when the manifest identified by name and tag is not known to the registry.
func ErrManifestUnknown(detail any) *HTTPError {
	return NewHTTPError(http.StatusNotFound, CodeManifestUnknown, "manifest unknown", detail)
}

// ErrManifestUnverified returns an error indicating manifest signature verification failed.
// Used when the manifest fails signature verification during upload.
func ErrManifestUnverified(detail any) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, CodeManifestUnverified, "manifest failed signature verification", detail)
}

// ErrNameInvalid returns an error indicating the repository name is invalid.
// Used when an invalid repository name is encountered during an operation.
func ErrNameInvalid(detail any) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, CodeNameInvalid, "invalid repository name", detail)
}

// ErrNameUnknown returns an error indicating the repository name is not known.
// Used when the repository name is not known to the registry.
func ErrNameUnknown(detail any) *HTTPError {
	return NewHTTPError(http.StatusNotFound, CodeNameUnknown, "repository name not known to registry", detail)
}

// ErrPaginationNumberInvalid returns an error indicating invalid pagination parameters.
// Used when the number of results requested is outside the acceptable range.
func ErrPaginationNumberInvalid(detail any) *HTTPError {
	return NewHTTPError(
		http.StatusBadRequest,
		CodePaginationNumberInvald,
		"invalid number of results requested",
		detail,
	)
}

// ErrRangeInvalid returns an error indicating an invalid content range.
// Used when the provided content range is out of order during a layer upload.
func ErrRangeInvalid(detail any) *HTTPError {
	return NewHTTPError(http.StatusRequestedRangeNotSatisfiable, CodeRangeInvalid, "invalid content range", detail)
}

// ErrSizeInvalid returns an error indicating a size mismatch.
// Used when the provided length does not match the content length during upload.
func ErrSizeInvalid(detail any) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, CodeSizeInvalid, "provided length did not match content length", detail)
}

// ErrTagInvalid returns an error indicating the manifest tag is invalid.
// Used when the manifest tag in the path does not match the tag in the request.
func ErrTagInvalid(detail any) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, CodeTagInvalid, "manifest tag did not match URI", detail)
}

// ErrUnauthorized returns an error indicating authentication is required.
// Used when the client fails to authenticate or authentication is required.
func ErrUnauthorized(detail any) *HTTPError {
	return NewHTTPError(http.StatusUnauthorized, CodeUnauthorized, "authentication required", detail)
}

// ErrDenied returns an error indicating access to the resource is denied.
// Used when the access controller denies access for the operation on a resource.
func ErrDenied(detail any) *HTTPError {
	return NewHTTPError(http.StatusForbidden, CodeDenied, "requested access to the resource is denied", detail)
}

// ErrUnsupported returns an error indicating the operation is unsupported.
// Used when the operation is not supported or has missing/invalid parameters.
func ErrUnsupported(detail any) *HTTPError {
	return NewHTTPError(http.StatusMethodNotAllowed, CodeUnsupported, "the operation is unsupported", detail)
}

// ErrTooManyRequests returns an error indicating rate limiting is in effect.
// Used when the client has exceeded rate limits and should slow down.
func ErrTooManyRequests(detail any) *HTTPError {
	return NewHTTPError(http.StatusTooManyRequests, CodeTooManyRequests, "too many requests", detail)
}

// TranslateTransportError inspects err to determine if it is a *transport.Error
// from go-containerregistry. If so, it translates the upstream registry's HTTP status
// and OCI error codes into an appropriate *HTTPError. If err is not a transport error,
// fallback is returned.
func TranslateTransportError(err error, fallback *HTTPError) *HTTPError {
	var transportErr *transport.Error
	if !errors.As(err, &transportErr) {
		return fallback
	}

	// Build a message from the upstream diagnostics
	var messages []string
	for _, d := range transportErr.Errors {
		messages = append(messages, d.String())
	}
	message := strings.Join(messages, "; ")
	if message == "" {
		message = err.Error()
	}

	// Use the first diagnostic code if available, otherwise derive from status
	code := CodeUnknown
	if len(transportErr.Errors) > 0 {
		code = string(transportErr.Errors[0].Code)
	}

	return NewHTTPError(transportErr.StatusCode, code, message, err.Error())
}

// Error implements the error interface, returning the error message.
func (e *HTTPError) Error() string {
	return e.Message
}

// ToDTO converts the HTTPError to an HTTPErrorDTO suitable for JSON serialization
// in API responses.
func (e *HTTPError) ToDTO() HTTPErrorDTO {
	detail := e.Detail
	if err, ok := detail.(error); ok {
		detail = err.Error()
	}

	return HTTPErrorDTO{
		Code:    e.Code,
		Message: e.Message,
		Detail:  detail,
	}
}

// ToStatus returns the HTTP status code associated with this error.
func (e *HTTPError) ToStatus() int {
	return e.Status
}

// IsBlobUnknown returns true if this error has the BLOB_UNKNOWN code.
func (e *HTTPError) IsBlobUnknown() bool {
	return e.Code == CodeBlobUnknown
}

// IsBlobUploadInvalid returns true if this error has the BLOB_UPLOAD_INVALID code.
func (e *HTTPError) IsBlobUploadInvalid() bool {
	return e.Code == CodeBlobUploadInvalid
}

// IsBlobUploadUnknown returns true if this error has the BLOB_UPLOAD_UNKNOWN code.
func (e *HTTPError) IsBlobUploadUnknown() bool {
	return e.Code == CodeBlobUploadUnknown
}

// IsDigestInvalid returns true if this error has the DIGEST_INVALID code.
func (e *HTTPError) IsDigestInvalid() bool {
	return e.Code == CodeDigestInvalid
}

// IsManifestBlobUnknown returns true if this error has the MANIFEST_BLOB_UNKNOWN code.
func (e *HTTPError) IsManifestBlobUnknown() bool {
	return e.Code == CodeManifestBlobUnknown
}

// IsManifestInvalid returns true if this error has the MANIFEST_INVALID code.
func (e *HTTPError) IsManifestInvalid() bool {
	return e.Code == CodeManifestInvalid
}

// IsManifestUnknown returns true if this error has the MANIFEST_UNKNOWN code.
func (e *HTTPError) IsManifestUnknown() bool {
	return e.Code == CodeManifestUnknown
}

// IsManifestUnverified returns true if this error has the MANIFEST_UNVERIFIED code.
func (e *HTTPError) IsManifestUnverified() bool {
	return e.Code == CodeManifestUnverified
}

// IsNameInvalid returns true if this error has the NAME_INVALID code.
func (e *HTTPError) IsNameInvalid() bool {
	return e.Code == CodeNameInvalid
}

// IsNameUnknown returns true if this error has the NAME_UNKNOWN code.
func (e *HTTPError) IsNameUnknown() bool {
	return e.Code == CodeNameUnknown
}

// IsPaginationNumberInvalid returns true if this error has the PAGINATION_NUMBER_INVALID code.
func (e *HTTPError) IsPaginationNumberInvalid() bool {
	return e.Code == CodePaginationNumberInvald
}

// IsRangeInvalid returns true if this error has the RANGE_INVALID code.
func (e *HTTPError) IsRangeInvalid() bool {
	return e.Code == CodeRangeInvalid
}

// IsSizeInvalid returns true if this error has the SIZE_INVALID code.
func (e *HTTPError) IsSizeInvalid() bool {
	return e.Code == CodeSizeInvalid
}

// IsTagInvalid returns true if this error has the TAG_INVALID code.
func (e *HTTPError) IsTagInvalid() bool {
	return e.Code == CodeTagInvalid
}

// IsUnauthorized returns true if this error has the UNAUTHORIZED code.
func (e *HTTPError) IsUnauthorized() bool {
	return e.Code == CodeUnauthorized
}

// IsDenied returns true if this error has the DENIED code.
func (e *HTTPError) IsDenied() bool {
	return e.Code == CodeDenied
}

// IsUnsupported returns true if this error has the UNSUPPORTED code.
func (e *HTTPError) IsUnsupported() bool {
	return e.Code == CodeUnsupported
}

// IsTooManyRequests returns true if this error has the TOOMANYREQUESTS code.
func (e *HTTPError) IsTooManyRequests() bool {
	return e.Code == CodeTooManyRequests
}

// IsUnknown returns true if this error has the UNKNOWN code.
func (e *HTTPError) IsUnknown() bool {
	return e.Code == CodeUnknown
}

// ToHTTPError is an interface for types that can be converted to an HTTP error response.
// Types implementing this interface can be used with HTTPErrorHandler to automatically
// generate OCI-compliant error responses.
type ToHTTPError interface {
	ToDTO() HTTPErrorDTO
	ToStatus() int
}

// HTTPErrorDTO represents a single error in the OCI distribution specification format.
// This struct is designed for JSON serialization in API error responses.
//
// See: https://distribution.github.io/distribution/spec/api/#errors
type HTTPErrorDTO struct {
	Code    string `json:"code"    example:"BLOB_UNKNOWN"`
	Message string `json:"message" example:"blob unknown to registry"`
	Detail  any    `json:"detail"`
}

// ErrorsList is the top-level response structure for OCI distribution specification
// error responses. It wraps one or more HTTPErrorDTO instances.
//
// See: https://distribution.github.io/distribution/spec/api/#errors
type ErrorsList struct {
	Errors []HTTPErrorDTO `json:"errors"`
}

// httpErrFromGenericErr converts a generic Go error to an HTTPErrorDTO with
// code "UNKNOWN". Used as a fallback when errors don't implement ToHTTPError.
func httpErrFromGenericErr(err error) HTTPErrorDTO {
	return HTTPErrorDTO{
		Code:    CodeUnknown,
		Message: err.Error(),
		Detail:  nil,
	}
}

// RedirectError represents a redirect response that should be issued to the client.
// When this error is encountered by HTTPErrorHandler, a 307 Temporary Redirect
// is issued to the specified URL instead of returning a JSON error response.
type RedirectError struct {
	// URL is the target location for the redirect.
	URL string
}

// NewRedirectError creates a new RedirectError with the given target URL.
func NewRedirectError(url string) *RedirectError {
	return &RedirectError{URL: url}
}

// NewBlobRedirectError creates a RedirectError that redirects a blob request
// to another node using the OCI distribution blob endpoint format.
func NewBlobRedirectError(nodeID, upstream, repo, digest string) *RedirectError {
	return &RedirectError{
		URL: fmt.Sprintf("http://%s/v2/%s/%s/blobs/%s", nodeID, upstream, repo, digest),
	}
}

// Error implements the error interface.
func (e *RedirectError) Error() string {
	return "redirect to " + e.URL
}

// findRedirectError checks if any error in the list is a RedirectError.
// Returns the first RedirectError found, or nil if none exists.
func findRedirectError(errs []*gin.Error) *RedirectError {
	for _, err := range errs {
		var redirectErr *RedirectError
		if errors.As(err.Err, &redirectErr) {
			return redirectErr
		}
	}
	return nil
}

// processHTTPErrors converts a list of Gin errors to HTTPErrorDTOs and determines the status code.
func processHTTPErrors(errs []*gin.Error) (int, []HTTPErrorDTO) {
	status := http.StatusInternalServerError
	errDTOs := make([]HTTPErrorDTO, 0, len(errs))

	for _, err := range errs {
		var httpErr *HTTPError
		if errors.As(err.Err, &httpErr) {
			errDTOs = append(errDTOs, httpErr.ToDTO())
			if status == http.StatusInternalServerError {
				status = httpErr.ToStatus()
			}
		} else {
			errDTOs = append(errDTOs, httpErrFromGenericErr(err.Err))
		}
	}

	return status, errDTOs
}

// HTTPErrorHandler returns a Gin middleware that processes accumulated errors
// after request handlers complete. Errors implementing ToHTTPError are converted
// to their DTO representation; other errors are wrapped with code "UNKNOWN".
// The response is formatted as an ErrorsList conforming to the OCI distribution spec.
//
// Special handling:
//   - RedirectError: Issues a 307 Temporary Redirect to the specified URL
func HTTPErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		// Check for redirect errors first - they take precedence
		if redirectErr := findRedirectError(c.Errors); redirectErr != nil {
			c.Redirect(http.StatusTemporaryRedirect, redirectErr.URL)
			return
		}

		status, errDTOs := processHTTPErrors(c.Errors)
		c.JSON(status, &ErrorsList{errDTOs})
	}
}
