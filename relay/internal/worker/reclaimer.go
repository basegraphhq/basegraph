package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"basegraph.app/relay/common/logger"
	"basegraph.app/relay/internal/queue"
	"github.com/redis/go-redis/v9"
)

type RedisReclaimerConfig struct {
	Stream    string
	Group     string
	Consumer  string
	MinIdle   time.Duration
	Interval  time.Duration
	BatchSize int64
}

// RedisReclaimer periodically reclaims stale pending messages.
// This handles the crash recovery scenario where a worker dies
// after XREADGROUP but before XACK.
type RedisReclaimer struct {
	client    *redis.Client
	cfg       RedisReclaimerConfig
	consumer  *queue.RedisConsumer
	processor queue.MessageProcessor

	stopCh    chan struct{}
	stoppedCh chan struct{}
}

// NewRedisReclaimer creates a new RedisReclaimer.
func NewRedisReclaimer(client *redis.Client, cfg RedisReclaimerConfig, consumer *queue.RedisConsumer, processor queue.MessageProcessor) *RedisReclaimer {
	return &RedisReclaimer{
		client:    client,
		cfg:       cfg,
		consumer:  consumer,
		processor: processor,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Run starts the reclaimer loop. Blocks until Stop() is called.
func (r *RedisReclaimer) Run(ctx context.Context) {
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		Component: "relay.worker.reclaimer",
	})

	defer close(r.stoppedCh)

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	slog.InfoContext(ctx, "reclaimer started",
		"interval", r.cfg.Interval,
		"min_idle", r.cfg.MinIdle,
		"stream", r.cfg.Stream,
		"group", r.cfg.Group)

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			slog.InfoContext(ctx, "reclaimer stopping")
			return
		case <-ticker.C:
			if err := r.reclaimOnce(ctx); err != nil {
				slog.ErrorContext(ctx, "reclaim cycle error", "error", err)
			}
		}
	}
}

// Stop signals the reclaimer to stop gracefully.
func (r *RedisReclaimer) Stop() {
	close(r.stopCh)
	<-r.stoppedCh
}

// reclaimOnce performs one reclaim cycle.
func (r *RedisReclaimer) reclaimOnce(ctx context.Context) error {
	pending, err := r.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: r.cfg.Stream,
		Group:  r.cfg.Group,
		Idle:   r.cfg.MinIdle,
		Start:  "-",
		End:    "+",
		Count:  r.cfg.BatchSize,
	}).Result()
	if err != nil {
		return fmt.Errorf("xpending: %w", err)
	}

	if len(pending) == 0 {
		return nil
	}

	slog.InfoContext(ctx, "found stale pending messages", "count", len(pending))

	for _, p := range pending {
		if err := r.reclaimMessage(ctx, p); err != nil {
			slog.ErrorContext(ctx, "failed to reclaim message",
				"error", err,
				"message_id", p.ID,
				"original_consumer", p.Consumer,
				"idle_time", p.Idle)
			// Continue with other messages
		}
	}

	return nil
}

// reclaimMessage claims and processes a single stale message.
func (r *RedisReclaimer) reclaimMessage(ctx context.Context, pending redis.XPendingExt) error {
	// Enrich context with message ID for logging
	msgID := pending.ID
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		MessageID: &msgID,
	})

	slog.InfoContext(ctx, "reclaiming stale message",
		"original_consumer", pending.Consumer,
		"idle_time", pending.Idle,
		"retry_count", pending.RetryCount)

	messages, err := r.client.XClaim(ctx, &redis.XClaimArgs{
		Stream:   r.cfg.Stream,
		Group:    r.cfg.Group,
		Consumer: r.cfg.Consumer,
		MinIdle:  r.cfg.MinIdle,
		Messages: []string{pending.ID},
	}).Result()
	if err != nil {
		return fmt.Errorf("xclaim: %w", err)
	}

	if len(messages) == 0 {
		slog.DebugContext(ctx, "message already reclaimed by another worker")
		return nil
	}

	msg := messages[0]

	parsed, err := queue.ParseMessage(msg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to parse reclaimed message, acknowledging to prevent loop",
			"error", err)
		_ = r.consumer.Ack(ctx, queue.Message{ID: msg.ID, Raw: msg})
		return nil
	}

	// Enrich context with parsed message fields
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		IssueID:    &parsed.IssueID,
		EventLogID: &parsed.EventLogID,
	})

	slog.DebugContext(ctx, "message claimed successfully")

	start := time.Now()
	if err := r.processor(ctx, parsed); err != nil {
		return fmt.Errorf("processing reclaimed message: %w", err)
	}

	slog.InfoContext(ctx, "reclaimed message processed successfully",
		"duration_ms", time.Since(start).Milliseconds())

	return nil
}
