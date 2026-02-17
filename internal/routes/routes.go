package routes

import (
	"context"
	"html/template"
	"log/slog"
	"strings"
	"time"

	"github.com/Amitanand0123/goqueue/internal/config"
	"github.com/Amitanand0123/goqueue/internal/core"
	"github.com/Amitanand0123/goqueue/internal/handlers"
	"github.com/Amitanand0123/goqueue/internal/middleware"
	"github.com/gin-gonic/gin"
)

// Setup configures all routes for the application.
func Setup(ctx context.Context, engine *core.Engine, cfg *config.Config, logger *slog.Logger) *gin.Engine {
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger(logger))

	// Template functions
	funcMap := template.FuncMap{
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"statusBadge": func(s string) string {
			switch core.Status(s) {
			case core.StatusCompleted:
				return "badge-green"
			case core.StatusRunning:
				return "badge-blue"
			case core.StatusPending:
				return "badge-yellow"
			case core.StatusRetrying:
				return "badge-orange"
			case core.StatusFailed, core.StatusDead:
				return "badge-red"
			case core.StatusCancelled:
				return "badge-gray"
			case core.StatusScheduled:
				return "badge-purple"
			default:
				return "badge-gray"
			}
		},
		"priorityBadge": func(p core.Priority) string {
			switch p {
			case core.PriorityCritical:
				return "badge-red"
			case core.PriorityHigh:
				return "badge-orange"
			case core.PriorityDefault:
				return "badge-teal"
			case core.PriorityLow:
				return "badge-gray"
			default:
				return "badge-gray"
			}
		},
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "-"
			}
			return t.Format("2006-01-02 15:04:05")
		},
		"formatTimePtr": func(t *time.Time) string {
			if t == nil {
				return "-"
			}
			return t.Format("2006-01-02 15:04:05")
		},
		"formatDuration": func(t *time.Time, start *time.Time) string {
			if t == nil || start == nil {
				return "-"
			}
			d := t.Sub(*start)
			return d.Round(time.Millisecond).String()
		},
		"upper": strings.ToUpper,
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"seq": func(start, end int) []int {
			s := make([]int, 0)
			for i := start; i <= end; i++ {
				s = append(s, i)
			}
			return s
		},
		"totalPages": func(total, limit int) int {
			if limit == 0 {
				return 1
			}
			pages := total / limit
			if total%limit != 0 {
				pages++
			}
			return pages
		},
		"json": func(v interface{}) string {
			switch val := v.(type) {
			case string:
				return val
			default:
				return ""
			}
		},
	}

	// Load templates: each page gets its own isolated template tree
	// so that {{define "content"}} blocks don't collide across pages
	r.HTMLRender = NewMultiRenderer(funcMap, logger)
	r.Static("/static", "web/static")

	// API routes
	api := r.Group("/api/v1")
	{
		// Rate limiting
		rl := middleware.NewRateLimiter(10, 20) // 10 req/s, burst of 20
		api.Use(rl.Middleware())

		// API key auth
		api.Use(middleware.APIKeyAuth(cfg.APIKey))

		apiHandler := handlers.NewAPIHandler(engine)

		api.POST("/jobs", apiHandler.SubmitJob)
		api.GET("/jobs", apiHandler.ListJobs)
		api.GET("/jobs/:id", apiHandler.GetJob)
		api.DELETE("/jobs/:id", apiHandler.CancelJob)
		api.POST("/jobs/:id/retry", apiHandler.RetryJob)

		api.GET("/dead", apiHandler.ListDead)
		api.DELETE("/dead", apiHandler.PurgeDead)

		api.GET("/stats", apiHandler.GetStats)
	}

	// Dashboard routes
	if cfg.DashboardEnabled {
		wsHub := handlers.NewWebSocketHub(engine.Broker, logger)
		wsHub.Start(ctx)

		dash := r.Group("/dashboard")
		if cfg.DashboardUsername != "" && cfg.DashboardPassword != "" {
			dash.Use(middleware.BasicAuth(cfg.DashboardUsername, cfg.DashboardPassword))
		}

		dashHandler := handlers.NewDashboardHandler(engine)

		dash.GET("", dashHandler.Overview)
		dash.GET("/submit", dashHandler.Submit)
		dash.POST("/submit", dashHandler.SubmitJob)
		dash.GET("/queues", dashHandler.Queues)
		dash.GET("/jobs", dashHandler.Jobs)
		dash.GET("/jobs/:id", dashHandler.JobDetail)
		dash.GET("/dead", dashHandler.Dead)
		dash.GET("/scheduled", dashHandler.Scheduled)
		dash.GET("/workers", dashHandler.Workers)

		// HTMX partials
		dash.GET("/partials/stats", dashHandler.StatsPartial)
		dash.GET("/partials/jobs", dashHandler.JobsPartial)

		// WebSocket
		r.GET("/ws", wsHub.HandleWS)
	}

	// Health check (public, no auth)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Root redirect
	r.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/dashboard")
	})

	return r
}
