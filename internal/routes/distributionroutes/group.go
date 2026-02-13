package distributionroutes

import (
	"github.com/gin-gonic/gin"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/routes/distributionroutes/registry"
)

func Register(engine *gin.Engine, deps *dependencies.Dependencies) {
	group := engine.Group("/")

	registry.RegisterDistribution(group, deps)
}
