package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Amitanand0123/goqueue/internal/broker"
	"github.com/Amitanand0123/goqueue/internal/config"
	"github.com/Amitanand0123/goqueue/internal/core"
	"github.com/Amitanand0123/goqueue/internal/routes"
	"github.com/spf13/cobra"
)

var cfg *config.Config

func main() {
	cfg = config.Load()

	rootCmd := &cobra.Command{
		Use:   "goqueue",
		Short: "GoQueue — Distributed Task Queue System",
	}

	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(serverCmd())
	rootCmd.AddCommand(workerCmd())
	rootCmd.AddCommand(enqueueCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(statsCmd())
	rootCmd.AddCommand(retryDeadCmd())
	rootCmd.AddCommand(purgeDeadCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	if cfg.Environment == "development" {
		opts.Level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func newEngine(logger *slog.Logger) (*core.Engine, error) {
	b, err := broker.New(cfg, logger)
	if err != nil {
		return nil, err
	}
	return core.NewEngine(cfg, b, logger), nil
}

// registerExampleTasks registers demo task handlers.
func registerExampleTasks(engine *core.Engine) {
	engine.Register("email:send", func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
		var p struct {
			To      string `json:"to"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, err
		}
		slog.Info("sending email", "to", p.To, "subject", p.Subject)
		time.Sleep(500 * time.Millisecond) // simulate work
		return json.Marshal(map[string]string{"status": "sent", "to": p.To})
	})

	engine.Register("image:resize", func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
		slog.Info("resizing image")
		time.Sleep(1 * time.Second) // simulate work
		return json.Marshal(map[string]string{"status": "resized"})
	})

	engine.Register("webhook:deliver", func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
		var p struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, err
		}
		slog.Info("delivering webhook", "url", p.URL)
		time.Sleep(300 * time.Millisecond)
		return json.Marshal(map[string]string{"status": "delivered", "url": p.URL})
	})
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start API server + workers + dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger()
			engine, err := newEngine(logger)
			if err != nil {
				return err
			}
			registerExampleTasks(engine)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Start workers
			engine.StartWorkers(ctx)

			// Setup HTTP server
			router := routes.Setup(ctx, engine, cfg, logger)
			srv := &http.Server{
				Addr:    ":" + cfg.Port,
				Handler: router,
			}

			// Graceful shutdown
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh

				logger.Info("shutdown signal received")
				cancel()

				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
				defer shutdownCancel()

				srv.Shutdown(shutdownCtx)
				engine.Shutdown()
			}()

			logger.Info("GoQueue started", "port", cfg.Port, "dashboard", cfg.DashboardEnabled)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}
}

func serverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "Start only the API server + dashboard (no workers)",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger()
			engine, err := newEngine(logger)
			if err != nil {
				return err
			}
			registerExampleTasks(engine)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			router := routes.Setup(ctx, engine, cfg, logger)
			srv := &http.Server{
				Addr:    ":" + cfg.Port,
				Handler: router,
			}

			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				cancel()
				srv.Shutdown(context.Background())
				engine.Broker.Close()
			}()

			logger.Info("GoQueue server started (no workers)", "port", cfg.Port)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}
}

func workerCmd() *cobra.Command {
	var concurrency int
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Start only workers (no API server)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if concurrency > 0 {
				cfg.WorkerConcurrency = concurrency
			}

			logger := newLogger()
			engine, err := newEngine(logger)
			if err != nil {
				return err
			}
			registerExampleTasks(engine)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			engine.StartWorkers(ctx)

			logger.Info("GoQueue workers started", "concurrency", cfg.WorkerConcurrency)

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh

			logger.Info("shutting down workers...")
			engine.Shutdown()
			return nil
		},
	}
	cmd.Flags().IntVar(&concurrency, "concurrency", 0, "Number of worker goroutines")
	return cmd
}

func enqueueCmd() *cobra.Command {
	var (
		jobType  string
		payload  string
		priority string
	)
	cmd := &cobra.Command{
		Use:   "enqueue",
		Short: "Submit a job from the CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger()
			engine, err := newEngine(logger)
			if err != nil {
				return err
			}
			defer engine.Broker.Close()
			registerExampleTasks(engine)

			opts := []core.JobOption{}
			if priority != "" {
				opts = append(opts, core.WithPriority(core.ParsePriority(priority)))
			}

			job := core.NewJob(jobType, json.RawMessage(payload), opts...)
			if err := engine.Scheduler.Enqueue(context.Background(), job); err != nil {
				return err
			}

			fmt.Printf("Job enqueued: %s (type=%s, priority=%s)\n", job.ID, job.Type, job.Priority.String())
			return nil
		},
	}
	cmd.Flags().StringVar(&jobType, "type", "", "Task type (e.g., email:send)")
	cmd.Flags().StringVar(&payload, "payload", "{}", "JSON payload")
	cmd.Flags().StringVar(&priority, "priority", "default", "Priority (critical, high, default, low)")
	cmd.MarkFlagRequired("type")
	return cmd
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [job-id]",
		Short: "Check job status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger()
			engine, err := newEngine(logger)
			if err != nil {
				return err
			}
			defer engine.Broker.Close()

			job, err := engine.Results.GetResult(context.Background(), args[0])
			if err != nil {
				return err
			}

			data, _ := json.MarshalIndent(job, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}
}

func listCmd() *cobra.Command {
	var (
		status string
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger()
			engine, err := newEngine(logger)
			if err != nil {
				return err
			}
			defer engine.Broker.Close()

			var jobs []*core.Job
			if status != "" {
				jobs, err = engine.Results.ListByStatus(context.Background(), core.Status(status))
			} else {
				for _, s := range []core.Status{core.StatusPending, core.StatusRunning, core.StatusCompleted, core.StatusFailed} {
					sJobs, _ := engine.Results.ListByStatus(context.Background(), s)
					jobs = append(jobs, sJobs...)
				}
			}
			if err != nil {
				return err
			}

			if limit > 0 && len(jobs) > limit {
				jobs = jobs[:limit]
			}

			for _, j := range jobs {
				fmt.Printf("[%s] %s  type=%s  status=%s  priority=%s\n",
					j.ID[:8], j.CreatedAt.Format("15:04:05"), j.Type, j.Status, j.Priority.String())
			}
			fmt.Printf("\nTotal: %d jobs\n", len(jobs))
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max results")
	return cmd
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "View queue stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger()
			engine, err := newEngine(logger)
			if err != nil {
				return err
			}
			defer engine.Broker.Close()

			stats, err := engine.Stats(context.Background())
			if err != nil {
				return err
			}

			data, _ := json.MarshalIndent(stats, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}
}

func retryDeadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retry-dead",
		Short: "Retry all dead jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger()
			engine, err := newEngine(logger)
			if err != nil {
				return err
			}
			defer engine.Broker.Close()

			count, err := engine.DLQ.RetryAll(context.Background())
			if err != nil {
				return err
			}

			fmt.Printf("Retried %d dead jobs\n", count)
			return nil
		},
	}
}

func purgeDeadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "purge-dead",
		Short: "Purge dead letter queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger()
			engine, err := newEngine(logger)
			if err != nil {
				return err
			}
			defer engine.Broker.Close()

			count, err := engine.DLQ.Purge(context.Background())
			if err != nil {
				return err
			}

			fmt.Printf("Purged %d dead jobs\n", count)
			return nil
		},
	}
}
