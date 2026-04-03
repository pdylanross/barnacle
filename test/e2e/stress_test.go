//go:build e2e

package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/pdylanross/barnacle/test/e2e/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestStressImagePulls runs the main stress test that pulls images through barnacle.
func TestStressImagePulls(t *testing.T) {
	require.NotNil(t, testFramework, "test framework not initialized")
	require.NotNil(t, testFramework.Manifest(), "image manifest not loaded")

	opts := testFramework.Options()
	t.Logf("Running stress test with %d workers, %d iterations", opts.Workers, opts.Iterations)
	t.Logf("Using %d images from manifest", len(testFramework.Manifest().Images))

	// Create scheduler
	scheduler := workload.NewScheduler(testFramework, testLogger)

	// Create context with overall timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	// Run workload with progress reporting
	lastProgress := 0
	results, err := scheduler.RunWithProgress(ctx, func(completed, total int) {
		pct := (completed * 100) / total
		if pct >= lastProgress+10 {
			t.Logf("Progress: %d/%d (%d%%)", completed, total, pct)
			lastProgress = pct
		}
	})
	require.NoError(t, err, "workload execution failed")

	// Generate report
	reporter := workload.NewReporter(results, opts.ResultsPath != "")
	report := reporter.Generate()

	// Print summary
	report.PrintSummary()

	// Write results to output directory if path specified
	if opts.ResultsPath != "" {
		err := reporter.WriteOutputDir(opts.ResultsPath)
		if err != nil {
			t.Logf("Warning: failed to write results to %s: %v", opts.ResultsPath, err)
		} else {
			t.Logf("Results written to %s", opts.ResultsPath)
		}
	}

	// Assert success rate > 99%
	assert.GreaterOrEqual(t, report.Summary.SuccessRate, 99.0,
		"success rate should be >= 99%%, got %.2f%%", report.Summary.SuccessRate)

	// Log additional metrics
	testLogger.Info("Stress test completed",
		zap.Int("total", report.Summary.TotalIterations),
		zap.Int("success", report.Summary.SuccessCount),
		zap.Int("failure", report.Summary.FailureCount),
		zap.Float64("success_rate", report.Summary.SuccessRate),
		zap.Float64("p50_s", report.Latencies.P50),
		zap.Float64("p95_s", report.Latencies.P95),
		zap.Float64("p99_s", report.Latencies.P99),
	)
}

// TestBarnacleHealth verifies barnacle is healthy before running stress tests.
func TestBarnacleHealth(t *testing.T) {
	require.NotNil(t, testFramework, "test framework not initialized")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get replica count
	replicas, err := testFramework.Barnacle().GetReplicaCount(ctx)
	require.NoError(t, err, "failed to get replica count")
	assert.GreaterOrEqual(t, replicas, 2, "should have at least 2 replicas")

	t.Logf("Barnacle has %d ready replicas", replicas)

	// Get pod names
	pods, err := testFramework.Barnacle().GetPods(ctx)
	require.NoError(t, err, "failed to get pods")

	for _, pod := range pods {
		t.Logf("Barnacle pod: %s", pod)
	}
}

// TestImageManifest verifies the image manifest is loaded correctly.
func TestImageManifest(t *testing.T) {
	require.NotNil(t, testFramework, "test framework not initialized")
	require.NotNil(t, testFramework.Manifest(), "image manifest not loaded")

	manifest := testFramework.Manifest()

	t.Logf("Image manifest stats:")
	t.Logf("  Total images: %d", manifest.Statistics.TotalImages)
	t.Logf("  Total size: %d bytes", manifest.Statistics.TotalSize)
	t.Logf("  Average size: %d bytes", manifest.Statistics.AverageSize)
	t.Logf("  Total layers: %d", manifest.Statistics.TotalLayers)
	t.Logf("  Images with sharing: %d", manifest.Statistics.ImagesWithSharing)

	assert.Greater(t, manifest.Statistics.TotalImages, 0, "should have at least 1 image")
	assert.Greater(t, len(manifest.Images), 0, "should have image entries")
}

// TestSingleImagePull tests pulling a single image as a sanity check.
func TestSingleImagePull(t *testing.T) {
	require.NotNil(t, testFramework, "test framework not initialized")
	require.NotNil(t, testFramework.Manifest(), "image manifest not loaded")
	require.Greater(t, len(testFramework.Manifest().Images), 0, "need at least 1 image")

	// Get first image
	image := testFramework.Manifest().Images[0]
	imageRef := testFramework.BuildPodImageRef(image)

	t.Logf("Testing single image pull: %s", imageRef)

	// Create work channels
	workChan := make(chan workload.WorkItem, 1)
	resultChan := make(chan workload.WorkResult, 1)

	// Create worker
	worker := workload.NewWorker(0, testFramework, workChan, resultChan)

	// Send work item
	workChan <- workload.WorkItem{
		Iteration: 0,
		Image:     image,
		ImageRef:  imageRef,
	}
	close(workChan)

	// Run worker
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	go worker.Run(ctx)

	// Get result
	result := <-resultChan

	t.Logf("Pull result:")
	t.Logf("  Success: %v", result.Success)
	t.Logf("  Duration: %.3fs", result.Duration)
	t.Logf("  Pull time: %.3fs", result.PullTime)
	if result.Error != "" {
		t.Logf("  Error: %s", result.Error)
	}

	assert.True(t, result.Success, "single image pull should succeed: %s", result.Error)
}
