package queue

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.app/relay/common/logger"
	"github.com/redis/go-redis/v9"
)

type EventMessage struct {
	EventLogID int64
	IssueID    int64
	EventType  string
	TraceID    *string
	Attempt    int
}

type Producer interface {
	Enqueue(ctx context.Context, msg EventMessage) error
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

func (p *redisProducer) Enqueue(ctx context.Context, msg EventMessage) error {
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		IssueID:    &msg.IssueID,
		EventLogID: &msg.EventLogID,
		Component:  "relay.queue.producer",
	})

	attempt := msg.Attempt
	if attempt <= 0 {
		attempt = 1
	}

	fields := map[string]any{
		"event_log_id": msg.EventLogID,
		"issue_id":     msg.IssueID,
		"event_type":   msg.EventType,
		"attempt":      attempt,
	}

	traceIDStr := ""
	if msg.TraceID != nil && *msg.TraceID != "" {
		fields["trace_id"] = *msg.TraceID
		traceIDStr = *msg.TraceID
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
		"event_type", msg.EventType,
		"attempt", attempt,
		"trace_id", traceIDStr,
		"stream", p.stream)
	return nil
}

func (p *redisProducer) Close() error {
	return p.client.Close()
}
