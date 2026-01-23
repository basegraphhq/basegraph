package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"basegraph.co/relay/common/logger"
	"github.com/redis/go-redis/v9"
)

type ConsumerConfig struct {
	Stream       string        // Redis stream name
	Group        string        // Redis consumer group name
	Consumer     string        // Redis consumer name
	DLQStream    string        // Dead letter queue stream for failed messages
	BatchSize    int64         // Number of messages to process per batch
	Block        time.Duration // How long to block/poll for new messages
	MaxAttempts  int           // Maximum retry attempts before moving to DLQ
	RequeueDelay time.Duration // Delay before retrying failed messages
}

type Message struct {
	ID              string
	TaskType        TaskType
	EventLogID      *int64
	IssueID         *int64
	EventType       string
	Attempt         int
	TraceID         string
	TriggerThreadID string
	WorkspaceID     *int64
	OrganizationID  *int64
	RunID           *int64
	RepoID          *int64
	Branch          string
	Raw             redis.XMessage
}

// MessageProcessor processes a queue message.
type MessageProcessor func(ctx context.Context, msg Message) error

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
	// Consumer groups are just readers, messages live in the stream itself.
	// If we recreate the group, we want to see everything that's already there.
	// Starting from "0" instead of "$" means we don't lose messages during restarts.
	if err := c.client.XGroupCreateMkStream(ctx, c.cfg.Stream, c.cfg.Group, "0").Err(); err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("creating consumer group: %w", err)
	}
	return nil
}

func (c *RedisConsumer) Read(ctx context.Context) ([]Message, error) {
	ctx = logger.WithLogFields(ctx, logger.LogFields{
		Component: "relay.queue.consumer",
	})

	streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.cfg.Group,
		Consumer: c.cfg.Consumer,
		// > = New messages not yet delivered to anyone. 0 = this consumer's pending message
		// Unacked messages will be handled by reclaimer which runs on a different goroutine
		Streams: []string{c.cfg.Stream, ">"}, // we'll be reading newer message only from the configured stream 'relay_events'
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
	// XReadGroup supports multiple streams, but we only read one so this outer loop only runs once.
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			parsed, parseErr := ParseMessage(msg)
			if parseErr != nil {
				slog.ErrorContext(ctx, "failed to parse message",
					"error", parseErr,
					"raw_message_id", msg.ID,
					"stream", c.cfg.Stream)
				_ = c.Ack(ctx, Message{ID: msg.ID, Raw: msg})
				continue
			}
			messages = append(messages, parsed)
		}
	}

	if len(messages) > 0 {
		slog.DebugContext(ctx, "read messages from stream",
			"count", len(messages),
			"stream", c.cfg.Stream,
			"consumer", c.cfg.Consumer)
	}

	return messages, nil
}

func (c *RedisConsumer) Ack(ctx context.Context, msg Message) error {
	if err := c.client.XAck(ctx, c.cfg.Stream, c.cfg.Group, msg.ID).Err(); err != nil {
		return fmt.Errorf("xack (stream=%s): %w", c.cfg.Stream, err)
	}

	slog.DebugContext(ctx, "message acknowledged", "stream", c.cfg.Stream)
	return nil
}

func (c *RedisConsumer) Requeue(ctx context.Context, msg Message, errMsg string) error {
	nextAttempt := msg.Attempt + 1
	return c.RequeueWithAttempt(ctx, msg, nextAttempt, errMsg)
}

func (c *RedisConsumer) RequeueWithAttempt(ctx context.Context, msg Message, attempt int, errMsg string) error {
	if attempt <= 0 {
		attempt = msg.Attempt
		if attempt <= 0 {
			attempt = 1
		}
	}

	if err := c.Ack(ctx, msg); err != nil {
		return fmt.Errorf("acking failed message for requeue: %w", err)
	}

	values := messageValues(msg, attempt)
	if errMsg != "" {
		values["last_error"] = errMsg
	}

	if c.cfg.RequeueDelay > 0 {
		time.Sleep(c.cfg.RequeueDelay)
	}

	if err := c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: c.cfg.Stream,
		Values: values,
	}).Err(); err != nil {
		return fmt.Errorf("xadd requeue: %w", err)
	}

	slog.InfoContext(ctx, "message requeued for retry",
		"next_attempt", attempt,
		"reason", errMsg)
	return nil
}

func (c *RedisConsumer) SendDLQ(ctx context.Context, msg Message, errMsg string) error {
	if err := c.Ack(ctx, msg); err != nil {
		return fmt.Errorf("acking failed message for dlq: %w", err)
	}

	values := messageValues(msg, msg.Attempt)
	values["error"] = errMsg

	if err := c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: c.cfg.DLQStream,
		Values: values,
	}).Err(); err != nil {
		return fmt.Errorf("xadd dlq (stream=%s): %w", c.cfg.DLQStream, err)
	}

	slog.ErrorContext(ctx, "message sent to DLQ",
		"final_error", errMsg,
		"dlq_stream", c.cfg.DLQStream)
	return nil
}

