# Redis Stream Issue-Centric Processing

This document describes the architecture for processing issue events using Redis Streams with Postgres as the source of truth.

## Overview

**Core Insight**: Events are triggers, but the processing unit is the issue. This mirrors how humans work — look at an issue, see all pending events, decide what to reply.

**Components**:
- **Postgres**: Source of truth for issue state and event logs
- **Redis Stream**: Delivery mechanism with built-in crash recovery (XPENDING/XCLAIM)
- **Workers**: Consumer group processing issues

## Issue States

```
processing_status: 'idle' | 'queued' | 'processing'
```

| State | Meaning |
|-------|---------|
| `idle` | No pending work, not in queue |
| `queued` | Work enqueued, waiting for worker |
| `processing` | Worker actively handling this issue |

## Data Flow

### Webhook Path (< 100ms, within 5-10s SLA)

```
External System
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│ Webhook Handler                                         │
│                                                         │
│  1. Validate + parse                                    │
│  2. BEGIN transaction                                   │
│  3. INSERT INTO event_logs (raw payload, immutable)     │
│  4. UPDATE issues SET status = 'queued'                 │
│     WHERE status IN ('idle', 'queued')                  │
│     (no-op if already 'processing')                     │
│  5. COMMIT                                              │
│  6. XADD issue_work {issue_id, event_log_id}            │
│  7. Return 200 OK                                       │
└─────────────────────────────────────────────────────────┘
```

**Key Points**:
- Webhooks only INSERT to `event_logs` — no direct mutations to issue fields like `discussions`
- Status transition `idle → queued` is idempotent; multiple webhooks for same issue are safe
- If status is `processing`, leave it alone — worker will see new events

### Worker Path

```
Redis Stream (issue_work)
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│ Worker                                                  │
│                                                         │
│  1. XREADGROUP → receive {issue_id, event_log_id}       │
│                                                         │
│  2. BEGIN transaction                                   │
│                                                         │
│  3. CLAIM: UPDATE issues                                │
│            SET status = 'processing',                   │
│                processing_started_at = now()            │
│            WHERE id = ? AND status = 'queued'           │
│            RETURNING *                                  │
│                                                         │
│     → 1 row: claimed, continue                          │
│     → 0 rows: already claimed, ROLLBACK + XACK + exit   │
│                                                         │
│  4. SELECT * FROM event_logs                            │
│     WHERE issue_id = ? AND processed_at IS NULL         │
│     ORDER BY created_at ASC                             │
│                                                         │
│  5. Process ALL events together (LLM, build reply)      │
│                                                         │
│  6. Update issue fields (discussions, spec, etc.)       │
│                                                         │
│  7. UPDATE event_logs SET processed_at = now()          │
│     WHERE id IN (processed_ids)                         │
│                                                         │
│  8. UPDATE issues SET status = 'idle',                  │
│                       last_processed_at = now()         │
│                                                         │
│  9. COMMIT                                              │
│                                                         │
│ 10. XACK                                                │
└─────────────────────────────────────────────────────────┘
```

**Key Points**:
- Single DB transaction wraps claim → process → complete
- Only one worker can claim an issue (DB is the arbiter)
- Worker processes ALL unprocessed events for the issue in one batch
- `discussions` JSONB updated atomically by worker, no concurrent writes

## Concurrency Scenarios

### Two Webhooks, Same Issue

```
Webhook A                          Webhook B
    │                                  │
    ▼                                  ▼
INSERT event_log (event A)        INSERT event_log (event B)
    │                                  │
    ▼                                  ▼
UPDATE status = 'queued'          UPDATE status = 'queued'
WHERE status = 'idle'             WHERE status = 'idle'
→ 1 row (success)                 → 0 rows (already queued)
    │                                  │
    ▼                                  ▼
XADD {issue_id}                   XADD {issue_id}
```

Result: Two Redis messages, but worker claims once and processes both events together.

### Two Workers, Same Message (shouldn't happen, but handled)

```
Worker 1                           Worker 2
    │                                  │
    ▼                                  ▼
XREADGROUP → {issue_id}           XREADGROUP → {issue_id}
    │                                  │
    ▼                                  ▼
UPDATE status = 'processing'      UPDATE status = 'processing'
WHERE status = 'queued'           WHERE status = 'queued'
→ 1 row (claimed) ✓               → 0 rows (skip) ✗
    │                                  │
    ▼                                  ▼
Process events                    XACK + exit (no-op)
    │
    ▼
COMMIT + XACK
```

The conditional UPDATE is the lock — no need for `SELECT FOR UPDATE`.

### Worker Crash Recovery

