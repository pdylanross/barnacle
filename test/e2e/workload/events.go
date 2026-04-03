package workload

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// PodEventRecord holds K8s events for a failed pod.
type PodEventRecord struct {
	PodName        string            `json:"pod_name"`
	Namespace      string            `json:"namespace"`
	ContainerImage string            `json:"container_image"`
	Labels         map[string]string `json:"labels"`
	StartTime      *metav1.Time      `json:"start_time,omitempty"`
	FailureReason  string            `json:"failure_reason"`
	Events         []PodEvent        `json:"events"`
}

// PodEvent represents a single K8s event associated with a pod.
type PodEvent struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Count     int32     `json:"count"`
	Source    string    `json:"source"`
}

// PodEventWatcher watches K8s Event resources for a specific pod.
type PodEventWatcher struct {
	clientset kubernetes.Interface
	namespace string
}

// NewPodEventWatcher creates a new PodEventWatcher.
func NewPodEventWatcher(clientset kubernetes.Interface, namespace string) *PodEventWatcher {
	return &PodEventWatcher{
		clientset: clientset,
		namespace: namespace,
	}
}

// PullWatchResult holds the outcome of watching pull-related events on a pod.
type PullWatchResult struct {
	Duration     time.Duration
	SawPullError bool
}

// WatchPullTime watches for kubelet Pulling/Pulled events on a pod and returns
// the duration between them. Returns zero duration if the events aren't observed
// (e.g., image already cached on the node). SawPullError is set when transient
// image-pull errors (Failed, BackOff) are observed before the pull succeeds.
func (w *PodEventWatcher) WatchPullTime(ctx context.Context, podName string) (PullWatchResult, error) {
	selector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", podName),
		fields.OneTermEqualSelector("involvedObject.kind", "Pod"),
	)

	watcher, err := w.clientset.CoreV1().Events(w.namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: selector.String(),
	})
	if err != nil {
		return PullWatchResult{}, fmt.Errorf("failed to watch events for pod %s: %w", podName, err)
	}
	defer watcher.Stop()

	// We timestamp events at receipt rather than relying on K8s event timestamps,
	// which only have second-level precision in the core/v1 Events API.
	var pullingReceived time.Time
	var sawPullError bool

	for {
		select {
		case <-ctx.Done():
			return PullWatchResult{SawPullError: sawPullError}, nil
		case evt, ok := <-watcher.ResultChan():
			if !ok {
				return PullWatchResult{SawPullError: sawPullError}, nil
			}
			if evt.Type == watch.Error {
				continue
			}
			event, ok := evt.Object.(*corev1.Event)
			if !ok {
				continue
			}
			switch event.Reason {
			case "Pulling":
				pullingReceived = time.Now()
			case "Pulled":
				result := PullWatchResult{SawPullError: sawPullError}
				if !pullingReceived.IsZero() {
					result.Duration = time.Since(pullingReceived)
				}
				return result, nil
			case "Failed", "BackOff":
				sawPullError = true
			}
		}
	}
}

// FetchPodEvents lists all events for a pod and returns a structured PodEventRecord.
func (w *PodEventWatcher) FetchPodEvents(
	ctx context.Context,
	podName, containerImage, failureReason string,
) (*PodEventRecord, error) {
	selector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", podName),
		fields.OneTermEqualSelector("involvedObject.kind", "Pod"),
	)

	eventList, err := w.clientset.CoreV1().Events(w.namespace).List(ctx, metav1.ListOptions{
		FieldSelector: selector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list events for pod %s: %w", podName, err)
	}

	record := &PodEventRecord{
		PodName:        podName,
		Namespace:      w.namespace,
		ContainerImage: containerImage,
		FailureReason:  failureReason,
		Events:         make([]PodEvent, 0, len(eventList.Items)),
	}

	for _, e := range eventList.Items {
		firstSeen := e.EventTime.Time
		if firstSeen.IsZero() {
			firstSeen = e.FirstTimestamp.Time
		}
		lastSeen := e.LastTimestamp.Time
		if lastSeen.IsZero() {
			lastSeen = firstSeen
		}

		record.Events = append(record.Events, PodEvent{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			FirstSeen: firstSeen,
			LastSeen:  lastSeen,
			Count:     e.Count,
			Source:    e.Source.Component,
		})
	}

	return record, nil
}
