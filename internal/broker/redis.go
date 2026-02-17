package broker

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Amitanand0123/goqueue/internal/config"
	"github.com/redis/go-redis/v9"
)

// Broker wraps a Redis client and provides queue operations.
type Broker struct {
	client *redis.Client
	logger *slog.Logger
}

// New creates a new Redis broker from config.
func New(cfg *config.Config, logger *slog.Logger) (*Broker, error) {
	opts := &redis.Options{
		DB:           cfg.RedisDB,
		Password:     cfg.RedisPassword,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Support redis:// and rediss:// URLs (e.g., Upstash TLS)
	if strings.HasPrefix(cfg.RedisURL, "redis://") || strings.HasPrefix(cfg.RedisURL, "rediss://") {
		parsed, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("invalid Redis URL: %w", err)
		}
		opts = parsed
		if cfg.RedisPassword != "" {
			opts.Password = cfg.RedisPassword
		}
		opts.DB = cfg.RedisDB
	} else {
		opts.Addr = cfg.RedisURL
	}

	// Enable TLS for rediss:// URLs
	if strings.HasPrefix(cfg.RedisURL, "rediss://") {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("connected to Redis", "addr", opts.Addr)

	return &Broker{client: client, logger: logger}, nil
}

// Client returns the underlying Redis client.
func (b *Broker) Client() *redis.Client {
	return b.client
}

// Close closes the Redis connection.
func (b *Broker) Close() error {
	return b.client.Close()
}

// --- Queue Operations ---

// Enqueue pushes a job ID to the left of a queue (LPUSH for FIFO with BRPOP).
func (b *Broker) Enqueue(ctx context.Context, queueName, jobID string) error {
	return b.client.LPush(ctx, queueName, jobID).Err()
}

// Dequeue blocks and pops a job ID from the right of the highest priority queue.
// Checks queues in order: critical → high → default → low.
func (b *Broker) Dequeue(ctx context.Context, queues []string, timeout time.Duration) (string, string, error) {
	result, err := b.client.BRPop(ctx, timeout, queues...).Result()
	if err != nil {
		if err == redis.Nil {
			return "", "", nil // timeout, no job
		}
		return "", "", err
	}
	// result[0] = queue name, result[1] = job ID
	return result[0], result[1], nil
}

// QueueLen returns the number of items in a queue.
func (b *Broker) QueueLen(ctx context.Context, queueName string) (int64, error) {
	return b.client.LLen(ctx, queueName).Result()
}

// --- Job Storage ---

// SaveJob stores all job fields in a Redis hash.
func (b *Broker) SaveJob(ctx context.Context, jobID string, fields map[string]interface{}) error {
	key := "goqueue:job:" + jobID
	return b.client.HSet(ctx, key, fields).Err()
}

// GetJob retrieves all fields of a job from Redis.
func (b *Broker) GetJob(ctx context.Context, jobID string) (map[string]string, error) {
	key := "goqueue:job:" + jobID
	result, err := b.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	return result, nil
}

// UpdateJobField updates a single field in a job hash.
func (b *Broker) UpdateJobField(ctx context.Context, jobID, field string, value interface{}) error {
	key := "goqueue:job:" + jobID
	return b.client.HSet(ctx, key, field, value).Err()
}

// UpdateJobFields updates multiple fields in a job hash.
func (b *Broker) UpdateJobFields(ctx context.Context, jobID string, fields map[string]interface{}) error {
	key := "goqueue:job:" + jobID
	return b.client.HSet(ctx, key, fields).Err()
}

// DeleteJob removes a job hash from Redis.
func (b *Broker) DeleteJob(ctx context.Context, jobID string) error {
	key := "goqueue:job:" + jobID
	return b.client.Del(ctx, key).Err()
}

// --- Status Index ---

// AddToStatusSet adds a job ID to a status set.
func (b *Broker) AddToStatusSet(ctx context.Context, status, jobID string) error {
	key := "goqueue:status:" + status
	return b.client.SAdd(ctx, key, jobID).Err()
}

// RemoveFromStatusSet removes a job ID from a status set.
func (b *Broker) RemoveFromStatusSet(ctx context.Context, status, jobID string) error {
	key := "goqueue:status:" + status
	return b.client.SRem(ctx, key, jobID).Err()
}

// GetStatusMembers returns all job IDs in a status set.
func (b *Broker) GetStatusMembers(ctx context.Context, status string) ([]string, error) {
	key := "goqueue:status:" + status
	return b.client.SMembers(ctx, key).Result()
}

// StatusSetCount returns the number of members in a status set.
func (b *Broker) StatusSetCount(ctx context.Context, status string) (int64, error) {
	key := "goqueue:status:" + status
	return b.client.SCard(ctx, key).Result()
}

// MoveStatus moves a job ID from one status set to another.
func (b *Broker) MoveStatus(ctx context.Context, jobID, from, to string) error {
	pipe := b.client.Pipeline()
	pipe.SRem(ctx, "goqueue:status:"+from, jobID)
	pipe.SAdd(ctx, "goqueue:status:"+to, jobID)
	_, err := pipe.Exec(ctx)
	return err
}

// --- Scheduled Jobs (Sorted Set) ---

// ScheduleJob adds a job to the scheduled sorted set with a timestamp score.
func (b *Broker) ScheduleJob(ctx context.Context, jobID string, executeAt time.Time) error {
	return b.client.ZAdd(ctx, "goqueue:scheduled", redis.Z{
		Score:  float64(executeAt.Unix()),
		Member: jobID,
	}).Err()
}

// GetDueJobs returns job IDs that are due for execution (score <= now).
func (b *Broker) GetDueJobs(ctx context.Context, now time.Time, limit int64) ([]string, error) {
	return b.client.ZRangeByScore(ctx, "goqueue:scheduled", &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", now.Unix()),
		Count: limit,
	}).Result()
}

