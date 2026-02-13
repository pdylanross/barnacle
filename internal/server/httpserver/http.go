package httpserver

import (
	"context"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/routes"
	"github.com/pdylanross/barnacle/internal/tasks"
	"github.com/pdylanross/barnacle/internal/tk"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	"go.uber.org/zap"
)

// HTTPServer represents the HTTP server with its dependencies, logger, and routing components.
type HTTPServer struct {
	deps   *dependencies.Dependencies
	logger *zap.Logger

	router *gin.Engine
}

// NewHTTPServer creates and initializes an HTTPServer instance with provided dependencies and middleware.
func NewHTTPServer(deps *dependencies.Dependencies) (*HTTPServer, error) {
	logger := deps.Logger().Named("http-server")

	if !tk.IsDevelopment() {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	router := gin.New()
	router.Use(ginzap.Ginzap(logger, time.RFC3339, true))
	router.Use(ginzap.RecoveryWithZap(logger, true))
	router.Use(httptk.HTTPErrorHandler())

	routes.Register(router, deps)

	return &HTTPServer{
		deps:   deps,
		logger: logger,
		router: router,
	}, nil
}

// Start initializes and registers the HTTP server task with the task runner for execution.
func (s *HTTPServer) Start() {
	s.deps.TaskRunner().AddTask("http-server", s.makeTask())
}

func (s *HTTPServer) makeTask() tasks.Task {
	return tasks.NewOneShot(func(ctx context.Context) error {
		srv := s.deps.Config().Server.BuildHTTP()
		srv.Handler = s.router.Handler()

		errChan := make(chan error)
		go func() {
			s.logger.Info("starting HTTP server", zap.String("address", srv.Addr))
			errChan <- srv.ListenAndServe()
		}()

		select {
		case err := <-errChan:
			return err
		case <-ctx.Done():
			return srv.Shutdown(ctx)
		}
	})
}
