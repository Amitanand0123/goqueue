# GoQueue — Distributed Task Queue System

A distributed task queue built in Go with Redis as the message broker. Features priority queues, retry logic with exponential backoff, dead letter queue, real-time status updates via WebSocket, cron-like scheduling, and a server-rendered dashboard.

---

## 1. System Overview

```
                         ┌─────────────────────────────────┐
                         │         GoQueue Server           │
                         │                                  │
  HTTP API ─────────────▶│  [Gin Router]                    │
  (submit jobs,          │     │                            │
   query status)         │     ▼                            │
                         │  [Job Scheduler]                 │
                         │     │                            │
                         │     ▼                            │
                         │  [Redis Queues]                  │
  Dashboard ────────────▶│     │         ┌───────────────┐  │
  (Go templates +        │     ▼         │  Worker Pool   │  │
   WebSocket)            │  [Dispatcher]──▶  goroutine 1  │  │
                         │               │  goroutine 2  │  │
                         │               │  goroutine N  │  │
                         │               └───────┬───────┘  │
                         │                       │          │
                         │                       ▼          │
                         │              [Result Store]      │
                         │              [Dead Letter Q]     │
                         └─────────────────────────────────┘
```

### How it works

1. **Client** submits a job via REST API (or schedules it via cron expression)
2. **Scheduler** places the job into the appropriate Redis priority queue
3. **Dispatcher** pulls jobs from queues (highest priority first) and assigns to workers
4. **Worker goroutines** execute the job's handler function
5. **On success**: result stored, status updated, webhook fired (optional)
6. **On failure**: retry with exponential backoff, or move to dead letter queue after max retries
7. **Dashboard** shows real-time status via WebSocket

---

## 2. Core Concepts

### Job
A unit of work to be executed. Has a type, payload, priority, and lifecycle.

```
Job Lifecycle:
  PENDING → SCHEDULED → RUNNING → COMPLETED
                           │
                           ├──→ RETRYING → RUNNING (retry loop)
                           │
                           └──→ FAILED → DEAD (after max retries)
```

### Queue
A named Redis list that holds jobs. Each priority level has its own queue.

```
Queues:
  goqueue:queue:critical    (priority 1 — processed first)
  goqueue:queue:high        (priority 2)
  goqueue:queue:default     (priority 3)
  goqueue:queue:low         (priority 4)
```

### Worker
A goroutine that picks up jobs and executes registered handler functions.

### Dead Letter Queue (DLQ)
Jobs that have exhausted all retry attempts land here. Can be inspected, retried manually, or purged from the dashboard.

---

## 3. Tech Stack

| Component | Technology | Purpose |
|---|---|---|
| Language | Go 1.24 | Core application |
| HTTP Framework | Gin | REST API + dashboard serving |
| Message Broker | Redis | Job queues, scheduling, pub/sub |
| Dashboard | Go html/template + HTMX | Server-rendered UI with dynamic updates |
| Real-time | WebSocket (gorilla/websocket) | Live job status in dashboard |
| Styling | Tailwind CSS (CDN) | Dashboard styling |
| Charts | Chart.js (CDN) | Dashboard metrics |
| CLI | cobra | Command-line interface |
| Config | Environment variables | Configuration |
| Logging | slog (stdlib) | Structured logging |

### Why HTMX?
- No build step, no Node.js, no npm — pure Go project
- Server sends HTML fragments, HTMX swaps them in the DOM
- Feels like a SPA but rendered on the server
- Perfect for dashboards: tables that auto-refresh, forms that submit without page reload

---

## 4. Project Structure

