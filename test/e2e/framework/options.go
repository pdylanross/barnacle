package framework

import (
	"time"
)

// Options holds the configuration for e2e tests.
type Options struct {
	// Workers is the number of concurrent workers pulling images
	Workers int

	// Iterations is the total number of image pulls to perform
	Iterations int

	// Timeout is the maximum time to wait for a single pod operation
	Timeout time.Duration

	// KubeContext is the Kubernetes context to use
	KubeContext string

	// Namespace is the Kubernetes namespace where barnacle is deployed
	Namespace string

	// BarnacleService is the name of the barnacle service
	BarnacleService string

	// ManifestPath is the path to the image manifest JSON file
	ManifestPath string

	// ResultsPath is the directory path for test output (report.json, failed-pod-events.json)
	ResultsPath string

	// PodImage is the container image used for worker pods
	PodImage string

	// UpstreamName is the barnacle upstream name to use
	UpstreamName string

	// KubeQPS is the maximum queries per second to the K8s API server
	KubeQPS float32

	// KubeBurst is the maximum burst for throttle to the K8s API server
	KubeBurst int

	// DeletePods controls whether to delete pods after completion
	DeletePods bool

	// Verbose enables verbose logging
	Verbose bool

	// BarnacleIngressHost is the ingress hostname for barnacle (e.g., "barnacle.test")
	// If set, external health checks will use this instead of port-forwarding
	BarnacleIngressHost string

	// RegistryIngressHost is the ingress hostname for the registry (e.g., "registry.test")
	// If set, external registry access will use this instead of port-forwarding
	RegistryIngressHost string

	// BarnacleNodeAddress is the node address for barnacle (e.g., "192.168.49.2:30080")
	// This is required for kubelet to pull images since service DNS only works inside pods.
	// Use minikube ip to get the node IP, combined with the NodePort (30080).
	BarnacleNodeAddress string
}

// DefaultOptions returns the default options for e2e tests.
func DefaultOptions() Options {
	return Options{
		Workers:             10,
		Iterations:          10000,
		Timeout:             5 * time.Minute,
		KubeContext:         "barnacle-e2e",
		Namespace:           "barnacle-e2e",
		BarnacleService:     "barnacle",
		ManifestPath:        "e2e-images.json",
		ResultsPath:         "",
		PodImage:            "busybox:latest",
		UpstreamName:        "local",
		KubeQPS:             20000,
		KubeBurst:           40000,
		DeletePods:          true,
		Verbose:             false,
		BarnacleIngressHost: "barnacle.test",
		RegistryIngressHost: "registry.test",
	}
}

// Validate validates the options.
func (o *Options) Validate() error {
	if o.Workers < 1 {
		return ErrInvalidWorkers
	}
	if o.Iterations < 1 {
		return ErrInvalidIterations
	}
	if o.Timeout < time.Second {
		return ErrInvalidTimeout
	}
	if o.Namespace == "" {
		return ErrInvalidNamespace
	}
	if o.BarnacleService == "" {
		return ErrInvalidService
	}
	if o.ManifestPath == "" {
		return ErrInvalidManifestPath
	}
	return nil
}

// Error types for validation.
var (
	ErrInvalidWorkers      = validationError("workers must be at least 1")
	ErrInvalidIterations   = validationError("iterations must be at least 1")
	ErrInvalidTimeout      = validationError("timeout must be at least 1 second")
	ErrInvalidNamespace    = validationError("namespace cannot be empty")
	ErrInvalidService      = validationError("barnacle service cannot be empty")
	ErrInvalidManifestPath = validationError("manifest path cannot be empty")
)

type validationError string

func (e validationError) Error() string {
	return string(e)
}
