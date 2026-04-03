package framework

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Barnacle provides helpers for interacting with the barnacle deployment.
type Barnacle struct {
	cluster     *Cluster
	serviceName string
	port        int
	ingressHost string
	nodeAddress string
	logger      *zap.Logger
}

const (
	barnaclePort      = 8080
	httpClientTimeout = 10 * time.Second
)

// NewBarnacle creates a new barnacle helper.
func NewBarnacle(cluster *Cluster, serviceName string, logger *zap.Logger) *Barnacle {
	return &Barnacle{
		cluster:     cluster,
		serviceName: serviceName,
		port:        barnaclePort,
		logger:      logger,
	}
}

// SetIngressHost sets the ingress hostname for external access.
func (b *Barnacle) SetIngressHost(host string) {
	b.ingressHost = host
}

// SetNodeAddress sets the node address (ip:port) for kubelet access.
// This is required because kubelet cannot resolve Kubernetes service DNS names.
func (b *Barnacle) SetNodeAddress(addr string) {
	b.nodeAddress = addr
}

// IngressURL returns the external URL for barnacle via ingress.
// Returns empty string if ingress host is not configured.
func (b *Barnacle) IngressURL() string {
	if b.ingressHost == "" {
		return ""
	}
	return fmt.Sprintf("http://%s", b.ingressHost)
}

// ServiceURL returns the in-cluster URL for the barnacle service.
func (b *Barnacle) ServiceURL() string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		b.serviceName,
		b.cluster.Namespace(),
		b.port,
	)
}

// ImageURL returns the URL for pulling an image through barnacle.
// If nodeAddress is set, it uses that (required for kubelet access since
// service DNS only works inside pods). Otherwise falls back to service DNS.
func (b *Barnacle) ImageURL(upstream, imageName, tag string) string {
	var host string
	if b.nodeAddress != "" {
		host = b.nodeAddress
	} else {
		host = b.serviceName + "." + b.cluster.Namespace() + ".svc.cluster.local:" + strconv.Itoa(b.port)
	}
	return fmt.Sprintf("%s/%s/%s:%s", host, upstream, imageName, tag)
}

// WaitForReady waits for the barnacle deployment to be ready.
func (b *Barnacle) WaitForReady(ctx context.Context, timeout time.Duration) error {
	b.logger.Info("Waiting for barnacle deployment to be ready",
		zap.String("deployment", b.serviceName),
		zap.Duration("timeout", timeout),
	)

	if err := b.cluster.WaitForDeploymentReady(ctx, b.serviceName, timeout); err != nil {
		return fmt.Errorf("barnacle deployment not ready: %w", err)
	}

	b.logger.Info("Barnacle deployment is ready")
	return nil
}

// CheckHealth performs a health check against barnacle.
// If an ingress host is configured, it uses the ingress URL.
// Otherwise, it falls back to the provided local port (for port-forward scenarios).
func (b *Barnacle) CheckHealth(ctx context.Context, localPort int) error {
	var url string
	if b.ingressHost != "" {
		url = fmt.Sprintf("http://%s/healthz", b.ingressHost)
		b.logger.Debug("Checking health via ingress", zap.String("url", url))
	} else {
		url = fmt.Sprintf("http://localhost:%d/healthz", localPort)
		b.logger.Debug("Checking health via localhost", zap.String("url", url))
	}

	client := &http.Client{Timeout: httpClientTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check returned %d: %s", resp.StatusCode, string(body))
	}

	b.logger.Info("Barnacle health check passed", zap.String("url", url))
	return nil
}

// CheckHealthViaIngress performs a health check against barnacle via the ingress.
// Returns an error if ingress host is not configured.
func (b *Barnacle) CheckHealthViaIngress(ctx context.Context) error {
	if b.ingressHost == "" {
		return errors.New("ingress host not configured")
	}

	url := fmt.Sprintf("http://%s/healthz", b.ingressHost)
	b.logger.Debug("Checking health via ingress", zap.String("url", url))

	client := &http.Client{Timeout: httpClientTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check via ingress failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check returned %d: %s", resp.StatusCode, string(body))
	}

	b.logger.Info("Barnacle health check via ingress passed")
	return nil
}

// GetPods returns the pods for the barnacle deployment.
func (b *Barnacle) GetPods(ctx context.Context) ([]string, error) {
	pods, err := b.cluster.ListPods(ctx, "app.kubernetes.io/name="+b.serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list barnacle pods: %w", err)
	}

	var names []string
	for _, pod := range pods.Items {
		names = append(names, pod.Name)
	}

	return names, nil
}

// GetReplicaCount returns the number of ready barnacle replicas.
func (b *Barnacle) GetReplicaCount(ctx context.Context) (int, error) {
	deployment, err := b.cluster.Clientset().AppsV1().Deployments(b.cluster.Namespace()).Get(
		ctx, b.serviceName, metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to get deployment: %w", err)
	}

	return int(deployment.Status.ReadyReplicas), nil
}

// RegistryServiceURL returns the in-cluster URL for the local registry.
func (b *Barnacle) RegistryServiceURL() string {
	return fmt.Sprintf("registry.%s.svc.cluster.local:5000", b.cluster.Namespace())
}