func ParseMessage(msg redis.XMessage) (Message, error) {
	eventLogID, err := parseOptionalInt64(msg.Values, "event_log_id")
	if err != nil {
		return Message{}, err
	}
	issueID, err := parseOptionalInt64(msg.Values, "issue_id")
	if err != nil {
		return Message{}, err
	}
	runID, err := parseOptionalInt64(msg.Values, "run_id")
	if err != nil {
		return Message{}, err
	}
	workspaceID, err := parseOptionalInt64(msg.Values, "workspace_id")
	if err != nil {
		return Message{}, err
	}
	organizationID, err := parseOptionalInt64(msg.Values, "organization_id")
	if err != nil {
		return Message{}, err
	}
	repoID, err := parseOptionalInt64(msg.Values, "repo_id")
	if err != nil {
		return Message{}, err
	}

	branch, err := parseOptionalString(msg.Values, "branch")
	if err != nil {
		return Message{}, err
	}
	eventType, err := parseOptionalString(msg.Values, "event_type")
	if err != nil {
		return Message{}, err
	}
	traceID, err := parseOptionalString(msg.Values, "trace_id")
	if err != nil {
		return Message{}, err
	}
	triggerThreadID, err := parseOptionalString(msg.Values, "trigger_thread_id")
	if err != nil {
		return Message{}, err
	}

	attempt, err := parseOptionalInt(msg.Values, "attempt")
	if err != nil {
		return Message{}, err
	}
	if attempt == 0 {
		attempt = 1
	}

	taskTypeStr, err := parseOptionalString(msg.Values, "task_type")
	if err != nil {
		return Message{}, err
	}

	taskType := TaskType(taskTypeStr)
	if taskType == "" {
		if eventLogID != nil && issueID != nil {
			taskType = TaskTypeIssueEvent
		} else {
			return Message{}, fmt.Errorf("missing task_type")
		}
	}

	switch taskType {
	case TaskTypeIssueEvent:
		if eventLogID == nil || issueID == nil {
			return Message{}, fmt.Errorf("missing event_log_id or issue_id")
		}
		if eventType == "" {
			return Message{}, fmt.Errorf("missing event_type")
		}
	case TaskTypeWorkspaceSetup:
		if runID == nil {
			return Message{}, fmt.Errorf("missing run_id")
		}
	case TaskTypeRepoSync:
		if runID == nil || repoID == nil {
			return Message{}, fmt.Errorf("missing run_id or repo_id")
		}
	default:
		return Message{}, fmt.Errorf("unknown task_type %q", taskType)
	}

	return Message{
		ID:              msg.ID,
		TaskType:        taskType,
		EventLogID:      eventLogID,
		IssueID:         issueID,
		EventType:       eventType,
		Attempt:         attempt,
		TraceID:         traceID,
		TriggerThreadID: triggerThreadID,
		WorkspaceID:     workspaceID,
		OrganizationID:  organizationID,
		RunID:           runID,
		RepoID:          repoID,
		Branch:          branch,
		Raw:             msg,
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

func parseOptionalInt64(values map[string]any, key string) (*int64, error) {
	raw, ok := values[key]
	if !ok {
		return nil, nil
	}
	str := fmt.Sprint(raw)
	num, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", key, err)
	}
	return &num, nil
}

func parseOptionalInt(values map[string]any, key string) (int, error) {
	raw, ok := values[key]
	if !ok {
		return 0, nil
	}
	str := fmt.Sprint(raw)
	num, err := strconv.Atoi(str)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}
	return num, nil
}

func parseOptionalString(values map[string]any, key string) (string, error) {
	raw, ok := values[key]
	if !ok {
		return "", nil
	}
	return fmt.Sprint(raw), nil
}

func messageValues(msg Message, attempt int) map[string]any {
	values := map[string]any{
		"task_type": string(msg.TaskType),
		"attempt":   attempt,
	}

	if msg.TaskType == "" {
		values["task_type"] = string(TaskTypeIssueEvent)
	}

	if msg.TaskType == TaskTypeIssueEvent || msg.TaskType == "" {
		if msg.EventLogID != nil {
			values["event_log_id"] = *msg.EventLogID
		}
		if msg.IssueID != nil {
			values["issue_id"] = *msg.IssueID
		}
		if msg.EventType != "" {
			values["event_type"] = msg.EventType
		}
	}

	if msg.WorkspaceID != nil {
		values["workspace_id"] = *msg.WorkspaceID
	}
	if msg.OrganizationID != nil {
		values["organization_id"] = *msg.OrganizationID
	}
	if msg.RunID != nil {
		values["run_id"] = *msg.RunID
	}
	if msg.RepoID != nil {
		values["repo_id"] = *msg.RepoID
	}
	if msg.Branch != "" {
		values["branch"] = msg.Branch
	}

	if msg.TraceID != "" {
		values["trace_id"] = msg.TraceID
	}
	if msg.TriggerThreadID != "" {
		values["trigger_thread_id"] = msg.TriggerThreadID
	}

	return values
}
