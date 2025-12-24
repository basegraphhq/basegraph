package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/queue"
	"basegraph.app/relay/internal/store"
)

// Mirrors service.StoreProvider - defined here to avoid import cycles.
type StoreProvider interface {
	Issues() store.IssueStore
	EventLogs() store.EventLogStore
	LLMEvals() store.LLMEvalStore
}

// Mirrors service.TxRunner - defined here to avoid import cycles.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(stores StoreProvider) error) error
}

type Config struct {
	MaxAttempts int
}

type Worker struct {
	consumer  *queue.RedisConsumer
	txRunner  TxRunner
	processor IssueProcessor
	cfg       Config

	stopCh    chan struct{}
	stoppedCh chan struct{}
}

func New(consumer *queue.RedisConsumer, txRunner TxRunner, processor IssueProcessor, cfg Config) *Worker {
	return &Worker{
		consumer:  consumer,
		txRunner:  txRunner,
		processor: processor,
		cfg:       cfg,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

func (w *Worker) Run(ctx context.Context) error {
	defer close(w.stoppedCh)

	slog.InfoContext(ctx, "worker started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.stopCh:
			slog.InfoContext(ctx, "worker stopping")
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

	var processingErr error

	// Single transaction: claim -> process -> complete
	txErr := w.txRunner.WithTx(ctx, func(sp StoreProvider) error {
		// Step 1: Claim the issue
		claimed, issue, err := sp.Issues().ClaimQueued(ctx, msg.IssueID)
		if err != nil {
			return fmt.Errorf("claiming issue: %w", err)
		}

		if !claimed {
			// Issue already claimed or not queued - this is expected
			slog.InfoContext(ctx, "issue not claimable, skipping",
				"issue_id", msg.IssueID)
			return nil // Not an error - just ACK and move on
		}

		// Step 2: Get all unprocessed events
		events, err := sp.EventLogs().ListUnprocessedByIssue(ctx, msg.IssueID)
		if err != nil {
			return fmt.Errorf("listing unprocessed events: %w", err)
		}

		if len(events) == 0 {
			// No events to process (edge case: all already processed)
			slog.InfoContext(ctx, "no unprocessed events found",
				"issue_id", msg.IssueID)
			if err := sp.Issues().SetProcessed(ctx, msg.IssueID); err != nil {
				return fmt.Errorf("setting issue processed: %w", err)
			}
			return nil
		}

		slog.InfoContext(ctx, "processing events batch",
			"issue_id", msg.IssueID,
			"event_count", len(events))

		// Step 3: Process all events
		updatedIssue, err := w.processor.Process(ctx, issue, events, sp)
		if err != nil {
			processingErr = err
			// Mark events as failed but still complete the issue
			eventIDs := extractEventIDs(events)
			if markErr := sp.EventLogs().MarkBatchProcessed(ctx, eventIDs); markErr != nil {
				return fmt.Errorf("marking events processed after failure: %w", markErr)
			}
			if setErr := sp.Issues().SetProcessed(ctx, msg.IssueID); setErr != nil {
				return fmt.Errorf("setting issue processed after failure: %w", setErr)
			}
			return nil // Don't rollback - we want to persist the completion
		}

		// Step 4: Save updated issue if there were changes
		if updatedIssue != nil {
			if _, err := sp.Issues().Upsert(ctx, updatedIssue); err != nil {
				return fmt.Errorf("saving updated issue: %w", err)
			}
		}

		// Step 5: Mark events as processed
		eventIDs := extractEventIDs(events)
		if err := sp.EventLogs().MarkBatchProcessed(ctx, eventIDs); err != nil {
			return fmt.Errorf("marking events processed: %w", err)
		}

		// Step 6: Set issue back to idle
		if err := sp.Issues().SetProcessed(ctx, msg.IssueID); err != nil {
			return fmt.Errorf("setting issue processed: %w", err)
		}

		slog.InfoContext(ctx, "successfully processed events",
			"issue_id", msg.IssueID,
			"event_count", len(events))

		return nil
	})

	// Handle transaction result
	if txErr != nil {
		// Transaction failed - don't ACK, let Redis redeliver
		return fmt.Errorf("transaction failed: %w", txErr)
	}

	// Transaction succeeded - ACK the message
	if err := w.consumer.Ack(ctx, msg); err != nil {
		// Log but don't fail - message will be reclaimed but that's safe
		slog.WarnContext(ctx, "failed to ACK message",
			"error", err,
			"message_id", msg.ID)
	}

	if processingErr != nil {
		slog.WarnContext(ctx, "processing failed but transaction committed",
			"error", processingErr,
			"issue_id", msg.IssueID)
	}

	return nil
}

func (w *Worker) handleFailedMessage(ctx context.Context, msg queue.Message, err error) {
	if msg.Attempt >= w.cfg.MaxAttempts {
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
