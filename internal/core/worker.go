package core

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Amitanand0123/goqueue/internal/broker"
	"github.com/Amitanand0123/goqueue/internal/tasks"
)

// WorkerPool manages a pool of worker goroutines.
type WorkerPool struct {
	broker      *broker.Broker
	processor   *Processor
	concurrency int
	jobCh       chan string
	logger      *slog.Logger
	wg          sync.WaitGroup
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(b *broker.Broker, registry *tasks.Registry, concurrency int, jobCh chan string, logger *slog.Logger) *WorkerPool {
	return &WorkerPool{
		broker:      b,
		processor:   NewProcessor(b, registry, logger),
		concurrency: concurrency,
		jobCh:       jobCh,
		logger:      logger,
	}
}

// Start launches all worker goroutines.
func (wp *WorkerPool) Start(ctx context.Context) {
	wp.logger.Info("starting worker pool", "concurrency", wp.concurrency)

	for i := 0; i < wp.concurrency; i++ {
		workerID := fmt.Sprintf("worker-%d", i+1)
		wp.wg.Add(1)
		go wp.runWorker(ctx, workerID)
	}
}

// Wait blocks until all workers have finished.
func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

func (wp *WorkerPool) runWorker(ctx context.Context, workerID string) {
	defer wp.wg.Done()

	wp.logger.Info("worker started", "id", workerID)

	// Register worker
	wp.broker.RegisterWorker(ctx, workerID)

	// Start heartbeat
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go wp.heartbeat(heartbeatCtx, workerID)

	for {
		select {
		case <-ctx.Done():
			wp.logger.Info("worker stopping", "id", workerID)
			wp.broker.UnregisterWorker(context.Background(), workerID)
			return
		case jobID, ok := <-wp.jobCh:
			if !ok {
				wp.logger.Info("job channel closed, worker stopping", "id", workerID)
				wp.broker.UnregisterWorker(context.Background(), workerID)
				return
			}

			if err := wp.processor.Process(ctx, jobID, workerID); err != nil {
				wp.logger.Error("job processing error", "id", jobID, "worker", workerID, "error", err)
			}
		}
	}
}

func (wp *WorkerPool) heartbeat(ctx context.Context, workerID string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			wp.broker.UpdateHeartbeat(ctx, workerID)
		}
	}
}
