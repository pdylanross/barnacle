package framework

import (
	"context"
	"fmt"
	"time"

	"github.com/pdylanross/barnacle/test/e2e/imagegen"
	"go.uber.org/zap"
)

// Framework is the main orchestrator for e2e tests.
type Framework struct {
	options  Options
	cluster  *Cluster
	barnacle *Barnacle
	manifest *imagegen.ImageManifest
	logger   *zap.Logger
}

// New creates a new test framework.
func New(options Options, logger *zap.Logger) (*Framework, error) {
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("failed to create logger: %w", err)
		}
	}

	return &Framework{
		options: options,
		logger:  logger,
	}, nil
}

// Setup initializes the framework by connecting to the cluster and verifying barnacle is ready.
func (f *Framework) Setup(ctx context.Context) error {
	f.logger.Info("Setting up e2e test framework",
		zap.String("context", f.options.KubeContext),
		zap.String("namespace", f.options.Namespace),
	)

	// Connect to cluster
	cluster, err := NewCluster(f.options.KubeContext, f.options.Namespace, f.options.KubeQPS, f.options.KubeBurst)
	if err != nil {
		return fmt.Errorf("failed to connect to cluster: %w", err)
	}
	f.cluster = cluster

	// Verify namespace exists
	exists, err := cluster.CheckNamespaceExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check namespace: %w", err)
	}
	if !exists {
		return fmt.Errorf("namespace %s does not exist", f.options.Namespace)
	}

	// Create barnacle helper
	f.barnacle = NewBarnacle(cluster, f.options.BarnacleService, f.logger)
	if f.options.BarnacleIngressHost != "" {
		f.barnacle.SetIngressHost(f.options.BarnacleIngressHost)
	}
	if f.options.BarnacleNodeAddress != "" {
		f.barnacle.SetNodeAddress(f.options.BarnacleNodeAddress)
	}

	// Wait for barnacle to be ready
	const readyTimeout = 2 * time.Minute
	if readyErr := f.barnacle.WaitForReady(ctx, readyTimeout); readyErr != nil {
		return fmt.Errorf("barnacle not ready: %w", readyErr)
	}

	f.logger.Info("Framework setup complete")
	return nil
}

// LoadImageManifest loads the image manifest from the specified path.
func (f *Framework) LoadImageManifest() error {
	f.logger.Info("Loading image manifest", zap.String("path", f.options.ManifestPath))

	manifest, err := imagegen.LoadManifest(f.options.ManifestPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	f.manifest = manifest

	f.logger.Info("Loaded image manifest",
		zap.Int("total_images", manifest.Statistics.TotalImages),
		zap.Int64("total_size", manifest.Statistics.TotalSize),
	)

	return nil
}

// Manifest returns the loaded image manifest.
func (f *Framework) Manifest() *imagegen.ImageManifest {
	return f.manifest
}

// Cluster returns the cluster client.
func (f *Framework) Cluster() *Cluster {
	return f.cluster
}

// Barnacle returns the barnacle helper.
func (f *Framework) Barnacle() *Barnacle {
	return f.barnacle
}

// Options returns the framework options.
func (f *Framework) Options() Options {
	return f.options
}

// Logger returns the framework logger.
func (f *Framework) Logger() *zap.Logger {
	return f.logger
}

// Teardown cleans up resources created during the test.
func (f *Framework) Teardown(ctx context.Context) error {
	f.logger.Info("Tearing down e2e test framework")

	if f.options.DeletePods && f.cluster != nil {
		f.logger.Info("Cleaning up test pods")
		if err := f.cluster.DeletePodsByLabel(ctx, "app.kubernetes.io/part-of=barnacle-e2e-test"); err != nil {
			f.logger.Warn("Failed to delete test pods", zap.Error(err))
		}
	}

	f.logger.Info("Framework teardown complete")
	return nil
}

// GetImageForIteration returns the image entry to use for a given iteration.
// Uses round-robin selection across all available images.
func (f *Framework) GetImageForIteration(iteration int) imagegen.ImageEntry {
	if f.manifest == nil || len(f.manifest.Images) == 0 {
		return imagegen.ImageEntry{}
	}

	idx := iteration % len(f.manifest.Images)
	return f.manifest.Images[idx]
}

// BuildPodImageRef builds the full image reference for pulling through barnacle.
func (f *Framework) BuildPodImageRef(image imagegen.ImageEntry) string {
	return f.barnacle.ImageURL(f.options.UpstreamName, image.Name, image.Tag)
}
