package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Amitanand0123/goqueue/internal/core"
	"github.com/gin-gonic/gin"
)

// DashboardHandler handles dashboard page rendering.
type DashboardHandler struct {
	engine *core.Engine
}

// NewDashboardHandler creates a new dashboard handler.
func NewDashboardHandler(engine *core.Engine) *DashboardHandler {
	return &DashboardHandler{engine: engine}
}

// Overview renders the main dashboard page.
func (h *DashboardHandler) Overview(c *gin.Context) {
	stats, err := h.engine.Stats(c.Request.Context())
	if err != nil {
		c.HTML(http.StatusInternalServerError, "pages/dashboard.html", gin.H{"error": "Failed to load stats"})
		return
	}

	// Get recent jobs
	recentJobs := make([]*core.Job, 0)
	for _, status := range []core.Status{core.StatusRunning, core.StatusCompleted, core.StatusFailed, core.StatusPending} {
		jobs, _ := h.engine.Results.ListByStatus(c.Request.Context(), status)
		recentJobs = append(recentJobs, jobs...)
	}

	// Limit to 10 most recent
	if len(recentJobs) > 10 {
		recentJobs = recentJobs[:10]
	}

	c.HTML(http.StatusOK, "pages/dashboard.html", gin.H{
		"title":      "Dashboard",
		"active":     "dashboard",
		"stats":      stats,
		"recentJobs": recentJobs,
	})
}

// Queues renders the queues page.
func (h *DashboardHandler) Queues(c *gin.Context) {
	ctx := c.Request.Context()

	critLen, _ := h.engine.Broker.QueueLen(ctx, core.PriorityCritical.QueueName())
	highLen, _ := h.engine.Broker.QueueLen(ctx, core.PriorityHigh.QueueName())
	defaultLen, _ := h.engine.Broker.QueueLen(ctx, core.PriorityDefault.QueueName())
	lowLen, _ := h.engine.Broker.QueueLen(ctx, core.PriorityLow.QueueName())

	queues := []map[string]interface{}{
		{"name": "Critical", "key": "critical", "count": critLen, "color": "red"},
		{"name": "High", "key": "high", "count": highLen, "color": "orange"},
		{"name": "Default", "key": "default", "count": defaultLen, "color": "teal"},
		{"name": "Low", "key": "low", "count": lowLen, "color": "gray"},
	}

	c.HTML(http.StatusOK, "pages/queues.html", gin.H{
		"title":  "Queues",
		"active": "queues",
		"queues": queues,
	})
}

// Jobs renders the jobs list page.
func (h *DashboardHandler) Jobs(c *gin.Context) {
	statusFilter := c.Query("status")
	typeFilter := c.Query("type")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 20

	var jobs []*core.Job

	if statusFilter != "" {
		jobs, _ = h.engine.Results.ListByStatus(c.Request.Context(), core.Status(statusFilter))
	} else {
		for _, s := range []core.Status{core.StatusPending, core.StatusScheduled, core.StatusRunning, core.StatusCompleted, core.StatusRetrying, core.StatusFailed, core.StatusCancelled} {
			sJobs, _ := h.engine.Results.ListByStatus(c.Request.Context(), s)
			jobs = append(jobs, sJobs...)
		}
	}

	if typeFilter != "" {
		filtered := make([]*core.Job, 0)
		for _, j := range jobs {
			if j.Type == typeFilter {
				filtered = append(filtered, j)
			}
		}
		jobs = filtered
	}

	total := len(jobs)
	start := (page - 1) * limit
	end := start + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	c.HTML(http.StatusOK, "pages/jobs.html", gin.H{
		"title":  "Jobs",
		"active": "jobs",
		"jobs":   jobs[start:end],
		"total":  total,
		"page":   page,
		"limit":  limit,
		"status": statusFilter,
		"type":   typeFilter,
	})
}

