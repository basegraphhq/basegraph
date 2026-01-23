-- name: GetWorkspaceEventLog :one
SELECT * FROM workspace_event_logs WHERE id = $1;

-- name: ListWorkspaceEventLogsByWorkspace :many
SELECT * FROM workspace_event_logs
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: CreateWorkspaceEventLog :one
INSERT INTO workspace_event_logs (
    id, workspace_id, organization_id, repo_id,
    event_type, status, error, metadata
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateWorkspaceEventLog :one
UPDATE workspace_event_logs
SET status = $2,
    error = $3,
    metadata = $4,
    started_at = COALESCE($5, started_at),
    finished_at = COALESCE($6, finished_at),
    updated_at = now()
WHERE id = $1
RETURNING *;
