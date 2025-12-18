package worker

import (
	"context"
	"log/slog"

	"basegraph.app/relay/internal/model"
)

// StubProcessor is a no-op processor for testing and initial deployment.
// It logs events but doesn't modify the issue.
type StubProcessor struct{}

// NewStubProcessor creates a new stub processor.
func NewStubProcessor() *StubProcessor {
	return &StubProcessor{}
}

// Process logs the events and returns the issue unchanged.
func (p *StubProcessor) Process(ctx context.Context, issue *model.Issue, events []model.EventLog) (*model.Issue, error) {
	slog.InfoContext(ctx, "stub processor: processing events",
		"issue_id", issue.ID,
		"event_count", len(events),
		"event_types", eventTypes(events))

	for _, event := range events {
		slog.DebugContext(ctx, "stub processor: event details",
			"event_id", event.ID,
			"event_type", event.EventType,
			"source", event.Source,
			"created_at", event.CreatedAt)
	}

	// Return nil to indicate no changes to the issue
	return nil, nil
}

func eventTypes(events []model.EventLog) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.EventType
	}
	return types
}
