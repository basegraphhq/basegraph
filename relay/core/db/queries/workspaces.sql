-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = $1 AND is_deleted = false;

-- name: GetWorkspaceByOrgAndSlug :one
SELECT * FROM workspaces 
WHERE organization_id = $1 AND slug = $2 AND is_deleted = false;

-- name: ListWorkspacesByOrganization :many
SELECT * FROM workspaces 
WHERE organization_id = $1 AND is_deleted = false
ORDER BY created_at DESC;

-- name: ListWorkspacesByUser :many
SELECT * FROM workspaces 
WHERE user_id = $1 AND is_deleted = false
ORDER BY created_at DESC;

-- name: CreateWorkspace :one
INSERT INTO workspaces (
    id, admin_user_id, organization_id, user_id, 
    name, slug, description, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())
RETURNING *;

-- name: UpdateWorkspace :one
UPDATE workspaces
SET name = $2, slug = $3, description = $4, updated_at = now()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: SetWorkspaceRepoReadyAt :one
UPDATE workspaces
SET repo_ready_at = $2, updated_at = now()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: SoftDeleteWorkspace :exec
UPDATE workspaces SET is_deleted = true, updated_at = now() WHERE id = $1;