// RemoveScheduled removes a job from the scheduled sorted set.
func (b *Broker) RemoveScheduled(ctx context.Context, jobID string) error {
	return b.client.ZRem(ctx, "goqueue:scheduled", jobID).Err()
}

// --- Dead Letter Queue ---

// PushToDLQ pushes a job ID to the dead letter queue.
func (b *Broker) PushToDLQ(ctx context.Context, jobID string) error {
	return b.client.LPush(ctx, "goqueue:dead", jobID).Err()
}

// GetDLQJobs returns job IDs from the dead letter queue.
func (b *Broker) GetDLQJobs(ctx context.Context, start, stop int64) ([]string, error) {
	return b.client.LRange(ctx, "goqueue:dead", start, stop).Result()
}

// DLQLen returns the length of the dead letter queue.
func (b *Broker) DLQLen(ctx context.Context) (int64, error) {
	return b.client.LLen(ctx, "goqueue:dead").Result()
}

// RemoveFromDLQ removes a specific job from the DLQ.
func (b *Broker) RemoveFromDLQ(ctx context.Context, jobID string) error {
	return b.client.LRem(ctx, "goqueue:dead", 1, jobID).Err()
}

// PurgeDLQ removes all jobs from the dead letter queue and returns the count.
func (b *Broker) PurgeDLQ(ctx context.Context) (int64, error) {
	count, err := b.client.LLen(ctx, "goqueue:dead").Result()
	if err != nil {
		return 0, err
	}
	if err := b.client.Del(ctx, "goqueue:dead").Err(); err != nil {
		return 0, err
	}
	return count, nil
}

// --- Metrics ---

// IncrMetric increments a counter metric.
func (b *Broker) IncrMetric(ctx context.Context, metric string) error {
	pipe := b.client.Pipeline()
	pipe.Incr(ctx, "goqueue:metrics:"+metric)
	// Also increment daily counter
	date := time.Now().UTC().Format("2006-01-02")
	pipe.Incr(ctx, "goqueue:metrics:"+metric+":"+date)
	_, err := pipe.Exec(ctx)
	return err
}

// GetMetric returns a metric counter value.
func (b *Broker) GetMetric(ctx context.Context, metric string) (int64, error) {
	val, err := b.client.Get(ctx, "goqueue:metrics:"+metric).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// GetDailyMetric returns a daily metric counter value.
func (b *Broker) GetDailyMetric(ctx context.Context, metric, date string) (int64, error) {
	val, err := b.client.Get(ctx, "goqueue:metrics:"+metric+":"+date).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// --- Worker Registry ---

// RegisterWorker registers a worker with a heartbeat timestamp.
func (b *Broker) RegisterWorker(ctx context.Context, workerID string) error {
	return b.client.HSet(ctx, "goqueue:workers", workerID, time.Now().UTC().Format(time.RFC3339)).Err()
}

// UpdateHeartbeat updates the heartbeat for a worker.
func (b *Broker) UpdateHeartbeat(ctx context.Context, workerID string) error {
	return b.client.HSet(ctx, "goqueue:workers", workerID, time.Now().UTC().Format(time.RFC3339)).Err()
}

// UnregisterWorker removes a worker from the registry.
func (b *Broker) UnregisterWorker(ctx context.Context, workerID string) error {
	return b.client.HDel(ctx, "goqueue:workers", workerID).Err()
}

// GetWorkers returns all registered workers with their last heartbeat.
func (b *Broker) GetWorkers(ctx context.Context) (map[string]string, error) {
	return b.client.HGetAll(ctx, "goqueue:workers").Result()
}

// ActiveWorkerCount returns the number of registered workers.
func (b *Broker) ActiveWorkerCount(ctx context.Context) (int64, error) {
	return b.client.HLen(ctx, "goqueue:workers").Result()
}

// --- Pub/Sub ---

// Publish publishes an event to the events channel.
func (b *Broker) Publish(ctx context.Context, event string) error {
	return b.client.Publish(ctx, "goqueue:events", event).Err()
}

// Subscribe returns a pub/sub subscription to the events channel.
func (b *Broker) Subscribe(ctx context.Context) *redis.PubSub {
	return b.client.Subscribe(ctx, "goqueue:events")
}