```
Worker 1                           Worker 2 (Reclaimer)
    │
    ▼
XREADGROUP → message M
    │
    ▼
BEGIN
UPDATE status = 'processing' ✓
Processing...
    │
    💥 CRASH
    │
    ▼
Postgres: auto-rollback            After T seconds...
  (status back to 'queued')            │
                                       ▼
Redis: M still in PEL             XPENDING → finds M idle > T
                                       │
                                       ▼
                                  XCLAIM M → now owns M
                                       │
                                       ▼
                                  UPDATE status = 'processing' ✓
                                  Process events
                                  COMMIT + XACK
```

**Key Points**:
- Crash before COMMIT → Postgres rolls back, status returns to `queued`
- Redis XPENDING/XCLAIM redelivers after timeout
- No Postgres polling job needed for recovery

### Crash After COMMIT, Before XACK

This is the trickiest scenario. Worker 1 successfully commits to Postgres but dies before acknowledging the Redis message.

**State after crash**:
- Postgres: `status = 'idle'`, all events have `processed_at` set
- Redis: Message M still in PEL (pending entry list), will be reclaimed

#### Scenario A: No new events arrive

```
Worker 1                           Worker 2 (Reclaimer)
    │
    ▼
BEGIN
Process events A, B
UPDATE event_logs SET processed_at = now()
UPDATE issues SET status = 'idle'
COMMIT ✓
    │
    💥 CRASH (before XACK)
                                   After T seconds...
                                       │
                                       ▼
                                   XCLAIM M → now owns message M
                                       │
                                       ▼
                                   BEGIN
                                   UPDATE issues SET status = 'processing'
                                   WHERE id = 123 AND status = 'queued'
                                   → 0 rows returned (status is 'idle')
                                       │
                                       ▼
                                   ROLLBACK (nothing to do)
                                   XACK M
                                   Exit
```

**Why this is safe**: The claim query requires `status = 'queued'`. Since Worker 1 set it to `idle`, the claim fails. No duplicate processing.

#### Scenario B: New events arrive before reclaim

```
Worker 1                     Webhook C                    Worker 2 (Reclaimer)
    │
    ▼
COMMIT ✓ (status = 'idle')
    │
    💥 CRASH
                                  │
                                  ▼
                             INSERT event_log (event C)
                             UPDATE status = 'queued' ✓
                             XADD message N
                                                              │
                                                              ▼
                                                         XCLAIM M (the OLD message)
                                                              │
                                                              ▼
                                                         UPDATE status = 'processing'
                                                         WHERE status = 'queued'
                                                         → 1 row ✓ (webhook C set it)
                                                              │
                                                              ▼
                                                         SELECT * FROM event_logs
                                                         WHERE processed_at IS NULL
                                                         → returns event C only
                                                           (A, B already processed)
                                                              │
                                                              ▼
                                                         Process event C
                                                         COMMIT (status = 'idle')
                                                         XACK M
```

**What about message N?** It's still in the stream. When a worker picks it up:

```
Worker 3
    │
    ▼
XREADGROUP → message N
    │
    ▼
UPDATE status = 'processing'
WHERE status = 'queued'
→ 0 rows (status is 'idle', Worker 2 finished)
    │
    ▼
XACK N (no-op, work already done)
```

**Why this is safe**:
1. Events A, B were processed by Worker 1 (committed, `processed_at` set)
2. Event C was processed by Worker 2 (even though it reclaimed the OLD message M)
3. Message N becomes a no-op because the issue is already `idle`
4. No duplicates: `processed_at IS NULL` filter prevents reprocessing

#### Scenario C: Reclaim happens before new events

```
Worker 1                     Worker 2 (Reclaimer)         Webhook C
    │
    ▼
COMMIT ✓ (status = 'idle')
    │
    💥 CRASH
                                  │
                                  ▼
                             XCLAIM M
                             UPDATE WHERE status = 'queued'
                             → 0 rows (idle)
                             XACK M
                                                              │
                                                              ▼
                                                         INSERT event_log (event C)
                                                         UPDATE status = 'queued' ✓
                                                         XADD message N
                                                              │
                                                              ▼
                                                         Worker 3 picks up N
                                                         Processes event C normally
```

**Why this is safe**: Message M is cleaned up as a no-op. Event C gets its own message N and is processed normally.

#### Key Insight: Events, Not Messages

The system is **event-centric in storage, issue-centric in processing**:

- Redis messages are just "nudges" to check an issue
- The real work list is `event_logs WHERE processed_at IS NULL`
- Multiple messages for the same issue collapse into one processing run
- The `processed_at` column is the idempotency key

This means:
- **Lost messages** → Issue stays `queued`, next message picks it up
- **Duplicate messages** → Claim fails or no unprocessed events, becomes no-op
- **Out-of-order messages** → Doesn't matter, worker always fetches ALL unprocessed events

## Schema Changes

### New Columns on `issues`

```sql
ALTER TABLE issues
ADD COLUMN processing_started_at timestamptz;
```

### Add CHECK Constraint

```sql
ALTER TABLE issues
ADD CONSTRAINT chk_issues_processing_status
CHECK (processing_status IN ('idle', 'queued', 'processing'));
```

