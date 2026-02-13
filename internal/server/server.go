package server

import (
	"context"

	"github.com/pdylanross/barnacle/internal/dependencies"
	"github.com/pdylanross/barnacle/internal/server/httpserver"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"go.uber.org/zap"
)

// Server manages the lifecycle of the barnacle server and its dependencies.
type Server struct {
	deps *dependencies.Dependencies

	http *httpserver.HTTPServer
}

// NewServer creates a new Server instance with initialized dependencies.
// Returns an error if dependency initialization fails.
func NewServer(ctx context.Context, config *configuration.Configuration, logger *zap.Logger) (*Server, error) {
	deps, err := dependencies.NewDependencies(ctx, config, logger)
	if err != nil {
		return nil, err
	}

	httpServer, err := httpserver.NewHTTPServer(deps)
	if err != nil {
		_ = deps.Close()
		return nil, err
	}

	return &Server{
		deps: deps,
		http: httpServer,
	}, nil
}

// Run starts the server and blocks until it stops or encounters an error.
// Returns an error if the server fails during execution.
func (s *Server) Run() error {
	s.http.Start()
	s.startNodeRegistry()
	s.startRebalanceLeader()
	s.startRebalanceWorker()

	return s.deps.TaskRunner().Wait()
}

// startNodeRegistry starts the node registry background task.
func (s *Server) startNodeRegistry() {
	s.deps.TaskRunner().AddTask("node-registry", s.deps.NodeRegistry().MakeTask())
}

// startRebalanceLeader starts the rebalance leader election background task.
func (s *Server) startRebalanceLeader() {
	s.deps.TaskRunner().AddTask("rebalance-leader", s.deps.RebalanceLeader().MakeTask())
}

// startRebalanceWorker starts the rebalance worker background task.
func (s *Server) startRebalanceWorker() {
	if s.deps.Config().Rebalance.Enabled {
		s.deps.TaskRunner().AddTask("rebalance-worker", s.deps.RebalanceWorker().MakeTask())
	}
}

// Shutdown initiates a graceful shutdown of the server.
// It signals all running tasks to stop. Call this method from a separate goroutine
// while Run() is blocking to trigger shutdown.
func (s *Server) Shutdown() {
	s.deps.TaskRunner().Shutdown()
}

// Close releases all resources held by the server.
func (s *Server) Close() error {
	return s.deps.Close()
}