```
goqueue/
├── cmd/
│   └── goqueue/
│       └── main.go                 # CLI entry point (server, worker, both)
├── internal/
│   ├── config/
│   │   └── config.go               # Environment-based configuration
│   ├── core/
│   │   ├── job.go                  # Job struct, states, options
│   │   ├── queue.go                # Queue interface and Redis implementation
│   │   ├── scheduler.go            # Enqueue, delay, cron scheduling
│   │   ├── dispatcher.go           # Pulls jobs from queues, assigns to workers
│   │   ├── worker.go               # Worker pool management
│   │   ├── processor.go            # Job execution + retry logic
│   │   ├── deadletter.go           # Dead letter queue operations
│   │   └── result.go               # Job result storage
│   ├── broker/
│   │   └── redis.go                # Redis connection, queue operations, pub/sub
│   ├── handlers/
│   │   ├── api.go                  # REST API handlers (submit, status, cancel, retry)
│   │   ├── dashboard.go            # Dashboard page handlers
│   │   └── ws.go                   # WebSocket handler for real-time updates
│   ├── middleware/
│   │   ├── auth.go                 # API key authentication
│   │   ├── logger.go               # Request logging
│   │   └── ratelimit.go            # Rate limiting
│   ├── routes/
│   │   └── routes.go               # Route definitions
│   └── tasks/
│       └── registry.go             # Task type → handler function registry
├── web/
│   ├── templates/
│   │   ├── layouts/
│   │   │   └── base.html           # Base layout (nav, sidebar, scripts)
│   │   ├── pages/
│   │   │   ├── dashboard.html      # Overview: stats, charts, recent jobs
│   │   │   ├── queues.html         # Queue list with job counts per priority
│   │   │   ├── jobs.html           # Job list with filters (status, type, date)
│   │   │   ├── job_detail.html     # Single job: payload, logs, retries, result
│   │   │   ├── dead.html           # Dead letter queue: inspect, retry, purge
│   │   │   ├── scheduled.html      # Scheduled/cron jobs
│   │   │   └── workers.html        # Active workers, throughput
│   │   └── partials/
│   │       ├── job_row.html        # Single job table row (for HTMX swap)
│   │       ├── stats_cards.html    # Stats cards partial
│   │       └── toast.html          # Toast notification partial
│   └── static/
│       ├── css/
│       │   └── app.css             # Custom styles (minimal, mostly Tailwind)
│       └── js/
│           └── app.js              # WebSocket client, Chart.js init
├── examples/
│   ├── email_task.go               # Example: email sending task
│   ├── image_resize_task.go        # Example: image processing task
│   └── webhook_task.go             # Example: webhook delivery task
├── go.mod
├── go.sum
├── Dockerfile
├── docker-compose.yml              # Redis + GoQueue for local dev
├── .env.example
├── .gitignore
└── README.md
```

---

## 5. Data Models

### Job

```go
type Job struct {
    ID          string            `json:"id"`           // UUID v4
    Type        string            `json:"type"`         // e.g., "email:send", "image:resize"
    Payload     json.RawMessage   `json:"payload"`      // Arbitrary JSON payload
    Priority    Priority          `json:"priority"`     // critical, high, default, low
    Status      Status            `json:"status"`       // pending, scheduled, running, completed, failed, dead
    Queue       string            `json:"queue"`        // Queue name
    MaxRetries  int               `json:"max_retries"`  // Max retry attempts (default: 3)
    RetryCount  int               `json:"retry_count"`  // Current retry count
    RetryDelay  time.Duration     `json:"retry_delay"`  // Base delay between retries
    Timeout     time.Duration     `json:"timeout"`      // Max execution time per attempt
    Result      json.RawMessage   `json:"result"`       // Result data on completion
    Error       string            `json:"error"`        // Last error message
    CreatedAt   time.Time         `json:"created_at"`
    ScheduledAt time.Time         `json:"scheduled_at"` // When to execute (for delayed jobs)
    StartedAt   *time.Time        `json:"started_at"`
    CompletedAt *time.Time        `json:"completed_at"`
    WorkerID    string            `json:"worker_id"`    // Which worker is processing
}
```

### Priority Levels

```go
type Priority int

const (
    PriorityCritical Priority = 1  // Processed first, e.g., payment processing
    PriorityHigh     Priority = 2  // Important but not urgent, e.g., email delivery
    PriorityDefault  Priority = 3  // Normal tasks
    PriorityLow      Priority = 4  // Background tasks, e.g., cleanup, analytics
)
```

### Job Status Flow

