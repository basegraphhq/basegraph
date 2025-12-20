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
-- Atomically transition issue from 'idle' to 'queued'.
-- Returns the issue if transition happened, no rows if already queued/processing.
UPDATE issues
SET processing_status = 'queued',
    updated_at = now()
WHERE id = $1
  AND processing_status = 'idle'
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

-- name: SetIssueProcessed :execrows
-- Mark issue processing complete. Transition from 'processing' to 'idle'.
UPDATE issues
SET processing_status = 'idle',
    last_processed_at = now(),
    processing_started_at = NULL,
    updated_at = now()
WHERE id = $1
  AND processing_status = 'processing';
