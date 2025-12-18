# Pipeline Runs Table (Archived)

This document captures the original `pipeline_runs` table design before it was removed in favor of issue-centric processing.

## Original Schema

```sql
create table pipeline_runs (
    id bigint primary key,
    event_log_id bigint not null references event_logs(id),
    attempt int not null default 1,
    status text not null,
    error text,
    started_at timestamptz not null default now(),
    finished_at timestamptz
);

create index idx_pipeline_runs_event_log_id on pipeline_runs (event_log_id);
create index idx_pipeline_runs_status on pipeline_runs (status);
```

## Purpose

The table was designed for **event-centric processing**:
- One `pipeline_run` per `event_log` entry
- Track attempt count for retries
- Track status (e.g., `pending`, `processing`, `completed`, `failed`)
- Store error messages for debugging

## Why It Was Removed

We shifted to **issue-centric processing**:

| Event-Centric | Issue-Centric |
|---------------|---------------|
| Process each event independently | Process all pending events for an issue together |
| One pipeline run per event | One processing session per issue |
| Risk of race conditions on shared issue state | Lock per issue, sequential processing |
| Multiple replies to same issue | Coherent, human-like responses |

## Replacement Design

Instead of `pipeline_runs`, we now track processing state on the `issues` table:

```sql
ALTER TABLE issues 
ADD COLUMN processing_status text NOT NULL DEFAULT 'idle',
ADD COLUMN last_processed_at timestamptz;
```

For detailed run tracking (if needed later), consider:

```sql
create table issue_processing_runs (
    id bigint primary key,
    issue_id bigint not null references issues(id),
    event_log_ids bigint[] not null,  -- which events were processed
    status text not null,
    error text,
    started_at timestamptz not null default now(),
    finished_at timestamptz
);
```

This tracks runs at the issue level, with references to all events processed in that run.

## Migration Path

If you need to restore this functionality:
1. Create `issue_processing_runs` table (above)
2. In the worker, create a run record when starting, update when finishing
3. Use `event_log_ids` array to link processed events

## Related Files

- `relay/internal/store/pipeline_run.go` (removed)
- `relay/core/db/queries/pipeline_runs.sql` (removed)
