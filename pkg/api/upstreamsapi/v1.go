// Package upstreamsapi contains DTOs for the upstreams management API.
package upstreamsapi

// ListUpstreamsResponse is the response body for GET /api/v1/upstreams.
type ListUpstreamsResponse struct {
	// Upstreams is a list of configured upstream registry aliases.
	Upstreams []string `json:"upstreams" example:"dockerhub,ghcr"`
}

// GetUpstreamResponse is the response body for GET /api/v1/upstreams/:alias.
type GetUpstreamResponse struct {
	// Alias is the unique name for this upstream registry.
	Alias string `json:"alias" example:"dockerhub"`

	// Registry is the URL or hostname of the upstream registry.
	Registry string `json:"registry" example:"registry-1.docker.io"`

	// AuthType is the type of authentication configured for this upstream.
	// Possible values: "anonymous", "basic", "bearer".
	AuthType string `json:"authType" example:"anonymous"`
}
