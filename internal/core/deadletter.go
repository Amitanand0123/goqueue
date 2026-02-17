package core

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Amitanand0123/goqueue/internal/broker"
)

// DeadLetterManager handles dead letter queue operations.
type DeadLetterManager struct {
	broker *broker.Broker
	logger *slog.Logger
}

// NewDeadLetterManager creates a new DLQ manager.
func NewDeadLetterManager(b *broker.Broker, logger *slog.Logger) *DeadLetterManager {
	return &DeadLetterManager{broker: b, logger: logger}
}

// List returns all jobs in the dead letter queue.
func (d *DeadLetterManager) List(ctx context.Context, start, stop int64) ([]*Job, error) {
	ids, err := d.broker.GetDLQJobs(ctx, start, stop)
	if err != nil {
		return nil, err
	}

	jobs := make([]*Job, 0, len(ids))
	for _, id := range ids {
		jobData, err := d.broker.GetJob(ctx, id)
		if err != nil {
			d.logger.Warn("failed to load dead job", "id", id, "error", err)
			continue
		}
		job, err := JobFromMap(jobData)
		if err != nil {
			d.logger.Warn("failed to parse dead job", "id", id, "error", err)
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// Count returns the number of jobs in the DLQ.
func (d *DeadLetterManager) Count(ctx context.Context) (int64, error) {
	return d.broker.DLQLen(ctx)
}

// RetryJob removes a job from the DLQ and re-enqueues it.
func (d *DeadLetterManager) RetryJob(ctx context.Context, jobID string) error {
	// Load job data
	jobData, err := d.broker.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	job, err := JobFromMap(jobData)
	if err != nil {
		return fmt.Errorf("failed to parse job: %w", err)
	}

	// Remove from DLQ
	if err := d.broker.RemoveFromDLQ(ctx, jobID); err != nil {
		return fmt.Errorf("failed to remove from DLQ: %w", err)
	}

	// Reset job state
	d.broker.UpdateJobFields(ctx, jobID, map[string]interface{}{
		"status":      string(StatusPending),
		"retry_count": 0,
		"error":       "",
		"worker_id":   "",
	})
	d.broker.MoveStatus(ctx, jobID, string(StatusDead), string(StatusPending))

	// Re-enqueue
	if err := d.broker.Enqueue(ctx, job.Queue, jobID); err != nil {
		return fmt.Errorf("failed to re-enqueue job: %w", err)
	}

	d.logger.Info("dead job retried", "id", jobID, "type", job.Type)
	return nil
}

// RetryAll retries all jobs in the DLQ.
func (d *DeadLetterManager) RetryAll(ctx context.Context) (int, error) {
	ids, err := d.broker.GetDLQJobs(ctx, 0, -1)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, id := range ids {
		if err := d.RetryJob(ctx, id); err != nil {
			d.logger.Warn("failed to retry dead job", "id", id, "error", err)
			continue
		}
		count++
	}
	return count, nil
}

// Purge removes all jobs from the DLQ.
func (d *DeadLetterManager) Purge(ctx context.Context) (int64, error) {
	// Get all DLQ job IDs to clean up status sets
	ids, err := d.broker.GetDLQJobs(ctx, 0, -1)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		d.broker.RemoveFromStatusSet(ctx, string(StatusDead), id)
	}

	return d.broker.PurgeDLQ(ctx)
}

// DeleteJob removes a single job from the DLQ and deletes its data.
func (d *DeadLetterManager) DeleteJob(ctx context.Context, jobID string) error {
	if err := d.broker.RemoveFromDLQ(ctx, jobID); err != nil {
		return err
	}
	d.broker.RemoveFromStatusSet(ctx, string(StatusDead), jobID)
	return d.broker.DeleteJob(ctx, jobID)
}
