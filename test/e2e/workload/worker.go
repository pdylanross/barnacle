package workload

import (
	"context"
	"fmt"
	"time"

	"github.com/pdylanross/barnacle/test/e2e/framework"
	"github.com/pdylanross/barnacle/test/e2e/imagegen"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkItem represents a single unit of work for a worker.
type WorkItem struct {
	Iteration int
	Image     imagegen.ImageEntry
	ImageRef  string
}

// WorkResult represents the result of a single work item.
type WorkResult struct {
	Iteration       int              `json:"iteration"`
	ImageName       string           `json:"image_name"`
	ImageRef        string           `json:"image_ref"`
	Success         bool             `json:"success"`
	Error           string           `json:"error,omitempty"`
	Duration        float64          `json:"duration_s"`
	PullTime        float64          `json:"pull_time_s"`
	StartTime       time.Time        `json:"start_time"`
	EndTime         time.Time        `json:"end_time"`
	PodName         string           `json:"pod_name"`
	WorkerID        int              `json:"worker_id"`
	FailedPodEvents        *PodEventRecord `json:"failed_pod_events,omitempty"`
	EventualSuccessEvents  *PodEventRecord `json:"eventual_success_events,omitempty"`
}

// Worker executes work items by creating pods that pull images through barnacle.
type Worker struct {
	id           int
	framework    *framework.Framework
	workChan     <-chan WorkItem
	resultChan   chan<- WorkResult
	eventWatcher *PodEventWatcher
}

// NewWorker creates a new worker.
func NewWorker(id int, fw *framework.Framework, workChan <-chan WorkItem, resultChan chan<- WorkResult) *Worker {
	return &Worker{
		id:           id,
		framework:    fw,
		workChan:     workChan,
		resultChan:   resultChan,
		eventWatcher: NewPodEventWatcher(fw.Cluster().Clientset(), fw.Options().Namespace),
	}
}

// Run starts the worker and processes work items until the work channel is closed.
func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-w.workChan:
			if !ok {
				return
			}
			result := w.processItem(ctx, item)
			select {
			case w.resultChan <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (w *Worker) processItem(ctx context.Context, item WorkItem) WorkResult {
	result := WorkResult{
		Iteration: item.Iteration,
		ImageName: item.Image.Name,
		ImageRef:  item.ImageRef,
		StartTime: time.Now(),
		WorkerID:  w.id,
	}

	podName := fmt.Sprintf("e2e-pull-%d-%d", item.Iteration, time.Now().UnixNano()%10000)
	result.PodName = podName

	// Create the pod
	pod := w.buildPod(podName, item.ImageRef)

	createdPod, err := w.framework.Cluster().CreatePod(ctx, pod)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create pod: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime).Seconds()
		return result
	}

	// Start watching for pull timing events in a goroutine
	type pullResult struct {
		watchResult PullWatchResult
		err         error
	}
	pullCh := make(chan pullResult, 1)
	go func() {
		wr, watchErr := w.eventWatcher.WatchPullTime(ctx, podName)
		pullCh <- pullResult{watchResult: wr, err: watchErr}
	}()

	// Wait for pod to complete
	completedPod, err := w.framework.Cluster().WaitForPodComplete(ctx, podName, w.framework.Options().Timeout)

	// Collect pull time from the event watcher
	var sawPullError bool
	select {
	case pr := <-pullCh:
		if pr.err == nil {
			result.PullTime = pr.watchResult.Duration.Seconds()
			sawPullError = pr.watchResult.SawPullError
		}
	case <-time.After(5 * time.Second):
		// Timed out waiting for watcher result; leave PullTime as zero
	}

	if err != nil {
		result.Error = fmt.Sprintf("pod did not complete: %v", err)
		// Fetch events for the failed pod
		eventRecord, fetchErr := w.eventWatcher.FetchPodEvents(ctx, podName, item.ImageRef, result.Error)
		if fetchErr == nil {
			result.FailedPodEvents = eventRecord
		}
		w.cleanupPod(ctx, podName)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime).Seconds()
		return result
	}

	// Check pod status
	if completedPod.Status.Phase == corev1.PodSucceeded {
		result.Success = true
		if sawPullError {
			eventRecord, fetchErr := w.eventWatcher.FetchPodEvents(ctx, podName, item.ImageRef, "transient image pull error")
			if fetchErr == nil {
				result.EventualSuccessEvents = eventRecord
			}
		}
	} else {
		result.Error = fmt.Sprintf("pod failed with phase: %s", completedPod.Status.Phase)
		if len(completedPod.Status.ContainerStatuses) > 0 {
			cs := completedPod.Status.ContainerStatuses[0]
			if cs.State.Terminated != nil && cs.State.Terminated.Message != "" {
				result.Error = fmt.Sprintf("%s: %s", result.Error, cs.State.Terminated.Message)
			}
		}
		// Fetch events for the failed pod
		eventRecord, fetchErr := w.eventWatcher.FetchPodEvents(ctx, podName, item.ImageRef, result.Error)
		if fetchErr == nil {
			result.FailedPodEvents = eventRecord
		}
	}

	// Cleanup pod
	w.cleanupPod(ctx, podName)

	// Record timing for pod events if available
	if createdPod != nil {
		result.PodName = createdPod.Name
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).Seconds()

	return result
}

func (w *Worker) buildPod(name, imageRef string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: w.framework.Options().Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":    "e2e-pull-test",
				"app.kubernetes.io/part-of": "barnacle-e2e-test",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            "test",
					Image:           imageRef,
					ImagePullPolicy: corev1.PullAlways,
					Command:         []string{"echo", "hello world"},
				},
			},
		},
	}
}

func (w *Worker) cleanupPod(ctx context.Context, name string) {
	if w.framework.Options().DeletePods {
		// Use a separate context for cleanup to ensure it completes
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_ = w.framework.Cluster().DeletePod(cleanupCtx, name)
	}
}
