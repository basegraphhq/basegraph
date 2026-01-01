# Worker Logging & Tracing - Handoff Document

## Overview

This document describes the implementation of contextual, structured logging throughout the worker pipeline. The goal is full traceability of every engagement from webhook ingestion through completion.

## What Was Implemented (Phases 1-3)

### Phase 1: Context Enrichment Foundation

Created infrastructure for automatic context-based logging:

#### New Files

1. **`common/logger/context.go`**
   - `LogFields` struct containing: `IssueID`, `EventLogID`, `MessageID`, `IntegrationID`, `WorkspaceID`, `Component`
   - `WithLogFields(ctx, fields)` - enriches context with fields (merges with existing)
   - `GetLogFields(ctx)` - retrieves fields from context
   - `Ptr[T](v)` - helper to create pointers for inline field setting

2. **`common/logger/span.go`**
   - `SpanContext` - wrapper for OTel span lifecycle management
   - `StartSpan(ctx, name, opts...)` - creates new span
   - `StartSpanFromTraceID(ctx, traceID, name, opts...)` - recreates trace context from string (for Redis propagation)
   - Methods: `Context()`, `End()`, `RecordError(err)`, `Span()`

#### Modified Files

1. **`common/logger/logger.go`**
   - Enhanced `TraceHandler.Handle()` to extract `LogFields` from context
   - Automatically adds all non-nil fields to every log record
   - Works with existing OTel trace_id/span_id extraction

### Phase 2: Trace Propagation API → Redis → Worker

#### Webhook Handler (`internal/http/handler/webhook/gitlab.go`)
- Extracts trace_id from OTel span (set by otelgin middleware)
- Passes trace_id to `EventIngestParams`
- Enriches context with `Component: "relay.http.webhook.gitlab"` and `IntegrationID`

#### Event Ingest Service (`internal/service/event_ingest.go`)
- Enriches context with `IntegrationID` and `Component` at entry
- Adds `WorkspaceID` after integration lookup
- All subsequent logs automatically include these fields

#### Queue Producer (`internal/queue/producer.go`)
- Enriches context with `IssueID`, `EventLogID`, `Component`
- Logs on successful enqueue (INFO level)
- Returns enriched errors (no logging on error - caller handles)

#### Queue Consumer (`internal/queue/consumer.go`)
- Enriches context with `Component` in `Read()`
- Logging pattern:
  - `Read()` with messages: DEBUG level
  - `Ack()` success: DEBUG level
  - `Requeue()` success: INFO level (business event)
  - `SendDLQ()` success: ERROR level (terminal failure)
- Returns enriched errors (caller logs failures)

#### Worker Main (`cmd/worker/main.go`)
- `createMessageContext()` - creates span from trace_id, enriches context with message metadata
- `processMessageSafe()` - manages span lifecycle, logs duration, handles panics
- `handleFailure()` - detailed logging of failure decisions (requeue vs DLQ)
- `runLoop()` - enriched with component, logs loop lifecycle

### Phase 3: Component-Level Context Enrichment

#### Worker Reclaimer (`internal/worker/reclaimer.go`)
- Enriches context with `Component: "relay.worker.reclaimer"`
- Per-message context enrichment with `MessageID`, `IssueID`, `EventLogID`
- Logs reclaim cycle start/end, stale message detection, processing duration

#### Brain Orchestrator (`internal/brain/orchestrator_impl.go`)
- Enriches context with `IssueID`, `EventLogID`, `Component`
- Adds `IntegrationID` after issue lookup
- Logs engagement lifecycle with relevant context

#### Brain Planner (`internal/brain/planner.go`)
- Enriches context with `IssueID`, `Component`
- Logs iteration lifecycle with timing
- Logs explorer spawn/completion with query and duration
- Aggregates and logs token usage across all iterations (Phase 5)
  - `total_prompt_tokens`, `total_completion_tokens`, `total_tokens`
  - Per-iteration token counts at DEBUG level

#### Brain Explorer (`internal/brain/explore_agent.go`)
- Enriches context with `Component: "relay.brain.explorer"`
- Logs exploration lifecycle with timing and report metrics
- Aggregates and logs token usage across all iterations (Phase 5)
  - `total_prompt_tokens`, `total_completion_tokens`, `total_tokens`

## Logging Philosophy

### Log Levels

| Level | Usage |
|-------|-------|
| DEBUG | Infrastructure operations (Redis read/ack, polling cycles, iterations) |
| INFO | Business events (message processing, engagement handling, actions submitted) |
| WARN | Recoverable issues (ack failures, exploration failures) |
| ERROR | Terminal failures (DLQ sends, panics, fatal errors) |

### Error Handling Pattern

- **Don't log errors before returning them** - caller handles logging
- **Do log on successful operations** that are significant (business events)
- **Enrich errors with context** in the error message itself
- **Full error chains** logged at the point of handling (e.g., `handleFailure()`)

### Component Naming (OTel Convention)

```
relay.http.webhook.gitlab
relay.service.event_ingest
relay.queue.producer
relay.queue.consumer
relay.worker.loop
relay.worker.processor
relay.worker.reclaimer
relay.brain.orchestrator
relay.brain.planner
relay.brain.explorer
```

## What Remains (Phases 4-6)

### Phase 4: Duration Tracking
✅ **COMPLETED** - All duration tracking is implemented. Remaining:
- Action execution timing in orchestrator (blocked - actions not yet implemented)
- Individual action timing (blocked - actions not yet implemented)

