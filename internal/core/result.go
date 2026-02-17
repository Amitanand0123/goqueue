package core

import (
	"context"
	"fmt"

	"github.com/Amitanand0123/goqueue/internal/broker"
)

// ResultStore handles job result retrieval.
type ResultStore struct {
	broker *broker.Broker
}

// NewResultStore creates a new result store.
func NewResultStore(b *broker.Broker) *ResultStore {
	return &ResultStore{broker: b}
}

// GetResult retrieves the result of a completed job.
func (r *ResultStore) GetResult(ctx context.Context, jobID string) (*Job, error) {
	jobData, err := r.broker.GetJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("job not found: %w", err)
	}

	job, err := JobFromMap(jobData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job: %w", err)
	}

	return job, nil
}

// ListByStatus returns all jobs with a given status.
func (r *ResultStore) ListByStatus(ctx context.Context, status Status) ([]*Job, error) {
	ids, err := r.broker.GetStatusMembers(ctx, string(status))
	if err != nil {
		return nil, err
	}

	jobs := make([]*Job, 0, len(ids))
	for _, id := range ids {
		jobData, err := r.broker.GetJob(ctx, id)
		if err != nil {
			continue
		}
		job, err := JobFromMap(jobData)
		if err != nil {
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// CancelJob cancels a pending or scheduled job.
func (r *ResultStore) CancelJob(ctx context.Context, jobID string) error {
	jobData, err := r.broker.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	job, err := JobFromMap(jobData)
	if err != nil {
		return fmt.Errorf("failed to parse job: %w", err)
	}

	// Only cancel if not already completed/dead
	switch job.Status {
	case StatusCompleted, StatusDead:
		return fmt.Errorf("cannot cancel job with status: %s", job.Status)
	}

	oldStatus := string(job.Status)
	r.broker.UpdateJobField(ctx, jobID, "status", string(StatusCancelled))
	r.broker.MoveStatus(ctx, jobID, oldStatus, string(StatusCancelled))

	// Remove from scheduled set if applicable
	if job.Status == StatusScheduled {
		r.broker.RemoveScheduled(ctx, jobID)
	}

	return nil
}
