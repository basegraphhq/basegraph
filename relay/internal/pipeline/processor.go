package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"basegraph.app/relay/internal/domain"
	"basegraph.app/relay/internal/service"
)

// Processor orchestrates the event processing pipeline
type Processor struct {
	redisClient *redis.Client
	services    *service.Services
	config      PipelineConfig
	stopCh      chan struct{}
}

// PipelineConfig holds configuration for the pipeline
type PipelineConfig struct {
	RedisURL        string
	RedisStream     string
	ConsumerGroup   string
	ConsumerName    string
	DLQStream       string
	TraceHeaderName string
}

// NewProcessor creates a new pipeline processor
func NewProcessor(redisClient *redis.Client, services *service.Services, config PipelineConfig) *Processor {
	return &Processor{
		redisClient: redisClient,
		services:    services,
		config:      config,
		stopCh:      make(chan struct{}),
	}
}

// Start begins processing events from Redis streams
func (p *Processor) Start(ctx context.Context) error {
	slog.InfoContext(ctx, "starting pipeline processor", "stream", p.config.RedisStream, "group", p.config.ConsumerGroup)

	// Create consumer group if it doesn't exist
	if err := p.createConsumerGroup(ctx); err != nil {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.stopCh:
			return nil
		default:
			if err := p.processBatch(ctx); err != nil {
				slog.ErrorContext(ctx, "failed to process batch", "error", err)
			}
		}
	}
}

// Stop gracefully stops the processor
func (p *Processor) Stop() {
	close(p.stopCh)
}

func (p *Processor) createConsumerGroup(ctx context.Context) error {
	// Try to create the consumer group
	err := p.redisClient.XGroupCreate(ctx, p.config.RedisStream, p.config.ConsumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}
	return nil
}

func (p *Processor) processBatch(ctx context.Context) error {
	// Read from Redis stream
	args := redis.XReadGroupArgs{
		Group:    p.config.ConsumerGroup,
		Consumer: p.config.ConsumerName,
		Streams:  []string{p.config.RedisStream, ">"},
		Count:    10,
		Block:    5 * time.Second,
	}

	streams, err := p.redisClient.XReadGroup(ctx, args).Result()
	if err != nil {
		if err == redis.Nil {
			return nil // No messages
		}
		return fmt.Errorf("failed to read from stream: %w", err)
	}

	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := p.processMessage(ctx, message); err != nil {
				slog.ErrorContext(ctx, "failed to process message", "error", err, "message_id", message.ID)
				p.moveToDLQ(ctx, message, err)
			}

			// Acknowledge message
			if err := p.redisClient.XAck(ctx, p.config.RedisStream, p.config.ConsumerGroup, message.ID).Err(); err != nil {
				slog.ErrorContext(ctx, "failed to acknowledge message", "error", err, "message_id", message.ID)
			}
		}
	}

	return nil
}

func (p *Processor) processMessage(ctx context.Context, message redis.XMessage) error {
	eventType, ok := message.Values["event_type"].(string)
	if !ok {
		return fmt.Errorf("missing event_type in message")
	}

	payload, ok := message.Values["payload"].(string)
	if !ok {
		return fmt.Errorf("missing payload in message")
	}

	var event domain.Event
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	slog.InfoContext(ctx, "processing event", "type", eventType, "issue_id", event.IssueID)

	// Process based on event type
	switch event.Type {
	case domain.EventTypeIssueCreated:
		return p.processIssueCreated(ctx, &event)
	case domain.EventTypeReply:
		return p.processReply(ctx, &event)
	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}
}

func (p *Processor) processIssueCreated(ctx context.Context, event *domain.Event) error {
	// For now, we'll just log it. The actual implementation would call the pipeline service
	slog.InfoContext(ctx, "processing issue created event", "issue_id", event.IssueID)
	return nil
}

func (p *Processor) processReply(ctx context.Context, event *domain.Event) error {
	// For now, we'll just log it. The actual implementation would call the pipeline service
	slog.InfoContext(ctx, "processing reply event", "issue_id", event.IssueID)
	return nil
}

func (p *Processor) moveToDLQ(ctx context.Context, message redis.XMessage, processingErr error) {
	dlqPayload := map[string]interface{}{
		"original_message": message,
		"error":            processingErr.Error(),
		"timestamp":        time.Now().Unix(),
	}

	if err := p.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: p.config.DLQStream,
		Values: dlqPayload,
	}).Err(); err != nil {
		slog.ErrorContext(ctx, "failed to move message to DLQ", "error", err, "message_id", message.ID)
	}
}
