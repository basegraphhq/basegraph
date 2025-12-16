-- name: GetLearning :one
SELECT * FROM learnings
WHERE id = $1 LIMIT 1;

-- name: ListLearningsByWorkspace :many
SELECT * FROM learnings
WHERE workspace_id = $1
ORDER BY created_at DESC;

-- name: ListLearningsByWorkspaceAndType :many
SELECT * FROM learnings
WHERE workspace_id = $1 AND type = $2
ORDER BY created_at DESC;

-- name: CreateLearning :one
INSERT INTO learnings (
    id, workspace_id, rule_updated_by_issue_id, type, content
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: UpdateLearning :one
UPDATE learnings
SET content = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteLearning :exec
DELETE FROM learnings WHERE id = $1;