//go:build e2e

package e2e_test

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/pdylanross/barnacle/test/e2e/framework"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/klog/v2"
)

var (
	// Command line flags
	workers             = flag.Int("workers", 10, "Number of concurrent workers")
	iterations          = flag.Int("iterations", 10000, "Total number of iterations")
	timeout             = flag.Duration("timeout", 5*time.Minute, "Timeout per operation")
	kubeContext         = flag.String("kube-context", "barnacle-e2e", "Kubernetes context")
	namespace           = flag.String("namespace", "barnacle-e2e", "Kubernetes namespace")
	manifestPath        = flag.String("manifest", "e2e-images.json", "Path to image manifest")
	resultsPath         = flag.String("results", "", "Output directory for report and event data (optional)")
	verbose             = flag.Bool("verbose", false, "Enable verbose logging")
	barnacleIngressHost = flag.String("barnacle-ingress", "barnacle.test", "Barnacle ingress hostname")
	registryIngressHost = flag.String("registry-ingress", "registry.test", "Registry ingress hostname")
	barnacleNodeAddress = flag.String("barnacle-node-addr", "", "Barnacle node address (ip:port) for kubelet access, e.g., $(minikube ip):30080")
	kubeQPS             = flag.Float64("kube-qps", 20000, "Max queries per second to the K8s API server")
	kubeBurst           = flag.Int("kube-burst", 40000, "Max burst for throttle to the K8s API server")

	// Global test framework
	testFramework *framework.Framework
	testLogger    *zap.Logger
)

func TestMain(m *testing.M) {
	// Suppress noisy client-go klog messages (e.g., client-side throttling warnings).
	klog.InitFlags(nil)
	flag.Parse()
	klog.SetOutput(io.Discard)

	// Setup logger
	var logConfig zap.Config
	if *verbose {
		logConfig = zap.NewDevelopmentConfig()
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	} else {
		logConfig = zap.NewProductionConfig()
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	var err error
	testLogger, err = logConfig.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer testLogger.Sync() //nolint:errcheck

	// Build options
	opts := framework.Options{
		Workers:             *workers,
		Iterations:          *iterations,
		Timeout:             *timeout,
		KubeContext:         *kubeContext,
		Namespace:           *namespace,
		BarnacleService:     "barnacle",
		ManifestPath:        *manifestPath,
		ResultsPath:         *resultsPath,
		PodImage:            "busybox:latest",
		UpstreamName:        "local",
		KubeQPS:             float32(*kubeQPS),
		KubeBurst:           *kubeBurst,
		DeletePods:          true,
		Verbose:             *verbose,
		BarnacleIngressHost: *barnacleIngressHost,
		RegistryIngressHost: *registryIngressHost,
		BarnacleNodeAddress: *barnacleNodeAddress,
	}

	// Create framework
	testFramework, err = framework.New(opts, testLogger)
	if err != nil {
		testLogger.Fatal("Failed to create test framework", zap.Error(err))
	}

	// Setup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	if err := testFramework.Setup(ctx); err != nil {
		cancel()
		testLogger.Fatal("Failed to setup test framework", zap.Error(err))
	}
	cancel()

	// Load image manifest
	if err := testFramework.LoadImageManifest(); err != nil {
		testLogger.Fatal("Failed to load image manifest", zap.Error(err))
	}

	// Run tests
	code := m.Run()

	// Teardown
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	if err := testFramework.Teardown(ctx); err != nil {
		testLogger.Warn("Failed to teardown test framework", zap.Error(err))
	}
	cancel()

	os.Exit(code)
}
