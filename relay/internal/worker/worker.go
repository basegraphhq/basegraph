package worker

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/queue"
	"basegraph.app/relay/internal/store"
)

// Safety valve for chatty issues. Prevents runaway processing when new events
// keep arriving during LLM calls. Remaining events are picked up when user re-engages
// (QueueIfIdle resets stuck issues after 15 min timeout).
const maxProcessingIterations = 5

// Mirrors service.StoreProvider - defined here to avoid import cycles. Refer to txrunner.go for more deets
type StoreProvider interface {
	Issues() store.IssueStore
	EventLogs() store.EventLogStore
	LLMEvals() store.LLMEvalStore
}

type TxRunner interface {
	WithTx(ctx context.Context, fn func(stores StoreProvider) error) error
}

type Config struct {
	MaxAttempts int
}

type Worker struct {
	consumer  Consumer
	txRunner  TxRunner
	issues    store.IssueStore
	processor IssueProcessor
	cfg       Config

	stopCh    chan struct{}
	stoppedCh chan struct{}
}

func New(consumer Consumer, txRunner TxRunner, issues store.IssueStore, processor IssueProcessor, cfg Config) *Worker {
	return &Worker{
		consumer:  consumer,
		txRunner:  txRunner,
		issues:    issues,
		processor: processor,
		cfg:       cfg,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

func (w *Worker) Run(ctx context.Context) error {
	defer close(w.stoppedCh)

	slog.InfoContext(ctx, "relay-worker started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.stopCh:
			slog.InfoContext(ctx, "relay-worker stopping")
			return nil
		default:
			if err := w.processOneBatch(ctx); err != nil {
				slog.ErrorContext(ctx, "batch processing error", "error", err)
				// Brief backoff on error
				time.Sleep(time.Second)
			}
		}
	}
}

func (w *Worker) Stop() {
	close(w.stopCh)
	<-w.stoppedCh
}

func (w *Worker) processOneBatch(ctx context.Context) error {
	messages, err := w.consumer.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading from stream: %w", err)
	}

	for _, msg := range messages {
		if err := w.processMessageSafe(ctx, msg); err != nil {
			slog.ErrorContext(ctx, "message processing failed",
				"error", err,
				"message_id", msg.ID,
				"issue_id", msg.IssueID)
			w.handleFailedMessage(ctx, msg, err)
		}
	}

	return nil
}

func (w *Worker) processMessageSafe(ctx context.Context, msg queue.Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "panic recovered in message processing",
				"panic", r,
				"stack", string(debug.Stack()),
				"message_id", msg.ID,
				"issue_id", msg.IssueID)
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return w.ProcessMessage(ctx, msg)
}

