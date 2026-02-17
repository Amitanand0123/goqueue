package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Server
	Port        string
	Environment string
	APIKey      string

	// Redis
	RedisURL      string
	RedisPassword string
	RedisDB       int

	// Workers
	WorkerConcurrency  int
	ShutdownTimeout    time.Duration

	// Job defaults
	DefaultTimeout    time.Duration
	DefaultMaxRetries int
	DefaultRetryDelay time.Duration

	// Dashboard
	DashboardEnabled  bool
	DashboardUsername string
	DashboardPassword string

	// Metrics
	MetricsRetentionDays int
}

func Load() *Config {
	return &Config{
		Port:        getEnv("PORT", "8080"),
		Environment: getEnv("ENVIRONMENT", "development"),
		APIKey:      getEnv("API_KEY", ""),

		RedisURL:      getEnv("REDIS_URL", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		WorkerConcurrency:  getEnvInt("WORKER_CONCURRENCY", 10),
		ShutdownTimeout:    getEnvDuration("WORKER_SHUTDOWN_TIMEOUT", 30*time.Second),

		DefaultTimeout:    getEnvDuration("JOB_DEFAULT_TIMEOUT", 30*time.Second),
		DefaultMaxRetries: getEnvInt("JOB_DEFAULT_MAX_RETRIES", 3),
		DefaultRetryDelay: getEnvDuration("JOB_DEFAULT_RETRY_DELAY", 10*time.Second),

		DashboardEnabled:  getEnvBool("DASHBOARD_ENABLED", true),
		DashboardUsername: getEnv("DASHBOARD_USERNAME", "admin"),
		DashboardPassword: getEnv("DASHBOARD_PASSWORD", "admin"),

		MetricsRetentionDays: getEnvInt("METRICS_RETENTION_DAYS", 30),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
