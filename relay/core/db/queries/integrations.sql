-- name: GetIntegration :one
SELECT * FROM integrations WHERE id = $1;

-- name: GetIntegrationByWorkspaceAndProvider :one
SELECT * FROM integrations 
WHERE workspace_id = $1 AND provider = $2;

-- name: ListIntegrationsByWorkspace :many
SELECT * FROM integrations 
WHERE workspace_id = $1
ORDER BY created_at DESC;

-- name: ListIntegrationsByOrganization :many
SELECT * FROM integrations 
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: CreateIntegration :one
INSERT INTO integrations (
    id, workspace_id, organization_id, provider, provider_base_url,
    external_org_id, external_workspace_id,
    access_token, refresh_token, expires_at,
    created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
RETURNING *;

-- name: UpdateIntegrationTokens :one
UPDATE integrations
SET access_token = $2, refresh_token = $3, expires_at = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteIntegration :exec
DELETE FROM integrations WHERE id = $1;

