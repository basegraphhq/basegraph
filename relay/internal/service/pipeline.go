package service

import (
	"context"
	"log/slog"

	"basegraph.app/relay/internal/llm"
	"basegraph.app/relay/internal/queue"
	"basegraph.app/relay/internal/store"
)

// Pipeline handles the event processing pipeline (currently empty).
type Pipeline struct {
	stores    PipelineStores
	consumer  *queue.RedisConsumer
	llmClient llm.Client
}

// PipelineStores defines the minimal set of stores required by the pipeline.
// This keeps the pipeline implementation aligned with the main store
// abstractions while allowing focused dependencies for testing.
type PipelineStores interface {
	EventLogs() store.EventLogStore
	Issues() store.IssueStore
	PipelineRuns() store.PipelineRunStore
}

// NewPipeline creates a new pipeline instance.
func NewPipeline(stores PipelineStores, consumer *queue.RedisConsumer, llmClient llm.Client) *Pipeline {
	return &Pipeline{
		stores:    stores,
		consumer:  consumer,
		llmClient: llmClient,
	}
}

// ProcessIssue processes a single issue event (placeholder implementation)
func (p *Pipeline) ProcessIssue(ctx context.Context, eventLogID int64) error {
	slog.InfoContext(ctx, "processing issue event", "event_log_id", eventLogID)
	// TODO: Implement actual pipeline processing
	return nil
}
