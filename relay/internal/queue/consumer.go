package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type ConsumerConfig struct {
	Stream       string
	Group        string
	Consumer     string
	DLQStream    string
	BatchSize    int64
	Block        time.Duration
	MaxAttempts  int
	RequeueDelay time.Duration
}

type Message struct {
	ID         string
	EventLogID int64
	IssueID    int64
	EventType  string
	Attempt    int
	TraceID    string
	Raw        redis.XMessage
}

type RedisConsumer struct {
	client *redis.Client
	cfg    ConsumerConfig
}

func NewRedisConsumer(client *redis.Client, cfg ConsumerConfig) (*RedisConsumer, error) {
	consumer := &RedisConsumer{
		client: client,
		cfg:    cfg,
	}

	if err := consumer.ensureGroup(context.Background()); err != nil { //nolint:contextcheck
		return nil, err
	}

	return consumer, nil
}

func (c *RedisConsumer) ensureGroup(ctx context.Context) error {
	if err := c.client.XGroupCreateMkStream(ctx, c.cfg.Stream, c.cfg.Group, "0").Err(); err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("creating consumer group: %w", err)
	}
	return nil
}

func (c *RedisConsumer) Read(ctx context.Context) ([]Message, error) {
	streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.cfg.Group,
		Consumer: c.cfg.Consumer,
		// > = New messages not yet delivered to anyone. 0 = this consumer's pending message
		Streams: []string{c.cfg.Stream, ">"},
		Count:   c.cfg.BatchSize,
		Block:   c.cfg.Block,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return []Message{}, nil
		}
		return nil, fmt.Errorf("reading from stream: %w", err)
	}

	var messages []Message
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			parsed, parseErr := parseMessage(msg)
			if parseErr != nil {
				slog.ErrorContext(ctx, "failed to parse message", "error", parseErr, "message_id", msg.ID)
				_ = c.Ack(ctx, Message{ID: msg.ID, Raw: msg})
				continue
			}
			messages = append(messages, parsed)
		}
	}
	return messages, nil
}

func (c *RedisConsumer) Ack(ctx context.Context, msg Message) error {
	return c.client.XAck(ctx, c.cfg.Stream, c.cfg.Group, msg.ID).Err()
}

func (c *RedisConsumer) Requeue(ctx context.Context, msg Message, errMsg string) error {
	if err := c.Ack(ctx, msg); err != nil {
		return fmt.Errorf("acking failed message: %w", err)
	}

	nextAttempt := msg.Attempt + 1
	values := map[string]any{
		"event_log_id": msg.EventLogID,
		"issue_id":     msg.IssueID,
		"event_type":   msg.EventType,
		"attempt":      nextAttempt,
	}
	if msg.TraceID != "" {
		values["trace_id"] = msg.TraceID
	}
	if errMsg != "" {
		values["last_error"] = errMsg
	}

	if c.cfg.RequeueDelay > 0 {
		time.Sleep(c.cfg.RequeueDelay)
	}

	return c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: c.cfg.Stream,
		Values: values,
	}).Err()
}

func (c *RedisConsumer) SendDLQ(ctx context.Context, msg Message, errMsg string) error {
	if err := c.Ack(ctx, msg); err != nil {
		return fmt.Errorf("acking failed message for dlq: %w", err)
	}

	values := map[string]any{
		"event_log_id": msg.EventLogID,
		"issue_id":     msg.IssueID,
		"event_type":   msg.EventType,
		"attempt":      msg.Attempt,
		"error":        errMsg,
	}
	if msg.TraceID != "" {
		values["trace_id"] = msg.TraceID
	}

	return c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: c.cfg.DLQStream,
		Values: values,
	}).Err()
}

func parseMessage(msg redis.XMessage) (Message, error) {
	eventLogID, err := parseInt64(msg.Values, "event_log_id")
	if err != nil {
		return Message{}, err
	}
	issueID, err := parseInt64(msg.Values, "issue_id")
	if err != nil {
		return Message{}, err
	}
	eventType, err := parseString(msg.Values, "event_type")
	if err != nil {
		return Message{}, err
	}
	attempt, err := parseInt(msg.Values, "attempt")
	if err != nil {
		attempt = 1
	}
	traceID, _ := parseString(msg.Values, "trace_id")

	return Message{
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
	num, err := strconv.ParseInt(str, 10, 64)
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
	num, err := strconv.Atoi(str)
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
