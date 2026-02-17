package core

import (
	"context"
	"log/slog"
	"time"

	"github.com/Amitanand0123/goqueue/internal/broker"
	"github.com/Amitanand0123/goqueue/internal/config"
	"github.com/Amitanand0123/goqueue/internal/tasks"
)

// Engine is the central orchestrator that ties together all core components.
type Engine struct {
	Broker     *broker.Broker
	Registry   *tasks.Registry
	Scheduler  *Scheduler
	Dispatcher *Dispatcher
	WorkerPool *WorkerPool
	DLQ        *DeadLetterManager
	Results    *ResultStore
	Config     *config.Config
	Logger     *slog.Logger
	jobCh      chan string
	cancelFn   context.CancelFunc
}

// NewEngine creates a new queue engine.
func NewEngine(cfg *config.Config, b *broker.Broker, logger *slog.Logger) *Engine {
	registry := tasks.NewRegistry()
	jobCh := make(chan string, cfg.WorkerConcurrency*2)

	return &Engine{
		Broker:     b,
		Registry:   registry,
		Scheduler:  NewScheduler(b, registry, logger),
		Dispatcher: NewDispatcher(b, jobCh, logger),
		WorkerPool: NewWorkerPool(b, registry, cfg.WorkerConcurrency, jobCh, logger),
		DLQ:        NewDeadLetterManager(b, logger),
		Results:    NewResultStore(b),
		Config:     cfg,
		Logger:     logger,
		jobCh:      jobCh,
	}
}

// Register registers a task handler.
func (e *Engine) Register(taskType string, handler tasks.HandlerFunc) {
	e.Registry.Register(taskType, handler)
}

// StartWorkers starts the dispatcher, worker pool, and scheduler poller.
func (e *Engine) StartWorkers(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	e.cancelFn = cancel

	e.Scheduler.StartPolling(ctx)
	e.Dispatcher.Start(ctx)
	e.WorkerPool.Start(ctx)

	e.Logger.Info("queue engine started",
		"concurrency", e.Config.WorkerConcurrency,
		"queues", []string{"critical", "high", "default", "low"},
	)
}

// Shutdown gracefully stops the engine.
func (e *Engine) Shutdown() {
	e.Logger.Info("shutting down queue engine...")

	// Stop scheduler
	e.Scheduler.Stop()

	// Cancel context (stops dispatcher and workers from picking new jobs)
	if e.cancelFn != nil {
		e.cancelFn()
	}

	// Wait for in-flight jobs with timeout
	done := make(chan struct{})
	go func() {
		e.WorkerPool.Wait()
		close(done)
	}()

	select {
	case <-done:
		e.Logger.Info("all workers stopped gracefully")
	case <-time.After(e.Config.ShutdownTimeout):
		e.Logger.Warn("shutdown timeout reached, some jobs may not have completed")
	}

	// Close broker connection
	e.Broker.Close()
}

// Stats returns current queue statistics.
func (e *Engine) Stats(ctx context.Context) (map[string]interface{}, error) {
	critLen, _ := e.Broker.QueueLen(ctx, PriorityCritical.QueueName())
	highLen, _ := e.Broker.QueueLen(ctx, PriorityHigh.QueueName())
	defaultLen, _ := e.Broker.QueueLen(ctx, PriorityDefault.QueueName())
	lowLen, _ := e.Broker.QueueLen(ctx, PriorityLow.QueueName())

	totalProcessed, _ := e.Broker.GetMetric(ctx, "processed")
	totalFailed, _ := e.Broker.GetMetric(ctx, "failed")
	deadCount, _ := e.DLQ.Count(ctx)
	activeWorkers, _ := e.Broker.ActiveWorkerCount(ctx)

	today := time.Now().UTC().Format("2006-01-02")
	processedToday, _ := e.Broker.GetDailyMetric(ctx, "processed", today)
	failedToday, _ := e.Broker.GetDailyMetric(ctx, "failed", today)

	return map[string]interface{}{
		"queues": map[string]int64{
			"critical": critLen,
			"high":     highLen,
			"default":  defaultLen,
			"low":      lowLen,
		},
		"total_processed": totalProcessed,
		"total_failed":    totalFailed,
		"dead_count":      deadCount,
		"active_workers":  activeWorkers,
		"jobs_today": map[string]int64{
			"processed": processedToday,
			"failed":    failedToday,
		},
	}, nil
}
