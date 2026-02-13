package routes

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/pdylanross/barnacle/docs" // swagger docs
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/routes/apiroutes"
	"github.com/pdylanross/barnacle/internal/routes/distributionroutes"
)

func Register(engine *gin.Engine, deps *dependencies.Dependencies) {
	NewHealthController(deps).RegisterRoutes(engine)

	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	apiroutes.Register(engine, deps)
	distributionroutes.Register(engine, deps)
}
