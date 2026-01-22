package queue

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.co/relay/common/logger"
	"github.com/redis/go-redis/v9"
)

type Event struct {
	EventLogID      int64
	IssueID         int64
	EventType       string
	TraceID         *string
	Attempt         int
	TriggerThreadID string
}

type Producer interface {
	Enqueue(ctx context.Context, event Event) error
	Close() error
}

type redisProducer struct {
	client *redis.Client
	stream string
}

func NewRedisProducer(client *redis.Client, stream string) Producer {
	return &redisProducer{
		client: client,
		stream: stream,
	}
}

func (p *redisProducer) Enqueue(ctx context.Context, event Event) error {
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		IssueID:    &event.IssueID,
		EventLogID: &event.EventLogID,
		Component:  "relay.queue.producer",
	})

	attempt := event.Attempt
	if attempt <= 0 {
		attempt = 1
	}

	fields := map[string]any{
		"event_log_id": event.EventLogID,
		"issue_id":     event.IssueID,
		"event_type":   event.EventType,
		"attempt":      attempt,
	}

	traceIDStr := ""
	if event.TraceID != nil && *event.TraceID != "" {
		fields["trace_id"] = *event.TraceID
		traceIDStr = *event.TraceID
	}

	if event.TriggerThreadID != "" {
		fields["trigger_thread_id"] = event.TriggerThreadID
	}

	// TODO - @nithinsj - Add MAXLEN to prevent stream growing unbounded. Redis streams grow until out of memory.
	// Consider XTRIM periodically or MAXLEN ~ with XAdd to cap at ~1M entries.
	if err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: fields,
	}).Err(); err != nil {
		return fmt.Errorf("enqueue event (stream=%s): %w", p.stream, err)
	}

	slog.InfoContext(ctx, "enqueued event log",
		"event_type", event.EventType,
		"attempt", attempt,
		"trace_id", traceIDStr,
		"stream", p.stream)
	return nil
}

func (p *redisProducer) Close() error {
	return p.client.Close()
}
