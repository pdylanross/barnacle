package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pdylanross/barnacle/internal/dependencies"
)

// HealthController is responsible for managing health-related HTTP endpoints in the application.
type HealthController struct{}

// NewHealthController creates and returns a new instance of HealthController with the provided application dependencies.
func NewHealthController(_ *dependencies.Dependencies) *HealthController {
	return &HealthController{}
}

// RegisterRoutes configures HTTP routes for health-related endpoints using the provided Gin engine.
// Registers the following endpoints:
//   - GET /health - Returns a health check status
func (hc *HealthController) RegisterRoutes(router *gin.Engine) {
	router.GET("/health", hc.healthCheck)
}

// healthCheck handles the HTTP GET request for the health check endpoint.
// Returns [http.StatusOK] with a JSON response containing {"status": "ok"}.
//
// @Summary      Health check
// @Description  Returns the health status of the service
// @Tags         system
// @Produce      json
// @Success      200  {object}  map[string]string  "Health status"
// @Router       /health [get].
func (hc *HealthController) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
