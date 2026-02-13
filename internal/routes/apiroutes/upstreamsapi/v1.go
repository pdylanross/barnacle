package upstreamsapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/registry"
	upstreamdtos "github.com/pdylanross/barnacle/pkg/api/upstreamsapi"
	"github.com/pdylanross/barnacle/pkg/configuration"
)

// RegisterV1 registers the v1 upstreams API routes on the provided router group.
// It creates a new controller instance and binds the following endpoints:
//   - GET / - List all configured upstream registries
//   - GET /:alias - Get details for a specific upstream registry by alias
func RegisterV1(group *gin.RouterGroup, deps *dependencies.Dependencies) {
	controller := newControllerV1(deps)
	group.GET("/", controller.List)
	group.GET("/:alias", controller.Get)
}

// newControllerV1 creates a new v1 upstreams API controller with the provided dependencies.
func newControllerV1(deps *dependencies.Dependencies) *upstreamControllerV1 {
	return &upstreamControllerV1{
		registry:  deps.UpstreamRegistry(),
		upstreams: deps.Config().Upstreams,
	}
}

// upstreamControllerV1 handles HTTP requests for the v1 upstreams API.
type upstreamControllerV1 struct {
	registry  *registry.UpstreamRegistry
	upstreams map[string]configuration.UpstreamConfiguration
}

// List handles GET / requests and returns a list of all configured upstream registry aliases.
// Returns [http.StatusOK] with a [upstreamdtos.ListUpstreamsResponse].
//
// @Summary      List upstreams
// @Description  Returns a list of all configured upstream registry aliases
// @Tags         upstreams
// @Produce      json
// @Success      200  {object}  upstreamsapi.ListUpstreamsResponse
// @Router       /api/v1/upstreams [get].
func (c *upstreamControllerV1) List(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, upstreamdtos.ListUpstreamsResponse{
		Upstreams: c.registry.ListUpstreams(),
	})
}

// Get handles GET /:alias requests and returns details for a specific upstream registry.
// Returns [http.StatusOK] with a [upstreamdtos.GetUpstreamResponse] if found.
// Returns [http.StatusNotFound] if the upstream alias does not exist.
//
// @Summary      Get upstream
// @Description  Returns details for a specific upstream registry
// @Tags         upstreams
// @Produce      json
// @Param        alias  path      string  true  "Upstream alias"
// @Success      200    {object}  upstreamsapi.GetUpstreamResponse
// @Failure      404    {object}  httptk.ErrorsList
// @Router       /api/v1/upstreams/{alias} [get].
func (c *upstreamControllerV1) Get(ctx *gin.Context) {
	alias := ctx.Param("alias")

	// Verify upstream exists in the registry
	_, err := c.registry.GetUpstream(alias)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	cfg := c.upstreams[alias]

	ctx.JSON(http.StatusOK, upstreamdtos.GetUpstreamResponse{
		Alias:    alias,
		Registry: cfg.Registry,
		AuthType: authType(&cfg.Authentication),
	})
}

// authType returns a string identifying the configured authentication type.
func authType(auth *configuration.UpstreamAuthentication) string {
	if auth.Basic != nil {
		return "basic"
	}
	if auth.Bearer != nil {
		return "bearer"
	}
	return "anonymous"
}