// Exported so it can be reused by the reclaimer.
func (w *Worker) ProcessMessage(ctx context.Context, msg queue.Message) error {
	slog.InfoContext(ctx, "processing message",
		"message_id", msg.ID,
		"issue_id", msg.IssueID,
		"event_log_id", msg.EventLogID,
		"attempt", msg.Attempt)

	// TX1: Claim issue and get initial events
	// This is quick - just DB operations, no LLM calls
	var issue *model.Issue
	var events []model.EventLog
	var claimed bool

	tx1Err := w.txRunner.WithTx(ctx, func(sp StoreProvider) error {
		var err error
		claimed, issue, err = sp.Issues().ClaimQueued(ctx, msg.IssueID)
		if err != nil {
			return fmt.Errorf("claiming issue: %w", err)
		}

		if !claimed {
			return nil // Not an error - issue already claimed or not queued
		}

		events, err = sp.EventLogs().ListUnprocessedByIssue(ctx, msg.IssueID)
		if err != nil {
			return fmt.Errorf("listing unprocessed events: %w", err)
		}

		return nil
	})

	if tx1Err != nil {
		return fmt.Errorf("TX1 (claim) failed: %w", tx1Err)
	}

	if !claimed {
		// Only ACK if issue is idle (already processed) or doesn't exist.
		// Don't ACK if still 'processing' - another worker may have it which is taking more time for exploration and context gathering.
		currentIssue, err := w.issues.GetByID(ctx, msg.IssueID)
		if err != nil && err != store.ErrNotFound {
			slog.WarnContext(ctx, "failed to check issue status", "error", err, "issue_id", msg.IssueID)
			return nil // Leave message pending
		}

		if currentIssue == nil || currentIssue.ProcessingStatus == model.ProcessingStatusIdle {
			slog.InfoContext(ctx, "issue already processed, acknowledging", "issue_id", msg.IssueID)
			if err := w.consumer.Ack(ctx, msg); err != nil {
				slog.WarnContext(ctx, "failed to ACK message", "error", err, "message_id", msg.ID)
			}
		} else {
			slog.InfoContext(ctx, "issue still in progress, leaving message pending",
				"issue_id", msg.IssueID,
				"status", currentIssue.ProcessingStatus)
		}
		return nil
	}

	// Process loop: continue until no more events to ensure that we come back for all the replies at once, rather than processing for each reply.
	// LLM calls happen OUTSIDE transactions to avoid holding DB connections
	totalProcessed := 0
	var processingErr error
	var iteration int

	for iteration = 0; iteration < maxProcessingIterations; iteration++ {
		if len(events) == 0 {
			// No events to process, safe to release the issue
			if err := w.releaseIssue(ctx, msg.IssueID); err != nil {
				return fmt.Errorf("releasing issue with no events: %w", err)
			}
			break
		}

		slog.InfoContext(ctx, "processing events batch",
			"issue_id", msg.IssueID,
			"event_count", len(events),
			"iteration", iteration+1)

		// LLM processing OUTSIDE transaction
		// This can take 30s to 10 minutes - we don't want to hold DB connections
		updatedIssue, err := w.processor.Process(ctx, issue, events)
		if err != nil {
			processingErr = err
			// TODO - @nithinsj : Surface pipeline errors to user via gap detector once implemented.
			// For now, we log and continue. If user re-engages, the new event will
			// fetch full discussion context and trigger a fresh pipeline run.
			slog.ErrorContext(ctx, "pipeline processing failed",
				"error", err,
				"issue_id", msg.IssueID,
				"event_count", len(events))
		} else if updatedIssue != nil {
			issue = updatedIssue
		}

		eventIDs := extractEventIDs(events)
		totalProcessed += len(events)

		// TX2: Save results, mark events processed, check for new events
		var newEvents []model.EventLog
		var hasMoreEvents bool

		tx2Err := w.txRunner.WithTx(ctx, func(sp StoreProvider) error {
			// Save updated issue if there were changes
			if updatedIssue != nil {
				if _, err := sp.Issues().Upsert(ctx, updatedIssue); err != nil {
					return fmt.Errorf("saving updated issue: %w", err)
				}
			}

			if err := sp.EventLogs().MarkBatchProcessed(ctx, eventIDs); err != nil {
				return fmt.Errorf("marking events processed: %w", err)
			}

			// Check for new events that arrived during processing
			var err error
			newEvents, err = sp.EventLogs().ListUnprocessedByIssue(ctx, msg.IssueID)
			if err != nil {
				return fmt.Errorf("checking for new events: %w", err)
			}

			hasMoreEvents = len(newEvents) > 0
			if !hasMoreEvents {
				// No more events - set issue back to idle
				if err := sp.Issues().SetIdle(ctx, msg.IssueID); err != nil {
					return fmt.Errorf("setting issue idle: %w", err)
				}
			}

			return nil
		})

		if tx2Err != nil {
			// TX2 failed - issue remains in 'processing' state
			// QueueIfIdle will reset to 'queued' after 15 min when user re-engages
			return fmt.Errorf("TX2 (save) failed: %w", tx2Err)
		}

		if !hasMoreEvents {
			break
		}

		// New events arrived during processing - continue the loop
		events = newEvents
		slog.InfoContext(ctx, "new events arrived during processing, continuing",
			"issue_id", msg.IssueID,
			"new_event_count", len(newEvents))
	}

	// Check if we hit max iterations with events still pending
	if iteration >= maxProcessingIterations && len(events) > 0 {
		slog.WarnContext(ctx, "max iterations reached with events still pending",
			"issue_id", msg.IssueID,
			"pending_events", len(events),
			"max_iterations", maxProcessingIterations)
	}

	// ACK the message
	if err := w.consumer.Ack(ctx, msg); err != nil {
		slog.WarnContext(ctx, "failed to ACK message", "error", err, "message_id", msg.ID)
	}

	if totalProcessed > 0 {
		slog.InfoContext(ctx, "successfully processed events",
			"issue_id", msg.IssueID,
			"event_count", totalProcessed)
	} else {
		slog.InfoContext(ctx, "no unprocessed events found",
			"issue_id", msg.IssueID)
	}

	if processingErr != nil {
		slog.WarnContext(ctx, "processing completed with errors",
			"error", processingErr,
			"issue_id", msg.IssueID)
	}

	return nil
}

// releaseIssue sets an issue back to idle when there are no events to process.
func (w *Worker) releaseIssue(ctx context.Context, issueID int64) error {
	return w.txRunner.WithTx(ctx, func(sp StoreProvider) error {
		if err := sp.Issues().SetIdle(ctx, issueID); err != nil {
			return fmt.Errorf("setting issue idle: %w", err)
		}
		return nil
	})
}

func (w *Worker) handleFailedMessage(ctx context.Context, msg queue.Message, err error) {
	if msg.Attempt >= w.cfg.MaxAttempts {
		// Reset issue to idle before DLQ to break infinite loop.
		// Without this: DLQ message sits forever → issue stays 'processing' →
		// user re-engages → QueueIfIdle resets to 'queued' → Redis reclaimer picks up → repeat.
		if resetErr := w.issues.SetIdle(ctx, msg.IssueID); resetErr != nil {
			slog.WarnContext(ctx, "failed to reset issue to idle before DLQ",
				"error", resetErr,
				"issue_id", msg.IssueID)
		}

		slog.ErrorContext(ctx, "max attempts reached, sending to DLQ",
			"message_id", msg.ID,
			"issue_id", msg.IssueID,
			"attempts", msg.Attempt)
		if dlqErr := w.consumer.SendDLQ(ctx, msg, err.Error()); dlqErr != nil {
			slog.ErrorContext(ctx, "failed to send to DLQ", "error", dlqErr)
		}
		return
	}

	slog.WarnContext(ctx, "requeuing failed message",
		"message_id", msg.ID,
		"issue_id", msg.IssueID,
		"attempt", msg.Attempt)
	if requeueErr := w.consumer.Requeue(ctx, msg, err.Error()); requeueErr != nil {
		slog.ErrorContext(ctx, "failed to requeue message", "error", requeueErr)
	}
}

func extractEventIDs(events []model.EventLog) []int64 {
	ids := make([]int64, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	return ids
}
