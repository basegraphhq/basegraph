-- name: GetOrganization :one
SELECT * FROM organizations WHERE id = $1 AND is_deleted = false;

-- name: GetOrganizationBySlug :one
SELECT * FROM organizations WHERE slug = $1 AND is_deleted = false;

-- name: ListOrganizationsByAdmin :many
SELECT * FROM organizations 
WHERE admin_user_id = $1 AND is_deleted = false
ORDER BY created_at DESC;

-- name: CreateOrganization :one
INSERT INTO organizations (id, admin_user_id, name, slug, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
RETURNING *;

-- name: UpdateOrganization :one
UPDATE organizations
SET name = $2, slug = $3, updated_at = now()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: SoftDeleteOrganization :exec
UPDATE organizations SET is_deleted = true, updated_at = now() WHERE id = $1;

