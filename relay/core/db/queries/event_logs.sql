-- name: GetEventLog :one
SELECT * FROM event_logs WHERE id = $1;

-- name: ListEventLogsByWorkspace :many
SELECT * FROM event_logs 
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListEventLogsByWorkspaceAndSource :many
SELECT * FROM event_logs 
WHERE workspace_id = $1 AND source = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: ListUnprocessedEventLogs :many
SELECT * FROM event_logs 
WHERE processed_at IS NULL
ORDER BY created_at ASC
LIMIT $1;

-- name: CreateEventLog :one
INSERT INTO event_logs (
    id, workspace_id, source, event_type,
    payload, external_id, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, now())
RETURNING *;

-- name: MarkEventLogProcessed :exec
UPDATE event_logs
SET processed_at = now()
WHERE id = $1;

-- name: MarkEventLogFailed :exec
UPDATE event_logs
SET processed_at = now(), processing_error = $2
WHERE id = $1;

