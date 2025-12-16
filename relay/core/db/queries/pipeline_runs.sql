-- name: CreatePipelineRun :one
INSERT INTO pipeline_runs (id, event_log_id, attempt, status, error, started_at)
VALUES ($1, $2, $3, $4, $5, now())
RETURNING *;

-- name: FinishPipelineRun :exec
UPDATE pipeline_runs
SET status = $2, error = $3, finished_at = now()
WHERE id = $1;

-- name: ListPipelineRunsForEvent :many
SELECT * FROM pipeline_runs
WHERE event_log_id = $1
ORDER BY started_at DESC
LIMIT $2;