```go
type Status string

const (
    StatusPending   Status = "pending"    // Enqueued, waiting to be picked up
    StatusScheduled Status = "scheduled"  // Delayed or cron-scheduled, not yet in queue
    StatusRunning   Status = "running"    // Being processed by a worker
    StatusCompleted Status = "completed"  // Successfully completed
    StatusRetrying  Status = "retrying"   // Failed, waiting for retry
    StatusFailed    Status = "failed"     // Failed after all retries
    StatusDead      Status = "dead"       // Moved to dead letter queue
    StatusCancelled Status = "cancelled"  // Manually cancelled
)
```

---

## 6. Redis Key Design

All keys are prefixed with `goqueue:` to avoid collisions.

```
# Queues (Redis Lists — LPUSH/BRPOP for FIFO)
goqueue:queue:critical              # Priority 1 queue
goqueue:queue:high                  # Priority 2 queue
goqueue:queue:default               # Priority 3 queue
goqueue:queue:low                   # Priority 4 queue

# Job data (Redis Hashes)
goqueue:job:{job_id}                # Full job data as hash fields

# Job status index (Redis Sets — for filtering)
goqueue:status:pending              # Set of job IDs with pending status
goqueue:status:running              # Set of job IDs with running status
goqueue:status:completed            # Set of job IDs with completed status
goqueue:status:failed               # Set of job IDs with failed status
goqueue:status:dead                 # Set of job IDs in DLQ

# Scheduled jobs (Redis Sorted Set — score = unix timestamp)
goqueue:scheduled                   # Jobs to be enqueued at a future time

# Dead letter queue (Redis List)
goqueue:dead                        # Failed jobs after max retries

# Metrics (Redis Hashes)
goqueue:metrics:processed           # Counter: total jobs processed
goqueue:metrics:failed              # Counter: total jobs failed
goqueue:metrics:processed:{date}    # Daily processed count (e.g., 2026-02-17)
goqueue:metrics:failed:{date}       # Daily failed count

# Worker registry (Redis Hash — field: worker_id, value: heartbeat timestamp)
goqueue:workers                     # Active workers with last heartbeat

# Pub/Sub channel
goqueue:events                      # Real-time job events for WebSocket
```

---

## 7. Core Components

### 7.1 Scheduler (Enqueue Logic)

```
EnqueueJob(job):
  1. Assign UUID if not set
  2. Validate job type is registered
  3. Set defaults (retries=3, timeout=30s, priority=default)
  4. If ScheduledAt is in the future:
       → ZADD goqueue:scheduled {timestamp} {job_id}
       → SET status = "scheduled"
  5. Else:
       → LPUSH goqueue:queue:{priority} {job_id}
       → SET status = "pending"
  6. HSET goqueue:job:{job_id} {all fields}
  7. SADD goqueue:status:{status} {job_id}
  8. PUBLISH goqueue:events {job_created event}
```

**Scheduled Job Poller** (runs every 1s):
```
Every 1 second:
  1. ZRANGEBYSCORE goqueue:scheduled -inf {now} LIMIT 0 100
  2. For each job:
       → ZREM goqueue:scheduled {job_id}
       → LPUSH goqueue:queue:{priority} {job_id}
       → Update status: scheduled → pending
       → PUBLISH event
```

### 7.2 Dispatcher (Job Assignment)

```
Dispatcher loop (runs continuously):
  1. BRPOP goqueue:queue:critical goqueue:queue:high
         goqueue:queue:default goqueue:queue:low
         TIMEOUT 5s
     (BRPOP checks queues left-to-right, so critical is always first)
  2. If job received:
       → Send job_id to worker channel
  3. If timeout:
       → Continue loop (check for shutdown signal)
```

### 7.3 Worker Pool

