-- name: GetRepository :one
SELECT * FROM repositories WHERE id = $1;

-- name: GetRepositoryByExternalID :one
SELECT * FROM repositories 
WHERE integration_id = $1 AND external_repo_id = $2;

-- name: ListRepositoriesByWorkspace :many
SELECT * FROM repositories 
WHERE workspace_id = $1
ORDER BY name ASC;

-- name: ListEnabledRepositoriesByWorkspace :many
SELECT * FROM repositories
WHERE workspace_id = $1 AND is_enabled = true
ORDER BY name ASC;

-- name: ListRepositoriesByIntegration :many
SELECT * FROM repositories 
WHERE integration_id = $1
ORDER BY name ASC;

-- name: ListEnabledRepositoriesByIntegration :many
SELECT * FROM repositories
WHERE integration_id = $1 AND is_enabled = true
ORDER BY name ASC;

-- name: CreateRepository :one
INSERT INTO repositories (
    id, workspace_id, integration_id,
    name, slug, url, description, external_repo_id,
    is_enabled, default_branch,
    created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
RETURNING *;

-- name: UpdateRepository :one
UPDATE repositories
SET name = $2, slug = $3, url = $4, description = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetRepositoryEnabled :one
UPDATE repositories
SET is_enabled = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateRepositoryDefaultBranch :one
UPDATE repositories
SET default_branch = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteRepository :exec
DELETE FROM repositories WHERE id = $1;

-- name: DeleteRepositoriesByIntegration :exec
DELETE FROM repositories WHERE integration_id = $1;
