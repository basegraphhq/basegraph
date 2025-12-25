package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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
	processor MessageProcessor

	stopCh    chan struct{}
	stoppedCh chan struct{}
}

// MessageProcessor processes a reclaimed message.
type MessageProcessor func(ctx context.Context, msg queue.Message) error

// NewRedisReclaimer creates a new RedisReclaimer.
func NewRedisReclaimer(client *redis.Client, cfg RedisReclaimerConfig, processor MessageProcessor) *RedisReclaimer {
	return &RedisReclaimer{
		client:    client,
		cfg:       cfg,
		processor: processor,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Run starts the reclaimer loop. Blocks until Stop() is called.
func (r *RedisReclaimer) Run(ctx context.Context) {
	defer close(r.stoppedCh)

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	slog.InfoContext(ctx, "reclaimer started",
		"interval", r.cfg.Interval,
		"min_idle", r.cfg.MinIdle)

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			slog.InfoContext(ctx, "reclaimer stopping")
			return
		case <-ticker.C:
			if err := r.reclaimOnce(ctx); err != nil {
				slog.ErrorContext(ctx, "reclaim error", "error", err)
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
	// Step 1: Find pending messages older than MinIdle
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

	// Step 2: Claim each message
	for _, p := range pending {
		if err := r.reclaimMessage(ctx, p); err != nil {
			slog.ErrorContext(ctx, "failed to reclaim message",
				"error", err,
				"message_id", p.ID,
				"consumer", p.Consumer,
				"idle", p.Idle)
			// Continue with other messages
		}
	}

	return nil
}

// reclaimMessage claims and processes a single stale message.
func (r *RedisReclaimer) reclaimMessage(ctx context.Context, pending redis.XPendingExt) error {
	slog.InfoContext(ctx, "reclaiming stale message",
		"message_id", pending.ID,
		"original_consumer", pending.Consumer,
		"idle_time", pending.Idle,
		"retry_count", pending.RetryCount)

	// Step 1: XCLAIM the message
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
		// Message was already claimed by someone else
		slog.DebugContext(ctx, "message already reclaimed by another worker",
			"message_id", pending.ID)
		return nil
	}

	msg := messages[0]

	// Step 2: Parse the message
	parsed, err := queue.ParseMessage(msg)
	if err != nil {
		// Can't parse - ACK it to prevent infinite loop
		slog.ErrorContext(ctx, "failed to parse reclaimed message, acknowledging",
			"error", err,
			"message_id", msg.ID)
		_ = r.client.XAck(ctx, r.cfg.Stream, r.cfg.Group, msg.ID).Err()
		return nil
	}

	// Step 3: Process the message (reuses worker logic)
	if err := r.processor(ctx, parsed); err != nil {
		return fmt.Errorf("processing reclaimed message: %w", err)
	}

	return nil
}
