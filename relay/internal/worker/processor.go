package worker

import (
	"context"

	"basegraph.app/relay/internal/model"
)

// IssueProcessor defines the interface for processing an issue's events.
// Implementations can plug in different processing strategies (LLM, rule-based, etc.)
type IssueProcessor interface {
	// Process handles all events for an issue in a batch.
	//
	// Parameters:
	//   - ctx: Context for cancellation and tracing
	//   - issue: The claimed issue with current state
	//   - events: All unprocessed events, ordered by created_at ASC
	//
	// Returns:
	//   - *model.Issue: The updated issue (will be saved via Upsert), or nil if no changes
	//   - error: Processing error (events will be marked as failed)
	//
	// Contract:
	//   - This method is called within a database transaction
	//   - The issue has already been claimed (status='processing')
	//   - All events belong to this issue and have processed_at=NULL
	//   - If error is returned, events are marked failed but issue returns to 'idle'
	Process(ctx context.Context, issue *model.Issue, events []model.EventLog) (*model.Issue, error)
}
