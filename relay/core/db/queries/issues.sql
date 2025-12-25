-- name: UpsertIssue :one
INSERT INTO issues (
    id,
    integration_id,
    external_issue_id,
    provider,
    title,
    description,
    labels,
    members,
    assignees,
    reporter,
    keywords,
    code_findings,
    learnings,
    discussions,
    spec,
    created_at,
    updated_at
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now(), now()
)
ON CONFLICT (integration_id, external_issue_id)
DO UPDATE
SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    labels = EXCLUDED.labels,
    members = EXCLUDED.members,
    assignees = EXCLUDED.assignees,
    reporter = EXCLUDED.reporter,
    keywords = EXCLUDED.keywords,
    code_findings = EXCLUDED.code_findings,
    learnings = EXCLUDED.learnings,
    discussions = EXCLUDED.discussions,
    spec = EXCLUDED.spec,
    updated_at = now()
RETURNING *;

-- name: GetIssue :one
SELECT * FROM issues WHERE id = $1;

-- name: GetIssueByIntegrationAndExternalID :one
SELECT * FROM issues WHERE integration_id = $1 AND external_issue_id = $2;

-- name: QueueIssueIfIdle :one
--
-- Queue an issue for processing, with automatic recovery of stuck issues.
--
-- This query handles three scenarios:
--
-- 1. NORMAL PATH (idle → queued)
--    Issue is idle and ready to be processed. This is the happy path.
--
-- 2. STUCK IN 'processing' (processing → queued after 15 min)
--    Worker crashed after claiming the issue (TX1) but before completing (TX2).
--    The issue is stuck in 'processing' forever. When user pings again after
--    15 minutes, we reset it to 'queued' so it can be reprocessed.
--
-- 3. STUCK IN 'queued' (remains queued, but gets re-queued after 15 min)
--    Server crashed after QueueIfIdle but before publishing to Redis.
--    The issue is stuck in 'queued' with no Redis message. When user pings
--    again after 15 minutes, we update it (triggering a new Redis publish).
--
-- Why 15 minutes?
--    - LLM calls typically take 5-60 seconds, never 15 minutes
--    - Gives legitimate processing plenty of time to complete
--    - Short enough that users don't wait too long before "ping again" works
--
-- Why this approach instead of a background reclaimer?
--    - Simpler: one SQL query vs. separate goroutine with timers
--    - User-triggered: recovery happens when user cares enough to ping again
--    - Matches UX: "ping again if no response" - just like a human teammate
--
UPDATE issues
SET processing_status = 'queued',
    processing_started_at = NULL,
    updated_at = now()
WHERE id = $1
  AND (
    processing_status = 'idle'
    OR (processing_status = 'processing' AND processing_started_at < NOW() - INTERVAL '15 minutes')
    OR (processing_status = 'queued' AND updated_at < NOW() - INTERVAL '15 minutes')
  )
RETURNING *;

-- name: ClaimQueuedIssue :one
-- Atomically transition issue from 'queued' to 'processing'.
-- Returns the issue if claimed, no rows if already claimed by another worker.
UPDATE issues
SET processing_status = 'processing',
    processing_started_at = now(),
    updated_at = now()
WHERE id = $1
  AND processing_status = 'queued'
RETURNING *;

-- name: SetIssueIdle :execrows
-- Transition issue from 'processing' to 'idle'.
UPDATE issues
SET processing_status = 'idle',
    last_processed_at = now(),
    processing_started_at = NULL,
    updated_at = now()
WHERE id = $1
  AND processing_status = 'processing';