### Add Composite Index

```sql
CREATE INDEX idx_issues_status_id 
ON issues (processing_status, id) 
WHERE processing_status != 'idle';
```

This supports queries like "give me N queued issues" for monitoring/admin.

## Store Interface Changes

### IssueStore

```go
type IssueStore interface {
    // Existing
    Upsert(ctx context.Context, issue *model.Issue) (*model.Issue, error)
    GetByID(ctx context.Context, id int64) (*model.Issue, error)
    GetByIntegrationAndExternalID(ctx context.Context, integrationID int64, externalIssueID string) (*model.Issue, error)
    
    // New: Atomic state transitions
    QueueIfIdle(ctx context.Context, issueID int64) (queued bool, err error)
    ClaimQueued(ctx context.Context, issueID int64) (claimed bool, issue *model.Issue, err error)
    SetProcessed(ctx context.Context, issueID int64) error
}
```

### EventLogStore

```go
type EventLogStore interface {
    // Existing
    Create(ctx context.Context, log *model.EventLog) (*model.EventLog, error)
    CreateOrGet(ctx context.Context, log *model.EventLog) (*model.EventLog, bool, error)
    GetByID(ctx context.Context, id int64) (*model.EventLog, error)
    MarkProcessed(ctx context.Context, id int64) error
    MarkFailed(ctx context.Context, id int64, errMsg string) error
    
    // New: Issue-centric queries
    ListUnprocessedByIssue(ctx context.Context, issueID int64) ([]model.EventLog, error)
    MarkBatchProcessed(ctx context.Context, ids []int64) error
    
    // Deprecated (event-centric)
    // ListUnprocessed(ctx context.Context, limit int32) ([]model.EventLog, error)
}
```

## Redis Stream Setup

### Stream and Consumer Group

```bash
# Create stream (auto-created on first XADD)
# Create consumer group
XGROUP CREATE issue_work issue-workers 0 MKSTREAM
```

### Producer (Webhook)

```go
// After DB commit
redis.XAdd(ctx, &redis.XAddArgs{
    Stream: "issue_work",
    Values: map[string]interface{}{
        "issue_id":     issueID,
        "event_log_id": eventLogID,
    },
})
```

### Consumer (Worker)

```go
// Read new messages
streams, _ := redis.XReadGroup(ctx, &redis.XReadGroupArgs{
    Group:    "issue-workers",
    Consumer: workerID,
    Streams:  []string{"issue_work", ">"},
    Count:    1,
    Block:    5 * time.Second,
})

// After processing
redis.XAck(ctx, "issue_work", "issue-workers", messageID)
```

### Reclaimer (Crash Recovery)

```go
// Find pending messages older than 5 minutes
pending, _ := redis.XPendingExt(ctx, &redis.XPendingExtArgs{
    Stream: "issue_work",
    Group:  "issue-workers",
    Idle:   5 * time.Minute,
    Start:  "-",
    End:    "+",
    Count:  10,
})

// Claim and reprocess
for _, p := range pending {
    redis.XClaim(ctx, &redis.XClaimArgs{
        Stream:   "issue_work",
        Group:    "issue-workers",
        Consumer: workerID,
        MinIdle:  5 * time.Minute,
        Messages: []string{p.ID},
    })
}
```

## Observability

### Metrics to Track

- `issues_queued_total`: Count of issues entering queued state
- `issues_processing_duration_seconds`: Time spent in processing state
- `issues_processed_total`: Count of successfully processed issues
- `events_per_issue_batch`: Histogram of events processed per issue
- `redis_pending_messages`: Gauge of messages in PEL

### Admin Queries

```sql
-- Queued issues (backlog)
SELECT COUNT(*) FROM issues WHERE processing_status = 'queued';

-- Stuck in processing (potential crashes)
SELECT * FROM issues 
WHERE processing_status = 'processing' 
  AND processing_started_at < now() - interval '10 minutes';

-- Unprocessed events per issue
SELECT issue_id, COUNT(*) 
FROM event_logs 
WHERE processed_at IS NULL 
GROUP BY issue_id;
```

## Migration Plan

1. **Schema**: Add `processing_started_at`, CHECK constraint, composite index
2. **Store**: Add new methods to IssueStore and EventLogStore
3. **Webhook**: Update to use `QueueIfIdle` + Redis XADD
4. **Worker**: Implement claim → batch process → complete flow
5. **Reclaimer**: Add periodic XPENDING/XCLAIM task
6. **Deprecate**: Remove `ListUnprocessed` after migration

## Open Questions

- [ ] Should we track `last_processed_event_id` for idempotency on COMMIT-then-crash scenarios?
- [ ] What's the XCLAIM idle timeout? (suggest: 5 minutes)
- [ ] Do we need a dead-letter queue for repeatedly failing issues?