// JobDetail renders a single job detail page.
func (h *DashboardHandler) JobDetail(c *gin.Context) {
	jobID := c.Param("id")

	job, err := h.engine.Results.GetResult(c.Request.Context(), jobID)
	if err != nil {
		c.HTML(http.StatusNotFound, "pages/job_detail.html", gin.H{
			"title": "Job Not Found",
			"error": "Job not found",
		})
		return
	}

	c.HTML(http.StatusOK, "pages/job_detail.html", gin.H{
		"title":  "Job " + jobID[:8],
		"active": "jobs",
		"job":    job,
	})
}

// Dead renders the dead letter queue page.
func (h *DashboardHandler) Dead(c *gin.Context) {
	jobs, err := h.engine.DLQ.List(c.Request.Context(), 0, -1)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "pages/dead.html", gin.H{
			"title": "Dead Letter Queue",
			"error": "Failed to load DLQ",
		})
		return
	}

	count, _ := h.engine.DLQ.Count(c.Request.Context())

	c.HTML(http.StatusOK, "pages/dead.html", gin.H{
		"title":  "Dead Letter Queue",
		"active": "dead",
		"jobs":   jobs,
		"count":  count,
	})
}

// Workers renders the workers page.
func (h *DashboardHandler) Workers(c *gin.Context) {
	workers, err := h.engine.Broker.GetWorkers(c.Request.Context())
	if err != nil {
		c.HTML(http.StatusInternalServerError, "pages/workers.html", gin.H{
			"title": "Workers",
			"error": "Failed to load workers",
		})
		return
	}

	workerList := make([]map[string]string, 0)
	for id, heartbeat := range workers {
		workerList = append(workerList, map[string]string{
			"id":        id,
			"heartbeat": heartbeat,
		})
	}

	c.HTML(http.StatusOK, "pages/workers.html", gin.H{
		"title":   "Workers",
		"active":  "workers",
		"workers": workerList,
	})
}

// StatsPartial returns just the stats cards HTML for HTMX polling.
func (h *DashboardHandler) StatsPartial(c *gin.Context) {
	stats, err := h.engine.Stats(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "Error loading stats")
		return
	}
	c.HTML(http.StatusOK, "partials/stats_cards.html", gin.H{"stats": stats})
}

// JobsPartial returns just the jobs table rows for HTMX polling.
func (h *DashboardHandler) JobsPartial(c *gin.Context) {
	recentJobs := make([]*core.Job, 0)
	for _, status := range []core.Status{core.StatusRunning, core.StatusCompleted, core.StatusFailed, core.StatusPending} {
		jobs, _ := h.engine.Results.ListByStatus(c.Request.Context(), status)
		recentJobs = append(recentJobs, jobs...)
	}
	if len(recentJobs) > 10 {
		recentJobs = recentJobs[:10]
	}

	c.HTML(http.StatusOK, "partials/job_row.html", gin.H{"jobs": recentJobs})
}

// Submit renders the job submission page.
func (h *DashboardHandler) Submit(c *gin.Context) {
	taskTypes := h.engine.Registry.Types()

	c.HTML(http.StatusOK, "pages/submit.html", gin.H{
		"title":     "Submit Job",
		"active":    "submit",
		"taskTypes": taskTypes,
	})
}

// SubmitJob handles POST /dashboard/submit — creates a job without API key auth.
func (h *DashboardHandler) SubmitJob(c *gin.Context) {
	var req struct {
		Type       string          `json:"type" binding:"required"`
		Payload    json.RawMessage `json:"payload" binding:"required"`
		Priority   string          `json:"priority"`
		MaxRetries *int            `json:"max_retries"`
		Timeout    string          `json:"timeout"`
		Delay      string          `json:"delay"`
	}
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
		"id":       job.ID,
		"type":     job.Type,
		"status":   string(job.Status),
		"priority": job.Priority.String(),
	})
}

// Scheduled renders the scheduled jobs page.
func (h *DashboardHandler) Scheduled(c *gin.Context) {
	jobs, _ := h.engine.Results.ListByStatus(c.Request.Context(), core.StatusScheduled)

	c.HTML(http.StatusOK, "pages/scheduled.html", gin.H{
		"title":  "Scheduled Jobs",
		"active": "scheduled",
		"jobs":   jobs,
	})
}