```
WorkerPool:
  - N worker goroutines (configurable, default: 10)
  - Each worker reads from a shared job channel
  - Heartbeat: every 5s, update goqueue:workers with timestamp

Worker loop:
  1. Receive job_id from channel
  2. HGETALL goqueue:job:{job_id} → load job data
  3. Update status: pending → running, set started_at, worker_id
  4. Look up handler from task registry by job.Type
  5. Execute handler with context (has timeout)
  6. On success:
       → Update status: running → completed
       → Store result
       → INCR goqueue:metrics:processed
       → PUBLISH event
  7. On failure:
       → If retry_count < max_retries:
           → Increment retry_count
           → Calculate delay: base_delay * 2^retry_count (exponential backoff)
           → ZADD goqueue:scheduled {now + delay} {job_id}
           → Update status: running → retrying
       → Else:
           → LPUSH goqueue:dead {job_id}
           → Update status: running → dead
           → INCR goqueue:metrics:failed
       → PUBLISH event
```

### 7.4 Task Registry

```go
// Users register task handlers like this:
queue.Register("email:send", func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
    var p EmailPayload
    json.Unmarshal(payload, &p)

    err := sendEmail(p.To, p.Subject, p.Body)
    if err != nil {
        return nil, err  // Will trigger retry
    }

    return json.Marshal(map[string]string{"status": "sent"})
})

queue.Register("image:resize", func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
    // ... process image
})
```

---

## 8. REST API

### Submit a Job

```
POST /api/v1/jobs

Request:
{
  "type": "email:send",
  "payload": {
    "to": "user@example.com",
    "subject": "Welcome!",
    "body": "Hello from GoQueue"
  },
  "priority": "high",
  "max_retries": 5,
  "timeout": "30s",
  "delay": "5m"           // Optional: delay execution by 5 minutes
}

Response: 201 Created
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "email:send",
  "status": "pending",
  "priority": "high",
  "created_at": "2026-02-17T10:00:00Z"
}
```

### Get Job Status

```
GET /api/v1/jobs/{id}

Response: 200 OK
{
  "id": "550e8400-...",
  "type": "email:send",
  "status": "completed",
  "priority": "high",
  "retry_count": 0,
  "result": {"status": "sent"},
  "created_at": "2026-02-17T10:00:00Z",
  "started_at": "2026-02-17T10:00:01Z",
  "completed_at": "2026-02-17T10:00:02Z",
  "worker_id": "worker-3"
}
```

### List Jobs

```
GET /api/v1/jobs?status=running&type=email:send&page=1&limit=20

Response: 200 OK
{
  "jobs": [...],
  "total": 150,
  "page": 1,
  "limit": 20
}
```

### Cancel a Job

```
DELETE /api/v1/jobs/{id}

Response: 200 OK
{"message": "Job cancelled"}
```

### Retry a Dead Job

```
POST /api/v1/jobs/{id}/retry

Response: 200 OK
{"message": "Job re-enqueued", "status": "pending"}
```

### Purge Dead Letter Queue

```
DELETE /api/v1/dead

Response: 200 OK
{"message": "Purged 42 dead jobs"}
```

### Queue Stats

```
GET /api/v1/stats

Response: 200 OK
{
  "queues": {
    "critical": 5,
    "high": 23,
    "default": 142,
    "low": 8
  },
  "total_processed": 15420,
  "total_failed": 87,
  "dead_count": 12,
  "active_workers": 10,
  "jobs_today": {
    "processed": 320,
    "failed": 3
  }
}
```

---

## 9. Dashboard Pages

Built with Go `html/template` + HTMX + Tailwind CSS + Chart.js.

Dark theme with teal accents (same aesthetic as GoURL).

### Page: Overview Dashboard (`/dashboard`)
- **Stats cards**: Total processed, failed, success rate, active workers
- **Line chart**: Jobs processed per hour (last 24h)
- **Bar chart**: Jobs per queue (critical, high, default, low)
- **Recent jobs table**: Last 10 jobs with status badges (auto-refreshes via HTMX polling every 3s)

### Page: Queues (`/dashboard/queues`)
- **Queue cards**: Each priority queue with pending count, processing rate
- **Pause/resume** buttons per queue

### Page: Jobs (`/dashboard/jobs`)
- **Filterable table**: Filter by status, type, priority, date range
- **Columns**: ID (truncated), Type, Priority, Status (colored badge), Created, Duration
- **Click row** → job detail page
- **Bulk actions**: retry selected, cancel selected

