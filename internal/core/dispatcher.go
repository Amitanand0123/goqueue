package core

import (
	"context"
	"log/slog"
	"time"

	"github.com/Amitanand0123/goqueue/internal/broker"
)

// Dispatcher pulls jobs from Redis queues and sends them to the worker pool.
type Dispatcher struct {
	broker  *broker.Broker
	jobCh   chan string
	queues  []string
	logger  *slog.Logger
	timeout time.Duration
}

// NewDispatcher creates a new dispatcher.
func NewDispatcher(b *broker.Broker, jobCh chan string, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		broker: b,
		jobCh:  jobCh,
		queues: []string{
			PriorityCritical.QueueName(),
			PriorityHigh.QueueName(),
			PriorityDefault.QueueName(),
			PriorityLow.QueueName(),
		},
		logger:  logger,
		timeout: 5 * time.Second,
	}
}

// Start begins the dispatcher loop in a goroutine.
func (d *Dispatcher) Start(ctx context.Context) {
	go d.run(ctx)
}

func (d *Dispatcher) run(ctx context.Context) {
	d.logger.Info("dispatcher started", "queues", d.queues)

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("dispatcher stopping")
			return
		default:
			queueName, jobID, err := d.broker.Dequeue(ctx, d.queues, d.timeout)
			if err != nil {
				if ctx.Err() != nil {
					return // context cancelled
				}
				d.logger.Error("dequeue error", "error", err)
				time.Sleep(1 * time.Second)
				continue
			}

			if jobID == "" {
				continue // timeout, no job available
			}

			d.logger.Debug("job dequeued", "id", jobID, "queue", queueName)

			select {
			case d.jobCh <- jobID:
			case <-ctx.Done():
				// Put job back if we can't send it to a worker
				d.broker.Enqueue(context.Background(), queueName, jobID)
				return
			}
		}
	}
}
