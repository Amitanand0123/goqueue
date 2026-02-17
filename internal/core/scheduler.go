package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Amitanand0123/goqueue/internal/broker"
	"github.com/Amitanand0123/goqueue/internal/tasks"
)

// Scheduler handles enqueuing jobs and polling for scheduled jobs.
type Scheduler struct {
	broker   *broker.Broker
	registry *tasks.Registry
	logger   *slog.Logger
	stopCh   chan struct{}
}

// NewScheduler creates a new scheduler.
func NewScheduler(b *broker.Broker, r *tasks.Registry, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		broker:   b,
		registry: r,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Enqueue validates and enqueues a job.
func (s *Scheduler) Enqueue(ctx context.Context, job *Job) error {
	// Validate task type is registered
	if !s.registry.Has(job.Type) {
		return fmt.Errorf("unknown task type: %s", job.Type)
	}

	// Save job data to Redis
	if err := s.broker.SaveJob(ctx, job.ID, job.ToMap()); err != nil {
		return fmt.Errorf("failed to save job: %w", err)
	}

	// If scheduled for the future, add to sorted set
	if !job.ScheduledAt.IsZero() && job.ScheduledAt.After(time.Now().UTC()) {
		if err := s.broker.ScheduleJob(ctx, job.ID, job.ScheduledAt); err != nil {
			return fmt.Errorf("failed to schedule job: %w", err)
		}
		if err := s.broker.AddToStatusSet(ctx, string(StatusScheduled), job.ID); err != nil {
			return fmt.Errorf("failed to update status set: %w", err)
		}
		s.publishEvent(ctx, "job:created", job)
		s.logger.Info("job scheduled", "id", job.ID, "type", job.Type, "scheduled_at", job.ScheduledAt)
		return nil
	}

	// Otherwise, enqueue immediately
	job.Status = StatusPending
	if err := s.broker.UpdateJobField(ctx, job.ID, "status", string(StatusPending)); err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}
	if err := s.broker.Enqueue(ctx, job.Queue, job.ID); err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}
	if err := s.broker.AddToStatusSet(ctx, string(StatusPending), job.ID); err != nil {
		return fmt.Errorf("failed to update status set: %w", err)
	}

	s.publishEvent(ctx, "job:created", job)
	s.logger.Info("job enqueued", "id", job.ID, "type", job.Type, "priority", job.Priority.String())

	return nil
}

// StartPolling starts the scheduled job poller in a goroutine.
func (s *Scheduler) StartPolling(ctx context.Context) {
	go s.poll(ctx)
}

func (s *Scheduler) poll(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processDueJobs(ctx)
		}
	}
}

func (s *Scheduler) processDueJobs(ctx context.Context) {
	jobs, err := s.broker.GetDueJobs(ctx, time.Now().UTC(), 100)
	if err != nil {
		s.logger.Error("failed to get due jobs", "error", err)
		return
	}

	for _, jobID := range jobs {
		// Remove from scheduled set
		if err := s.broker.RemoveScheduled(ctx, jobID); err != nil {
			s.logger.Error("failed to remove scheduled job", "id", jobID, "error", err)
			continue
		}

		// Load job data to get priority/queue
		jobData, err := s.broker.GetJob(ctx, jobID)
		if err != nil {
			s.logger.Error("failed to load scheduled job", "id", jobID, "error", err)
			continue
		}

		job, err := JobFromMap(jobData)
		if err != nil {
			s.logger.Error("failed to parse scheduled job", "id", jobID, "error", err)
			continue
		}

		// Move to the queue
		if err := s.broker.Enqueue(ctx, job.Queue, jobID); err != nil {
			s.logger.Error("failed to enqueue scheduled job", "id", jobID, "error", err)
			continue
		}

		// Update status
		oldStatus := string(job.Status)
		s.broker.UpdateJobField(ctx, jobID, "status", string(StatusPending))
		s.broker.MoveStatus(ctx, jobID, oldStatus, string(StatusPending))

		s.publishEvent(ctx, "job:scheduled_enqueue", job)
		s.logger.Info("scheduled job enqueued", "id", jobID, "type", job.Type)
	}
}

// Stop stops the scheduler poller.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

func (s *Scheduler) publishEvent(ctx context.Context, event string, job *Job) {
	data, _ := json.Marshal(map[string]interface{}{
		"event":    event,
		"job_id":   job.ID,
		"type":     job.Type,
		"priority": job.Priority.String(),
		"status":   string(job.Status),
	})
	s.broker.Publish(ctx, string(data))
}
