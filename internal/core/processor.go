package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"

	"github.com/Amitanand0123/goqueue/internal/broker"
	"github.com/Amitanand0123/goqueue/internal/tasks"
)

// Processor handles execution of a single job, including retries and DLQ.
type Processor struct {
	broker   *broker.Broker
	registry *tasks.Registry
	logger   *slog.Logger
}

// NewProcessor creates a new processor.
func NewProcessor(b *broker.Broker, r *tasks.Registry, logger *slog.Logger) *Processor {
	return &Processor{
		broker:   b,
		registry: r,
		logger:   logger,
	}
}

// Process loads, executes, and handles the result of a job.
func (p *Processor) Process(ctx context.Context, jobID, workerID string) error {
	// Load job from Redis
	jobData, err := p.broker.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to load job %s: %w", jobID, err)
	}

	job, err := JobFromMap(jobData)
	if err != nil {
		return fmt.Errorf("failed to parse job %s: %w", jobID, err)
	}

	// Check if job was cancelled
	if job.Status == StatusCancelled {
		p.logger.Info("skipping cancelled job", "id", jobID)
		return nil
	}

	// Update status to running
	now := time.Now().UTC()
	job.StartedAt = &now
	job.WorkerID = workerID
	job.Status = StatusRunning

	p.broker.UpdateJobFields(ctx, jobID, map[string]interface{}{
		"status":     string(StatusRunning),
		"started_at": now.Format(time.RFC3339Nano),
		"worker_id":  workerID,
	})

	oldStatus := string(jobData["status"])
	p.broker.MoveStatus(ctx, jobID, oldStatus, string(StatusRunning))

	p.publishEvent(ctx, "job:started", map[string]interface{}{
		"job_id":    jobID,
		"worker_id": workerID,
		"type":      job.Type,
	})

	// Look up handler
	handler, err := p.registry.Get(job.Type)
	if err != nil {
		p.handleFailure(ctx, job, fmt.Errorf("handler not found: %w", err))
		return nil
	}

	// Execute with timeout
	execCtx, cancel := context.WithTimeout(ctx, job.Timeout)
	defer cancel()

	startTime := time.Now()
	result, execErr := handler(execCtx, job.Payload)
	duration := time.Since(startTime)

	if execErr != nil {
		p.logger.Warn("job failed", "id", jobID, "type", job.Type, "error", execErr, "duration", duration)
		p.handleFailure(ctx, job, execErr)
		return nil
	}

	// Success
	completedAt := time.Now().UTC()
	job.CompletedAt = &completedAt
	job.Status = StatusCompleted
	job.Result = result

	fields := map[string]interface{}{
		"status":       string(StatusCompleted),
		"completed_at": completedAt.Format(time.RFC3339Nano),
	}
	if result != nil {
		fields["result"] = string(result)
	}
	p.broker.UpdateJobFields(ctx, jobID, fields)
	p.broker.MoveStatus(ctx, jobID, string(StatusRunning), string(StatusCompleted))
	p.broker.IncrMetric(ctx, "processed")

	p.publishEvent(ctx, "job:completed", map[string]interface{}{
		"job_id":      jobID,
		"type":        job.Type,
		"duration_ms": duration.Milliseconds(),
	})

	p.logger.Info("job completed", "id", jobID, "type", job.Type, "duration", duration)
	return nil
}

func (p *Processor) handleFailure(ctx context.Context, job *Job, jobErr error) {
	job.Error = jobErr.Error()
	job.RetryCount++

	if job.RetryCount <= job.MaxRetries {
		// Retry with exponential backoff + jitter
		delay := p.calculateBackoff(job.RetryDelay, job.RetryCount)
		executeAt := time.Now().UTC().Add(delay)

		p.broker.UpdateJobFields(ctx, job.ID, map[string]interface{}{
			"status":      string(StatusRetrying),
			"error":       job.Error,
			"retry_count": job.RetryCount,
		})
		p.broker.MoveStatus(ctx, job.ID, string(StatusRunning), string(StatusRetrying))
		p.broker.ScheduleJob(ctx, job.ID, executeAt)

		p.publishEvent(ctx, "job:failed", map[string]interface{}{
			"job_id":      job.ID,
			"type":        job.Type,
			"error":       job.Error,
			"retry_count": job.RetryCount,
			"next_retry":  executeAt.Format(time.RFC3339),
		})

		p.logger.Info("job scheduled for retry",
			"id", job.ID,
			"type", job.Type,
			"retry", job.RetryCount,
			"max", job.MaxRetries,
			"delay", delay,
		)
	} else {
		// Max retries exhausted — move to DLQ
		p.broker.UpdateJobFields(ctx, job.ID, map[string]interface{}{
			"status":      string(StatusDead),
			"error":       job.Error,
			"retry_count": job.RetryCount,
		})
		p.broker.MoveStatus(ctx, job.ID, string(StatusRunning), string(StatusDead))
		p.broker.PushToDLQ(ctx, job.ID)
		p.broker.IncrMetric(ctx, "failed")

		p.publishEvent(ctx, "job:dead", map[string]interface{}{
			"job_id": job.ID,
			"type":   job.Type,
			"error":  "max retries exceeded: " + job.Error,
		})

		p.logger.Warn("job moved to DLQ", "id", job.ID, "type", job.Type, "retries", job.RetryCount)
	}
}

// calculateBackoff returns exponential backoff with jitter.
func (p *Processor) calculateBackoff(baseDelay time.Duration, attempt int) time.Duration {
	backoff := float64(baseDelay) * math.Pow(2, float64(attempt-1))
	// Add jitter: ±25%
	jitter := backoff * 0.25 * (rand.Float64()*2 - 1)
	d := time.Duration(backoff + jitter)
	// Cap at 1 hour
	if d > time.Hour {
		d = time.Hour
	}
	return d
}

func (p *Processor) publishEvent(ctx context.Context, event string, data map[string]interface{}) {
	data["event"] = event
	payload, _ := json.Marshal(data)
	p.broker.Publish(ctx, string(payload))
}
