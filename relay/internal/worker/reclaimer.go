package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"basegraph.app/relay/internal/queue"
	"github.com/redis/go-redis/v9"
)

// ReclaimerConfig holds reclaimer configuration.
type ReclaimerConfig struct {
	Stream    string
	Group     string
	Consumer  string
	MinIdle   time.Duration // Minimum time a message must be pending before reclaim
	Interval  time.Duration // How often to run the reclaim check
	BatchSize int64         // How many pending messages to check at once
}

// Reclaimer periodically reclaims stale pending messages.
// This handles the crash recovery scenario where a worker dies
// after XREADGROUP but before XACK.
type Reclaimer struct {
	client    *redis.Client
	cfg       ReclaimerConfig
	processor MessageProcessor

	stopCh    chan struct{}
	stoppedCh chan struct{}
}

// MessageProcessor processes a reclaimed message.
type MessageProcessor func(ctx context.Context, msg queue.Message) error

// NewReclaimer creates a new Reclaimer.
func NewReclaimer(client *redis.Client, cfg ReclaimerConfig, processor MessageProcessor) *Reclaimer {
	return &Reclaimer{
		client:    client,
		cfg:       cfg,
		processor: processor,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Run starts the reclaimer loop. Blocks until Stop() is called.
func (r *Reclaimer) Run(ctx context.Context) {
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
func (r *Reclaimer) Stop() {
	close(r.stopCh)
	<-r.stoppedCh
}

// reclaimOnce performs one reclaim cycle.
func (r *Reclaimer) reclaimOnce(ctx context.Context) error {
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
func (r *Reclaimer) reclaimMessage(ctx context.Context, pending redis.XPendingExt) error {
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
	parsed, err := parseRedisMessage(msg)
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

// parseRedisMessage converts a redis.XMessage to queue.Message.
// This duplicates queue.parseMessage but is needed here to avoid exporting it.
func parseRedisMessage(msg redis.XMessage) (queue.Message, error) {
	eventLogID, err := parseInt64(msg.Values, "event_log_id")
	if err != nil {
		return queue.Message{}, err
	}
	issueID, err := parseInt64(msg.Values, "issue_id")
	if err != nil {
		return queue.Message{}, err
	}
	eventType, err := parseString(msg.Values, "event_type")
	if err != nil {
		return queue.Message{}, err
	}
	attempt, err := parseInt(msg.Values, "attempt")
	if err != nil {
		attempt = 1
	}
	traceID, _ := parseString(msg.Values, "trace_id")

	return queue.Message{
		ID:         msg.ID,
		EventLogID: eventLogID,
		IssueID:    issueID,
		EventType:  eventType,
		Attempt:    attempt,
		TraceID:    traceID,
		Raw:        msg,
	}, nil
}

func parseInt64(values map[string]any, key string) (int64, error) {
	raw, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("missing %s", key)
	}
	str := fmt.Sprint(raw)
	var num int64
	_, err := fmt.Sscanf(str, "%d", &num)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}
	return num, nil
}

func parseInt(values map[string]any, key string) (int, error) {
	raw, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("missing %s", key)
	}
	str := fmt.Sprint(raw)
	var num int
	_, err := fmt.Sscanf(str, "%d", &num)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}
	return num, nil
}

func parseString(values map[string]any, key string) (string, error) {
	raw, ok := values[key]
	if !ok {
		return "", fmt.Errorf("missing %s", key)
	}
	return fmt.Sprint(raw), nil
}
