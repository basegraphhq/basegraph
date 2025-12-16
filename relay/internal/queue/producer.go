package queue

import (
	"context"
	"fmt"
	"log/slog"

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
	logger *slog.Logger
}

func NewRedisProducer(client *redis.Client, stream string, logger *slog.Logger) Producer {
	if logger == nil {
		logger = slog.Default()
	}
	return &redisProducer{
		client: client,
		stream: stream,
		logger: logger,
	}
}

func (p *redisProducer) Enqueue(ctx context.Context, msg EventMessage) error {
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

	if msg.TraceID != nil && *msg.TraceID != "" {
		fields["trace_id"] = *msg.TraceID
	}

	if err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: fields,
	}).Err(); err != nil {
		return fmt.Errorf("enqueue event: %w", err)
	}

	p.logger.InfoContext(ctx, "enqueued event log", "event_log_id", msg.EventLogID, "issue_id", msg.IssueID, "event_type", msg.EventType, "attempt", attempt)
	return nil
}

func (p *redisProducer) Close() error {
	return p.client.Close()
}
