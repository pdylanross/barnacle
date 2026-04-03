package framework

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Cluster provides operations for interacting with the Kubernetes cluster.
type Cluster struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
	namespace string
}

// NewCluster creates a new cluster client.
func NewCluster(kubeContext, namespace string, kubeQPS float32, kubeBurst int) (*Cluster, error) {
	config, err := buildConfig(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	config.QPS = kubeQPS
	config.Burst = kubeBurst

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Cluster{
		clientset: clientset,
		config:    config,
		namespace: namespace,
	}, nil
}

func buildConfig(context string) (*rest.Config, error) {
	// Try in-cluster config first
	if config, err := rest.InClusterConfig(); err == nil {
		return config, nil
	}

	// Fall back to kubeconfig
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}
	if context != "" {
		configOverrides.CurrentContext = context
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		configOverrides,
	).ClientConfig()
}

// Clientset returns the Kubernetes clientset.
func (c *Cluster) Clientset() *kubernetes.Clientset {
	return c.clientset
}

// Namespace returns the configured namespace.
func (c *Cluster) Namespace() string {
	return c.namespace
}

// CreatePod creates a pod in the cluster.
func (c *Cluster) CreatePod(ctx context.Context, pod *corev1.Pod) (*corev1.Pod, error) {
	return c.clientset.CoreV1().Pods(c.namespace).Create(ctx, pod, metav1.CreateOptions{})
}

// GetPod retrieves a pod by name.
func (c *Cluster) GetPod(ctx context.Context, name string) (*corev1.Pod, error) {
	return c.clientset.CoreV1().Pods(c.namespace).Get(ctx, name, metav1.GetOptions{})
}

// DeletePod deletes a pod by name.
func (c *Cluster) DeletePod(ctx context.Context, name string) error {
	return c.clientset.CoreV1().Pods(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// WaitForPodComplete waits for a pod to complete (either succeed or fail).
func (c *Cluster) WaitForPodComplete(ctx context.Context, name string, timeout time.Duration) (*corev1.Pod, error) {
	var resultPod *corev1.Pod

	err := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := c.GetPod(ctx, name)
		if err != nil {
			return false, err
		}

		resultPod = pod

		switch pod.Status.Phase {
		case corev1.PodSucceeded, corev1.PodFailed:
			return true, nil
		case corev1.PodPending, corev1.PodRunning:
			return false, nil
		default:
			return false, fmt.Errorf("unexpected pod phase: %s", pod.Status.Phase)
		}
	})

	if err != nil {
		return resultPod, fmt.Errorf("waiting for pod %s: %w", name, err)
	}

	return resultPod, nil
}

// WaitForPodRunning waits for a pod to be in running state.
func (c *Cluster) WaitForPodRunning(ctx context.Context, name string, timeout time.Duration) (*corev1.Pod, error) {
	var resultPod *corev1.Pod

	err := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := c.GetPod(ctx, name)
		if err != nil {
			return false, err
		}

		resultPod = pod

		switch pod.Status.Phase {
		case corev1.PodRunning:
			return true, nil
		case corev1.PodSucceeded, corev1.PodFailed:
			return false, fmt.Errorf("pod completed unexpectedly with phase: %s", pod.Status.Phase)
		case corev1.PodPending:
			return false, nil
		default:
			return false, fmt.Errorf("unexpected pod phase: %s", pod.Status.Phase)
		}
	})

	if err != nil {
		return resultPod, fmt.Errorf("waiting for pod %s: %w", name, err)
	}

	return resultPod, nil
}

// GetService retrieves a service by name.
func (c *Cluster) GetService(ctx context.Context, name string) (*corev1.Service, error) {
	return c.clientset.CoreV1().Services(c.namespace).Get(ctx, name, metav1.GetOptions{})
}

// ListPods lists pods matching a label selector.
func (c *Cluster) ListPods(ctx context.Context, labelSelector string) (*corev1.PodList, error) {
	return c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

// DeletePodsByLabel deletes all pods matching a label selector.
func (c *Cluster) DeletePodsByLabel(ctx context.Context, labelSelector string) error {
	return c.clientset.CoreV1().Pods(c.namespace).DeleteCollection(ctx,
		metav1.DeleteOptions{},
		metav1.ListOptions{LabelSelector: labelSelector},
	)
}

// WaitForDeploymentReady waits for a deployment to have all replicas ready.
func (c *Cluster) WaitForDeploymentReady(ctx context.Context, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		deployment, err := c.clientset.AppsV1().Deployments(c.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if deployment.Status.ReadyReplicas == *deployment.Spec.Replicas {
			return true, nil
		}

		return false, nil
	})
}

// CheckNamespaceExists checks if the namespace exists.
func (c *Cluster) CheckNamespaceExists(ctx context.Context) (bool, error) {
	_, err := c.clientset.CoreV1().Namespaces().Get(ctx, c.namespace, metav1.GetOptions{})
	if err != nil {
		return false, nil
	}
	return true, nil
}
