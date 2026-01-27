package queue

import (
	"context"
	"fmt"
	"log/slog"

	"basegraph.co/relay/common/logger"
	"github.com/redis/go-redis/v9"
)

type Producer interface {
	Enqueue(ctx context.Context, task Task) error
	Close() error
}

type StreamResolver func(task Task) (string, error)

type redisProducer struct {
	client  *redis.Client
	stream  string
	resolve StreamResolver
}

func NewRedisProducer(client *redis.Client, stream string) Producer {
	return &redisProducer{
		client: client,
		stream: stream,
	}
}

func NewRedisProducerWithResolver(client *redis.Client, resolver StreamResolver) Producer {
	return &redisProducer{
		client:  client,
		resolve: resolver,
	}
}

func (p *redisProducer) Enqueue(ctx context.Context, task Task) error {
	var issueID *int64
	var eventLogID *int64
	if task.IssueID != 0 {
		issueID = &task.IssueID
	}
	if task.EventLogID != 0 {
		eventLogID = &task.EventLogID
	}

	ctx = logger.WithLogFields(ctx, logger.LogFields{
		IssueID:    issueID,
		EventLogID: eventLogID,
		Component:  "relay.queue.producer",
	})

	attempt := task.Attempt
	if attempt <= 0 {
		attempt = 1
	}

	taskType := task.TaskType
	if taskType == "" {
		taskType = TaskTypeIssueEvent
	}

	fields := map[string]any{
		"task_type": string(taskType),
		"attempt":   attempt,
	}

	if taskType == TaskTypeIssueEvent {
		fields["event_log_id"] = task.EventLogID
		fields["issue_id"] = task.IssueID
		fields["event_type"] = task.EventType
	}

	if task.WorkspaceID != nil {
		fields["workspace_id"] = *task.WorkspaceID
	}
	if task.OrganizationID != nil {
		fields["organization_id"] = *task.OrganizationID
	}
	if task.RunID != nil {
		fields["run_id"] = *task.RunID
	}
	if task.RepoID != nil {
		fields["repo_id"] = *task.RepoID
	}
	if task.Branch != "" {
		fields["branch"] = task.Branch
	}

	traceIDStr := ""
	if task.TraceID != nil && *task.TraceID != "" {
		fields["trace_id"] = *task.TraceID
		traceIDStr = *task.TraceID
	}

	if task.TriggerThreadID != "" {
		fields["trigger_thread_id"] = task.TriggerThreadID
	}

	stream, err := p.resolveStream(task)
	if err != nil {
		return err
	}

	// TODO - @nithinsj - Add MAXLEN to prevent stream growing unbounded. Redis streams grow until out of memory.
	// Consider XTRIM periodically or MAXLEN ~ with XAdd to cap at ~1M entries.
	if err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: fields,
	}).Err(); err != nil {
		return fmt.Errorf("enqueue task (stream=%s): %w", stream, err)
	}

	slog.InfoContext(ctx, "enqueued task",
		"task_type", taskType,
		"event_type", task.EventType,
		"attempt", attempt,
		"trace_id", traceIDStr,
		"stream", stream)
	return nil
}

func (p *redisProducer) resolveStream(task Task) (string, error) {
	if p.resolve != nil {
		stream, err := p.resolve(task)
		if err != nil {
			return "", err
		}
		if stream == "" {
			return "", fmt.Errorf("resolved stream is empty")
		}
		return stream, nil
	}

	if p.stream == "" {
		return "", fmt.Errorf("stream not configured")
	}

	return p.stream, nil
}

func (p *redisProducer) Close() error {
	return p.client.Close()
}
