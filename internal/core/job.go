package core

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Priority represents the priority level of a job.
type Priority int

const (
	PriorityCritical Priority = 1
	PriorityHigh     Priority = 2
	PriorityDefault  Priority = 3
	PriorityLow      Priority = 4
)

func (p Priority) String() string {
	switch p {
	case PriorityCritical:
		return "critical"
	case PriorityHigh:
		return "high"
	case PriorityDefault:
		return "default"
	case PriorityLow:
		return "low"
	default:
		return "default"
	}
}

func ParsePriority(s string) Priority {
	switch s {
	case "critical":
		return PriorityCritical
	case "high":
		return PriorityHigh
	case "default", "":
		return PriorityDefault
	case "low":
		return PriorityLow
	default:
		return PriorityDefault
	}
}

// QueueName returns the Redis queue key for this priority.
func (p Priority) QueueName() string {
	return "goqueue:queue:" + p.String()
}

// Status represents the current state of a job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusScheduled Status = "scheduled"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusRetrying  Status = "retrying"
	StatusFailed    Status = "failed"
	StatusDead      Status = "dead"
	StatusCancelled Status = "cancelled"
)

// Job represents a unit of work to be executed.
type Job struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Priority    Priority        `json:"priority"`
	Status      Status          `json:"status"`
	Queue       string          `json:"queue"`
	MaxRetries  int             `json:"max_retries"`
	RetryCount  int             `json:"retry_count"`
	RetryDelay  time.Duration   `json:"retry_delay"`
	Timeout     time.Duration   `json:"timeout"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	ScheduledAt time.Time       `json:"scheduled_at,omitempty"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	WorkerID    string          `json:"worker_id,omitempty"`
}

// NewJob creates a new job with defaults.
func NewJob(jobType string, payload json.RawMessage, opts ...JobOption) *Job {
	j := &Job{
		ID:         uuid.New().String(),
		Type:       jobType,
		Payload:    payload,
		Priority:   PriorityDefault,
		Status:     StatusPending,
		MaxRetries: 3,
		RetryDelay: 10 * time.Second,
		Timeout:    30 * time.Second,
		CreatedAt:  time.Now().UTC(),
	}
	j.Queue = j.Priority.QueueName()

	for _, opt := range opts {
		opt(j)
	}
	// Update queue name if priority was changed by an option
	j.Queue = j.Priority.QueueName()

	return j
}

// JobOption configures a Job.
type JobOption func(*Job)

func WithPriority(p Priority) JobOption {
	return func(j *Job) {
		j.Priority = p
	}
}

func WithMaxRetries(n int) JobOption {
	return func(j *Job) {
		j.MaxRetries = n
	}
}

func WithTimeout(d time.Duration) JobOption {
	return func(j *Job) {
		j.Timeout = d
	}
}

func WithRetryDelay(d time.Duration) JobOption {
	return func(j *Job) {
		j.RetryDelay = d
	}
}

func WithDelay(d time.Duration) JobOption {
	return func(j *Job) {
		j.ScheduledAt = time.Now().UTC().Add(d)
		j.Status = StatusScheduled
	}
}

func WithScheduledAt(t time.Time) JobOption {
	return func(j *Job) {
		j.ScheduledAt = t.UTC()
		j.Status = StatusScheduled
	}
}

// ToMap serializes a Job to a map for Redis HSET.
func (j *Job) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"id":          j.ID,
		"type":        j.Type,
		"payload":     string(j.Payload),
		"priority":    int(j.Priority),
		"status":      string(j.Status),
		"queue":       j.Queue,
		"max_retries": j.MaxRetries,
		"retry_count": j.RetryCount,
		"retry_delay": j.RetryDelay.String(),
		"timeout":     j.Timeout.String(),
		"error":       j.Error,
		"created_at":  j.CreatedAt.Format(time.RFC3339Nano),
		"worker_id":   j.WorkerID,
	}
	if j.Result != nil {
		m["result"] = string(j.Result)
	}
	if !j.ScheduledAt.IsZero() {
		m["scheduled_at"] = j.ScheduledAt.Format(time.RFC3339Nano)
	}
	if j.StartedAt != nil {
		m["started_at"] = j.StartedAt.Format(time.RFC3339Nano)
	}
	if j.CompletedAt != nil {
		m["completed_at"] = j.CompletedAt.Format(time.RFC3339Nano)
	}
	return m
}

// JobFromMap deserializes a Job from a Redis hash map.
func JobFromMap(m map[string]string) (*Job, error) {
	j := &Job{}
	j.ID = m["id"]
	j.Type = m["type"]
	j.Payload = json.RawMessage(m["payload"])
	j.Status = Status(m["status"])
	j.Queue = m["queue"]
	j.Error = m["error"]
	j.WorkerID = m["worker_id"]

	if v, ok := m["priority"]; ok {
		p, _ := parseInt(v)
		j.Priority = Priority(p)
	}
	if v, ok := m["max_retries"]; ok {
		j.MaxRetries, _ = parseInt(v)
	}
	if v, ok := m["retry_count"]; ok {
		j.RetryCount, _ = parseInt(v)
	}
	if v, ok := m["retry_delay"]; ok {
		j.RetryDelay, _ = time.ParseDuration(v)
	}
	if v, ok := m["timeout"]; ok {
		j.Timeout, _ = time.ParseDuration(v)
	}
	if v, ok := m["result"]; ok && v != "" {
		j.Result = json.RawMessage(v)
	}
	if v, ok := m["created_at"]; ok {
		j.CreatedAt, _ = time.Parse(time.RFC3339Nano, v)
	}
	if v, ok := m["scheduled_at"]; ok && v != "" {
		j.ScheduledAt, _ = time.Parse(time.RFC3339Nano, v)
	}
	if v, ok := m["started_at"]; ok && v != "" {
		t, _ := time.Parse(time.RFC3339Nano, v)
		j.StartedAt = &t
	}
	if v, ok := m["completed_at"]; ok && v != "" {
		t, _ := time.Parse(time.RFC3339Nano, v)
		j.CompletedAt = &t
	}

	return j, nil
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			if c == '-' {
				continue
			}
			break
		}
		n = n*10 + int(c-'0')
	}
	if len(s) > 0 && s[0] == '-' {
		n = -n
	}
	return n, nil
}
