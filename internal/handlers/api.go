package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Amitanand0123/goqueue/internal/core"
	"github.com/gin-gonic/gin"
)

// APIHandler handles REST API requests.
type APIHandler struct {
	engine *core.Engine
}

// NewAPIHandler creates a new API handler.
func NewAPIHandler(engine *core.Engine) *APIHandler {
	return &APIHandler{engine: engine}
}

// SubmitJobRequest is the request body for submitting a job.
type SubmitJobRequest struct {
	Type       string          `json:"type" binding:"required"`
	Payload    json.RawMessage `json:"payload" binding:"required"`
	Priority   string          `json:"priority"`
	MaxRetries *int            `json:"max_retries"`
	Timeout    string          `json:"timeout"`
	Delay      string          `json:"delay"`
}

// SubmitJob handles POST /api/v1/jobs
func (h *APIHandler) SubmitJob(c *gin.Context) {
	var req SubmitJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	opts := []core.JobOption{}

	if req.Priority != "" {
		opts = append(opts, core.WithPriority(core.ParsePriority(req.Priority)))
	}
	if req.MaxRetries != nil {
		opts = append(opts, core.WithMaxRetries(*req.MaxRetries))
	}
	if req.Timeout != "" {
		if d, err := time.ParseDuration(req.Timeout); err == nil {
			opts = append(opts, core.WithTimeout(d))
		}
	}
	if req.Delay != "" {
		if d, err := time.ParseDuration(req.Delay); err == nil {
			opts = append(opts, core.WithDelay(d))
		}
	}

	job := core.NewJob(req.Type, req.Payload, opts...)

	if err := h.engine.Scheduler.Enqueue(c.Request.Context(), job); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         job.ID,
		"type":       job.Type,
		"status":     string(job.Status),
		"priority":   job.Priority.String(),
		"created_at": job.CreatedAt.Format(time.RFC3339),
	})
}

// GetJob handles GET /api/v1/jobs/:id
func (h *APIHandler) GetJob(c *gin.Context) {
	jobID := c.Param("id")

	job, err := h.engine.Results.GetResult(c.Request.Context(), jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	c.JSON(http.StatusOK, job)
}

// ListJobs handles GET /api/v1/jobs
func (h *APIHandler) ListJobs(c *gin.Context) {
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	var jobs []*core.Job
	var err error

	if status != "" {
		jobs, err = h.engine.Results.ListByStatus(c.Request.Context(), core.Status(status))
	} else {
		// Default to listing all non-dead jobs
		var allJobs []*core.Job
		for _, s := range []core.Status{core.StatusPending, core.StatusScheduled, core.StatusRunning, core.StatusCompleted, core.StatusRetrying, core.StatusFailed} {
			sJobs, e := h.engine.Results.ListByStatus(c.Request.Context(), s)
			if e != nil {
				continue
			}
			allJobs = append(allJobs, sJobs...)
		}
		jobs = allJobs
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list jobs"})
		return
	}

	// Filter by type if provided
	if jobType := c.Query("type"); jobType != "" {
		filtered := make([]*core.Job, 0)
		for _, j := range jobs {
			if j.Type == jobType {
				filtered = append(filtered, j)
			}
		}
		jobs = filtered
	}

	// Paginate
	total := len(jobs)
	start := (page - 1) * limit
	end := start + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	c.JSON(http.StatusOK, gin.H{
		"jobs":  jobs[start:end],
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// CancelJob handles DELETE /api/v1/jobs/:id
func (h *APIHandler) CancelJob(c *gin.Context) {
	jobID := c.Param("id")

	if err := h.engine.Results.CancelJob(c.Request.Context(), jobID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Job cancelled"})
}

// RetryJob handles POST /api/v1/jobs/:id/retry
func (h *APIHandler) RetryJob(c *gin.Context) {
	jobID := c.Param("id")

	if err := h.engine.DLQ.RetryJob(c.Request.Context(), jobID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Job re-enqueued",
		"status":  "pending",
	})
}

// PurgeDead handles DELETE /api/v1/dead
func (h *APIHandler) PurgeDead(c *gin.Context) {
	count, err := h.engine.DLQ.Purge(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to purge DLQ"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Purged " + strconv.FormatInt(count, 10) + " dead jobs",
	})
}

// ListDead handles GET /api/v1/dead
func (h *APIHandler) ListDead(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	start := int64((page - 1) * limit)
	stop := start + int64(limit) - 1

	jobs, err := h.engine.DLQ.List(c.Request.Context(), start, stop)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list dead jobs"})
		return
	}

	total, _ := h.engine.DLQ.Count(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"jobs":  jobs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetStats handles GET /api/v1/stats
func (h *APIHandler) GetStats(c *gin.Context) {
	stats, err := h.engine.Stats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	c.JSON(http.StatusOK, stats)
}