### Page: Job Detail (`/dashboard/jobs/{id}`)
- **Job info card**: All fields, formatted payload JSON
- **Timeline**: Created → Scheduled → Running → Completed/Failed (visual step indicator)
- **Retry history**: Table of each attempt with timestamp, error, duration
- **Actions**: Retry, Cancel, Delete
- **Live status**: WebSocket updates the status badge in real-time

### Page: Dead Letter Queue (`/dashboard/dead`)
- **Table of dead jobs**: Type, error message, retry count, failed at
- **Actions per job**: Retry, Delete
- **Bulk actions**: Retry all, Purge all

### Page: Workers (`/dashboard/workers`)
- **Active workers table**: Worker ID, status, current job, last heartbeat
- **Throughput**: Jobs/minute per worker

---

## 10. WebSocket Events

The server publishes job events to a Redis pub/sub channel. The WebSocket handler subscribes and pushes events to connected dashboard clients.

```json
// Job created
{"event": "job:created", "job_id": "...", "type": "email:send", "priority": "high"}

// Job started
{"event": "job:started", "job_id": "...", "worker_id": "worker-3"}

// Job completed
{"event": "job:completed", "job_id": "...", "duration_ms": 1250}

// Job failed
{"event": "job:failed", "job_id": "...", "error": "connection timeout", "retry_count": 2}

// Job dead
{"event": "job:dead", "job_id": "...", "error": "max retries exceeded"}
```

---

## 11. CLI Interface

The CLI uses Cobra for subcommands. The binary can run as server, worker, or both.

```bash
# Start everything (API server + workers + dashboard)
goqueue start

# Start only the API server + dashboard (no workers)
goqueue server

# Start only workers (connects to same Redis, processes jobs)
goqueue worker --concurrency 20

# Submit a job from CLI
goqueue enqueue --type "email:send" --payload '{"to":"a@b.com"}' --priority high

# Check job status
goqueue status <job-id>

# List jobs
goqueue list --status running --limit 10

# View stats
goqueue stats

# Retry all dead jobs
goqueue retry-dead

# Purge dead letter queue
goqueue purge-dead
```

---

## 12. Configuration

```env
# Server
PORT=8080
ENVIRONMENT=development          # development | production
API_KEY=your-secret-api-key      # For API authentication

# Redis
REDIS_URL=localhost:6379         # or rediss://... for Upstash TLS
REDIS_PASSWORD=
REDIS_DB=0

# Workers
WORKER_CONCURRENCY=10           # Number of worker goroutines
WORKER_SHUTDOWN_TIMEOUT=30s     # Grace period for in-flight jobs on shutdown

# Job defaults
JOB_DEFAULT_TIMEOUT=30s         # Default per-job timeout
JOB_DEFAULT_MAX_RETRIES=3       # Default max retries
JOB_DEFAULT_RETRY_DELAY=10s     # Base delay for exponential backoff

# Dashboard
DASHBOARD_ENABLED=true          # Enable/disable dashboard
DASHBOARD_USERNAME=admin        # Dashboard basic auth
DASHBOARD_PASSWORD=admin

# Metrics
METRICS_RETENTION_DAYS=30       # How long to keep daily metrics
```

---

## 13. Implementation Phases

### Phase 1 — Foundation (Core Queue Engine)
- [ ] Project setup: Go module, folder structure, config loading
- [ ] Redis connection with Upstash TLS support
- [ ] Job model with all states and serialization
- [ ] Task registry (register handler functions by type)
- [ ] Redis queue operations: enqueue, dequeue (LPUSH/BRPOP)
- [ ] Priority queue support (4 levels, BRPOP across all)
- [ ] Worker pool with configurable concurrency
- [ ] Dispatcher loop: pull from queues, send to workers
- [ ] Job execution with context timeout
- [ ] Job status tracking in Redis (hash + sets)
- [ ] Basic structured logging with slog