### Phase 5: Additional Logging Points
✅ **COMPLETED**
- ✅ LLM call timing - Already logged at DEBUG level in `common/llm/llm.go:130-135`
- ✅ Token usage logging - Added aggregated token tracking in Planner and ExploreAgent
  - Planner logs: `total_prompt_tokens`, `total_completion_tokens`, `total_tokens`
  - ExploreAgent logs: `total_prompt_tokens`, `total_completion_tokens`, `total_tokens`
  - Per-iteration token counts logged at DEBUG level
- ⏸️ Rate limit logging - Not implemented (OpenAI Go SDK doesn't expose rate limit headers)
  - Can be added later if needed by wrapping the HTTP client

### Phase 6: Metrics (Optional - Low Priority)
Could add OTel metrics:
- Message processing duration histogram
- Messages processed counter
- DLQ counter
- Planner iteration counter

## Example: Full Trace Output

With current implementation, a successful engagement shows:

```log
[INFO] enqueued event log
  issue_id=789, event_log_id=456, event_type=comment_created, trace_id=abc123, component=relay.queue.producer

[DEBUG] read messages from stream
  count=1, stream=relay_events, component=relay.queue.consumer

[INFO] processing message
  issue_id=789, event_log_id=456, message_id=1-0, event_type=comment_created, attempt=1, trace_id=abc123, component=relay.worker.processor

[INFO] handling engagement
  issue_id=789, event_log_id=456, event_type=comment_created, trace_id=abc123, component=relay.brain.orchestrator

[INFO] issue loaded
  issue_id=789, event_log_id=456, integration_id=123, external_issue_id=42, title="Add feature X", trace_id=abc123, component=relay.brain.orchestrator

[INFO] planner starting
  issue_id=789, trace_id=abc123, component=relay.brain.planner

[DEBUG] planner iteration starting
  issue_id=789, iteration=1, trace_id=abc123, component=relay.brain.planner

[INFO] planner spawning explore agent
  issue_id=789, query="payment processing...", trace_id=abc123, component=relay.brain.planner

[INFO] explore agent completed
  issue_id=789, iterations=5, report_length=2500, duration_ms=3200, total_prompt_tokens=4500, total_completion_tokens=1800, total_tokens=6300, trace_id=abc123, component=relay.brain.explorer

[INFO] planner completed
  issue_id=789, iterations=3, total_duration_ms=8100, total_prompt_tokens=8200, total_completion_tokens=3100, total_tokens=11300, trace_id=abc123, component=relay.brain.planner

[INFO] planner submitted actions
  issue_id=789, iterations=3, action_count=2, total_duration_ms=8100, trace_id=abc123, component=relay.brain.planner

[DEBUG] message acknowledged
  issue_id=789, event_log_id=456, message_id=1-0, stream=relay_events, trace_id=abc123, component=relay.queue.consumer

[INFO] message processed successfully
  issue_id=789, event_log_id=456, message_id=1-0, duration_ms=9250, trace_id=abc123, component=relay.worker.processor
```

## Testing

All existing tests pass:
- `make format` - clean
- `make lint` - clean
- `make test-unit` - all pass

## Files Changed

### Phase 1-3 (Initial Implementation)
```
common/logger/context.go      (NEW)
common/logger/span.go         (NEW)
common/logger/logger.go       (MODIFIED)
internal/http/handler/webhook/gitlab.go (MODIFIED)
internal/service/event_ingest.go (MODIFIED)
internal/queue/producer.go    (MODIFIED)
internal/queue/consumer.go    (MODIFIED)
cmd/worker/main.go            (MODIFIED)
internal/worker/reclaimer.go  (MODIFIED)
internal/brain/orchestrator_impl.go (MODIFIED)
internal/brain/planner.go     (MODIFIED)
internal/brain/explore_agent.go (MODIFIED)
```

### Code Review Fixes
```
common/logger/context.go              - Added Truncate helper, EventType field
common/logger/logger.go               - Auto-log EventType from context
internal/http/handler/webhook/gitlab.go    - DEBUG level, removed redundant fields
internal/http/handler/webhook/gitlab_test.go - Enable DEBUG logging
internal/service/event_ingest.go      - Removed redundant fields
internal/brain/orchestrator_impl.go   - InfoContext, EventType enrichment
internal/brain/planner.go             - WarnContext/InfoContext, use logger.Truncate
internal/brain/explore_agent.go       - WarnContext/InfoContext, use logger.Truncate
internal/worker/reclaimer.go          - Use consumer.Ack
cmd/worker/main.go                    - Fixed span pattern, EventType enrichment
```

### Phase 5 (Token Usage Logging)
```
internal/brain/planner.go       - Added aggregated token usage tracking
internal/brain/explore_agent.go - Added aggregated token usage tracking
```

## Usage Guide

### Enriching Context

```go
ctx = logger.WithLogFields(ctx, logger.LogFields{
    IssueID:   &issueID,
    Component: "relay.my.component",
})

// All subsequent logs automatically include issue_id and component
slog.InfoContext(ctx, "doing something", "extra_field", value)
```

### Creating Spans for Trace Propagation

```go
// From trace ID string (e.g., from Redis message)
sc := logger.StartSpanFromTraceID(ctx, msg.TraceID, "worker.process")
defer sc.End()
ctx = sc.Context()

// Regular span
sc := logger.StartSpan(ctx, "my.operation")
defer sc.End()
ctx = sc.Context()
```

### Error Handling

```go
// DON'T log and return
if err != nil {
    slog.ErrorContext(ctx, "failed", "error", err)  // NO!
    return fmt.Errorf("doing X: %w", err)
}

// DO return enriched error, let caller log
if err != nil {
    return fmt.Errorf("doing X (id=%d): %w", id, err)
}

// DO log business events on success
slog.InfoContext(ctx, "operation completed", "duration_ms", elapsed)
return nil
```
