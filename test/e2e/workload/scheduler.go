package workload

import (
	"context"
	"sync"

	"github.com/pdylanross/barnacle/test/e2e/framework"
	"go.uber.org/zap"
)

// Scheduler distributes work across multiple workers.
type Scheduler struct {
	framework  *framework.Framework
	logger     *zap.Logger
	workChan   chan WorkItem
	resultChan chan WorkResult
	workers    []*Worker
}

const workerBufferMultiplier = 10

// NewScheduler creates a new scheduler.
func NewScheduler(fw *framework.Framework, logger *zap.Logger) *Scheduler {
	numWorkers := fw.Options().Workers
	bufferSize := numWorkers * workerBufferMultiplier

	return &Scheduler{
		framework:  fw,
		logger:     logger,
		workChan:   make(chan WorkItem, bufferSize),
		resultChan: make(chan WorkResult, bufferSize),
		workers:    make([]*Worker, numWorkers),
	}
}

// Run executes the workload and returns all results.
func (s *Scheduler) Run(ctx context.Context) ([]WorkResult, error) {
	opts := s.framework.Options()

	s.logger.Info("Starting workload scheduler",
		zap.Int("workers", opts.Workers),
		zap.Int("iterations", opts.Iterations),
	)

	// Start workers
	var wg sync.WaitGroup
	for i := range opts.Workers {
		s.workers[i] = NewWorker(i, s.framework, s.workChan, s.resultChan)
		wg.Add(1)
		go func(w *Worker) {
			defer wg.Done()
			w.Run(ctx)
		}(s.workers[i])
	}

	// Queue work items
	go func() {
		defer close(s.workChan)
		for i := range opts.Iterations {
			select {
			case <-ctx.Done():
				return
			default:
				image := s.framework.GetImageForIteration(i)
				imageRef := s.framework.BuildPodImageRef(image)

				item := WorkItem{
					Iteration: i,
					Image:     image,
					ImageRef:  imageRef,
				}

				select {
				case s.workChan <- item:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// Collect results
	results := make([]WorkResult, 0, opts.Iterations)
	resultsDone := make(chan struct{})

	go func() {
		defer close(resultsDone)
		for result := range s.resultChan {
			results = append(results, result)

			// Log progress periodically
			if len(results)%100 == 0 {
				s.logger.Info("Progress",
					zap.Int("completed", len(results)),
					zap.Int("total", opts.Iterations),
				)
			}
		}
	}()

	// Wait for workers to finish
	wg.Wait()
	close(s.resultChan)

	// Wait for results collection to finish
	<-resultsDone

	s.logger.Info("Workload completed",
		zap.Int("total_results", len(results)),
	)

	return results, nil
}

// RunWithProgress executes the workload and calls the progress callback periodically.
func (s *Scheduler) RunWithProgress(ctx context.Context, progressFn func(completed, total int)) ([]WorkResult, error) {
	opts := s.framework.Options()

	s.logger.Info("Starting workload scheduler with progress",
		zap.Int("workers", opts.Workers),
		zap.Int("iterations", opts.Iterations),
	)

	// Start workers
	var wg sync.WaitGroup
	for i := range opts.Workers {
		s.workers[i] = NewWorker(i, s.framework, s.workChan, s.resultChan)
		wg.Add(1)
		go func(w *Worker) {
			defer wg.Done()
			w.Run(ctx)
		}(s.workers[i])
	}

	// Queue work items
	go func() {
		defer close(s.workChan)
		for i := range opts.Iterations {
			select {
			case <-ctx.Done():
				return
			default:
				image := s.framework.GetImageForIteration(i)
				imageRef := s.framework.BuildPodImageRef(image)

				item := WorkItem{
					Iteration: i,
					Image:     image,
					ImageRef:  imageRef,
				}

				select {
				case s.workChan <- item:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// Collect results
	results := make([]WorkResult, 0, opts.Iterations)
	resultsDone := make(chan struct{})

	go func() {
		defer close(resultsDone)
		for result := range s.resultChan {
			results = append(results, result)

			if progressFn != nil {
				progressFn(len(results), opts.Iterations)
			}
		}
	}()

	// Wait for workers to finish
	wg.Wait()
	close(s.resultChan)

	// Wait for results collection to finish
	<-resultsDone

	s.logger.Info("Workload completed",
		zap.Int("total_results", len(results)),
	)

	return results, nil
}
