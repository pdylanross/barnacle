package apiroutes

import (
	"github.com/gin-gonic/gin"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/routes/apiroutes/blobsapi"
	"github.com/pdylanross/barnacle/internal/routes/apiroutes/nodesapi"
	"github.com/pdylanross/barnacle/internal/routes/apiroutes/rebalanceapi"
	"github.com/pdylanross/barnacle/internal/routes/apiroutes/upstreamsapi"
)

func Register(engine *gin.Engine, deps *dependencies.Dependencies) {
	groupV1 := engine.Group("/api/v1")

	upstreamsapi.RegisterV1(groupV1.Group("upstreams"), deps)
	nodesapi.RegisterV1(groupV1.Group("nodes"), deps)
	blobsapi.RegisterV1(groupV1.Group("nodes/:nodeId/blobs"), deps)
	rebalanceapi.RegisterV1(groupV1.Group("nodes/:nodeId/blobs"), deps)
}
