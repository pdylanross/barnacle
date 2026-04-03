package workload

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Report contains aggregated results from a workload run.
type Report struct {
	Summary     Summary               `json:"summary"`
	Latencies   Latencies             `json:"latencies"`
	ByImage     map[string]ImageStats `json:"by_image"`
	Results     []WorkResult          `json:"results,omitempty"`
	GeneratedAt time.Time             `json:"generated_at"`
}

// Summary contains high-level statistics.
type Summary struct {
	TotalIterations      int     `json:"total_iterations"`
	SuccessCount         int     `json:"success_count"`
	FailureCount         int     `json:"failure_count"`
	SuccessRate          float64 `json:"success_rate"`
	EventualSuccessCount int     `json:"eventual_success_count"`
	TotalDuration        float64 `json:"total_duration_s"`
}

// Latencies contains latency percentiles in seconds.
type Latencies struct {
	Min    float64 `json:"min_s"`
	Max    float64 `json:"max_s"`
	Mean   float64 `json:"mean_s"`
	Median float64 `json:"median_s"`
	P50    float64 `json:"p50_s"`
	P90    float64 `json:"p90_s"`
	P95    float64 `json:"p95_s"`
	P99    float64 `json:"p99_s"`
}

// ImageStats contains per-image statistics.
type ImageStats struct {
	ImageName    string  `json:"image_name"`
	PullCount    int     `json:"pull_count"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	MeanLatency  float64 `json:"mean_latency_s"`
	MinLatency   float64 `json:"min_latency_s"`
	MaxLatency   float64 `json:"max_latency_s"`
}

// Reporter aggregates and reports workload results.
type Reporter struct {
	results    []WorkResult
	includeRaw bool
}

// NewReporter creates a new reporter.
func NewReporter(results []WorkResult, includeRaw bool) *Reporter {
	return &Reporter{
		results:    results,
		includeRaw: includeRaw,
	}
}

const percentMultiplier = 100

// Generate generates a report from the results.
func (r *Reporter) Generate() *Report {
	report := &Report{
		GeneratedAt: time.Now(),
		ByImage:     make(map[string]ImageStats),
	}

	if r.includeRaw {
		report.Results = r.results
	}

	if len(r.results) == 0 {
		return report
	}

	// Calculate summary
	var totalDuration float64
	successCount := 0
	failureCount := 0
	eventualSuccessCount := 0

	for _, result := range r.results {
		totalDuration += result.Duration
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
		if result.EventualSuccessEvents != nil {
			eventualSuccessCount++
		}
	}

	report.Summary = Summary{
		TotalIterations:      len(r.results),
		SuccessCount:         successCount,
		FailureCount:         failureCount,
		SuccessRate:          float64(successCount) / float64(len(r.results)) * percentMultiplier,
		EventualSuccessCount: eventualSuccessCount,
		TotalDuration:        totalDuration,
	}

	// Calculate latencies
	report.Latencies = r.calculateLatencies()

	// Calculate per-image stats
	imageResults := make(map[string][]WorkResult)
	for _, result := range r.results {
		imageResults[result.ImageName] = append(imageResults[result.ImageName], result)
	}

	for imageName, results := range imageResults {
		stats := r.calculateImageStats(imageName, results)
		report.ByImage[imageName] = stats
	}

	return report
}

const (
	p50 = 50
	p90 = 90
	p95 = 95
	p99 = 99
)

func (r *Reporter) calculateLatencies() Latencies {
	// Filter out zero-duration results where the watch didn't capture timing
	var durations []float64
	for _, result := range r.results {
		if result.PullTime > 0 {
			durations = append(durations, result.PullTime)
		}
	}

	if len(durations) == 0 {
		return Latencies{}
	}

	sort.Float64s(durations)

	// Calculate stats
	var total float64
	for _, d := range durations {
		total += d
	}

	n := len(durations)
	mean := total / float64(n)

	return Latencies{
		Min:    durations[0],
		Max:    durations[n-1],
		Mean:   mean,
		Median: durations[n/2],
		P50:    percentile(durations, p50),
		P90:    percentile(durations, p90),
		P95:    percentile(durations, p95),
		P99:    percentile(durations, p99),
	}
}

func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / percentMultiplier
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (r *Reporter) calculateImageStats(imageName string, results []WorkResult) ImageStats {
	stats := ImageStats{
		ImageName: imageName,
		PullCount: len(results),
	}

	if len(results) == 0 {
		return stats
	}

	var totalLatency float64
	stats.MinLatency = results[0].PullTime
	stats.MaxLatency = results[0].PullTime

	for _, result := range results {
		if result.Success {
			stats.SuccessCount++
		} else {
			stats.FailureCount++
		}
		totalLatency += result.PullTime
		if result.PullTime < stats.MinLatency {
			stats.MinLatency = result.PullTime
		}
		if result.PullTime > stats.MaxLatency {
			stats.MaxLatency = result.PullTime
		}
	}

	stats.MeanLatency = totalLatency / float64(len(results))
	return stats
}

const filePermissions = 0644

// WriteJSON writes the report to a JSON file.
func (r *Report) WriteJSON(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	if writeErr := os.WriteFile(path, data, filePermissions); writeErr != nil {
		return fmt.Errorf("failed to write report: %w", writeErr)
	}

	return nil
}

const dirPermissions = 0755

// WriteOutputDir writes the report and failed pod events to an output directory.
func (r *Reporter) WriteOutputDir(dir string) error {
	if mkdirErr := os.MkdirAll(dir, dirPermissions); mkdirErr != nil {
		return fmt.Errorf("failed to create output directory: %w", mkdirErr)
	}

	report := r.Generate()

	// Write report.json
	if reportErr := report.WriteJSON(filepath.Join(dir, "report.json")); reportErr != nil {
		return reportErr
	}

	// Collect failed pod events from results
	var failedEvents []PodEventRecord
	for _, result := range r.results {
		if result.FailedPodEvents != nil {
			failedEvents = append(failedEvents, *result.FailedPodEvents)
		}
	}

	// Write failed-pod-events.json
	eventsData, marshalErr := json.MarshalIndent(failedEvents, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal failed pod events: %w", marshalErr)
	}

	eventsPath := filepath.Join(dir, "failed-pod-events.json")
	if writeErr := os.WriteFile(eventsPath, eventsData, filePermissions); writeErr != nil {
		return fmt.Errorf("failed to write failed pod events: %w", writeErr)
	}

	// Collect eventual success events from results
	var eventualSuccessEvents []PodEventRecord
	for _, result := range r.results {
		if result.EventualSuccessEvents != nil {
			eventualSuccessEvents = append(eventualSuccessEvents, *result.EventualSuccessEvents)
		}
	}

	// Write eventual-success-events.json
	esData, esMarshalErr := json.MarshalIndent(eventualSuccessEvents, "", "  ")
	if esMarshalErr != nil {
		return fmt.Errorf("failed to marshal eventual success events: %w", esMarshalErr)
	}

	esPath := filepath.Join(dir, "eventual-success-events.json")
	if esWriteErr := os.WriteFile(esPath, esData, filePermissions); esWriteErr != nil {
		return fmt.Errorf("failed to write eventual success events: %w", esWriteErr)
	}

	return nil
}

// PrintSummary prints a human-readable summary to stdout.
func (r *Report) PrintSummary() {
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "=== E2E Test Results ===")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "Total Iterations:     %d\n", r.Summary.TotalIterations)
	fmt.Fprintf(os.Stdout, "Success Count:        %d\n", r.Summary.SuccessCount)
	fmt.Fprintf(os.Stdout, "Failure Count:        %d\n", r.Summary.FailureCount)
	fmt.Fprintf(os.Stdout, "Eventual Successes:   %d\n", r.Summary.EventualSuccessCount)
	fmt.Fprintf(os.Stdout, "Success Rate:         %.2f%%\n", r.Summary.SuccessRate)
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Latency Percentiles (Pull Time):")
	fmt.Fprintf(os.Stdout, "  Min:    %.3fs\n", r.Latencies.Min)
	fmt.Fprintf(os.Stdout, "  P50:    %.3fs\n", r.Latencies.P50)
	fmt.Fprintf(os.Stdout, "  P90:    %.3fs\n", r.Latencies.P90)
	fmt.Fprintf(os.Stdout, "  P95:    %.3fs\n", r.Latencies.P95)
	fmt.Fprintf(os.Stdout, "  P99:    %.3fs\n", r.Latencies.P99)
	fmt.Fprintf(os.Stdout, "  Max:    %.3fs\n", r.Latencies.Max)
	fmt.Fprintf(os.Stdout, "  Mean:   %.3fs\n", r.Latencies.Mean)
	fmt.Fprintln(os.Stdout)
}
