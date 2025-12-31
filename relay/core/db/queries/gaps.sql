-- name: CreateGap :one
INSERT INTO gaps (
    id,
    issue_id,
    status,
    question,
    evidence,
    severity,
    respondent
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetGap :one
SELECT * FROM gaps WHERE id = $1;

-- name: ListGapsByIssue :many
SELECT * FROM gaps
WHERE issue_id = $1
ORDER BY created_at DESC;

-- name: ListOpenGapsByIssue :many
SELECT * FROM gaps
WHERE issue_id = $1 AND status = 'open'
ORDER BY
    CASE severity
        WHEN 'blocking' THEN 1
        WHEN 'high' THEN 2
        WHEN 'medium' THEN 3
        WHEN 'low' THEN 4
    END,
    created_at ASC;

-- name: ResolveGap :one
UPDATE gaps
SET status = 'resolved',
    resolved_at = now()
WHERE id = $1
RETURNING *;

-- name: SkipGap :one
UPDATE gaps
SET status = 'skipped',
    resolved_at = now()
WHERE id = $1
RETURNING *;

-- name: SetGapLearning :one
UPDATE gaps
SET learning_id = $2
WHERE id = $1
RETURNING *;

-- name: CountOpenBlockingGapsByIssue :one
SELECT COUNT(*)::bigint FROM gaps
WHERE issue_id = $1 AND status = 'open' AND severity = 'blocking';