### Phase 2 — Reliability (Retries, DLQ, Scheduling)
- [ ] Retry logic with exponential backoff (delay * 2^attempt)
- [ ] Dead letter queue: move to DLQ after max retries
- [ ] Delayed jobs: schedule execution for a future time (sorted set)
- [ ] Scheduled job poller (checks every 1s, moves to queue)
- [ ] Job cancellation
- [ ] Graceful shutdown: wait for in-flight jobs, then exit
- [ ] Worker heartbeat (detect stale workers)
- [ ] Stale job recovery: re-enqueue jobs from dead workers

### Phase 3 — API Server
- [ ] Gin HTTP server setup
- [ ] REST endpoints: submit, status, list, cancel, retry
- [ ] API key authentication middleware
- [ ] Rate limiting middleware
- [ ] Request logging middleware
- [ ] Queue stats endpoint
- [ ] Dead letter queue endpoints (list, retry, purge)
- [ ] Error handling with consistent JSON responses

### Phase 4 — Dashboard
- [ ] Go template setup with base layout
- [ ] Tailwind CSS (CDN) + dark theme with teal accents
- [ ] HTMX integration for dynamic updates
- [ ] Overview page: stats cards, charts (Chart.js)
- [ ] Jobs list page with filters and pagination
- [ ] Job detail page with timeline and retry history
- [ ] Dead letter queue page with bulk actions
- [ ] Queues page with counts per priority
- [ ] Workers page with heartbeat status
- [ ] Basic auth for dashboard access

### Phase 5 — Real-time & Polish
- [ ] Redis pub/sub for job events
- [ ] WebSocket endpoint for dashboard
- [ ] Live job status updates on dashboard
- [ ] Auto-refreshing stats (HTMX polling)
- [ ] Toast notifications for job events
- [ ] CLI with Cobra (start, worker, enqueue, status, stats)
- [ ] Example tasks (email, image resize, webhook)
- [ ] Docker setup (Dockerfile + docker-compose.yml)
- [ ] README with setup guide, architecture diagram, screenshots

---

## 14. Example Usage (Go SDK)

```go
package main

import (
    "context"
    "encoding/json"
    "log"

    "github.com/Amitanand0123/goqueue"
)

func main() {
    // Create a new queue instance
    q := goqueue.New(goqueue.Config{
        RedisURL:    "localhost:6379",
        Concurrency: 10,
    })

    // Register task handlers
    q.Register("email:send", handleEmailSend)
    q.Register("image:resize", handleImageResize)

    // Start server + workers + dashboard
    q.Start(":8080")
}

func handleEmailSend(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
    var p struct {
        To      string `json:"to"`
        Subject string `json:"subject"`
        Body    string `json:"body"`
    }
    if err := json.Unmarshal(payload, &p); err != nil {
        return nil, err
    }

    // Send the email...
    log.Printf("Sending email to %s: %s", p.To, p.Subject)

    return json.Marshal(map[string]string{"status": "sent"})
}

func handleImageResize(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
    // Resize logic...
    return json.Marshal(map[string]string{"status": "resized"})
}
```

### Submit a job via API

```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret-api-key" \
  -d '{
    "type": "email:send",
    "payload": {
      "to": "user@example.com",
      "subject": "Welcome!",
      "body": "Hello from GoQueue"
    },
    "priority": "high",
    "max_retries": 5
  }'
```

---

## 15. Key Design Decisions

| Decision | Choice | Reasoning |
|---|---|---|
| Queue backend | Redis | Fast, supports BRPOP (blocking pop), sorted sets for scheduling, pub/sub for events |
| Priority handling | Separate lists per priority + BRPOP | BRPOP checks lists left-to-right, so critical is always dequeued first |
| Retry strategy | Exponential backoff with jitter | Prevents thundering herd on transient failures |
| Job storage | Redis hashes | Fast reads, no need for a separate database |
| Dashboard | Go templates + HTMX | Single binary, no frontend build step, server-rendered |
| Real-time updates | WebSocket + Redis pub/sub | Workers publish events → WebSocket handler broadcasts to clients |
| CLI | Cobra | Industry standard for Go CLIs |
| Unique job IDs | UUID v4 | Globally unique, no coordination needed |
| Concurrency model | Goroutine pool + channels | Go's native concurrency primitives, simple and efficient |
