-- name: GetIntegrationCredential :one
SELECT * FROM integration_credentials WHERE id = $1;

-- name: GetPrimaryCredentialByIntegration :one
SELECT * FROM integration_credentials
WHERE integration_id = $1 AND is_primary = true AND revoked_at IS NULL;

-- name: GetCredentialByIntegrationAndUser :one
SELECT * FROM integration_credentials
WHERE integration_id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: ListCredentialsByIntegration :many
SELECT * FROM integration_credentials
WHERE integration_id = $1
ORDER BY is_primary DESC, created_at DESC;

-- name: ListActiveCredentialsByIntegration :many
SELECT * FROM integration_credentials
WHERE integration_id = $1 AND revoked_at IS NULL
ORDER BY is_primary DESC, created_at DESC;

-- name: CreateIntegrationCredential :one
INSERT INTO integration_credentials (
    id, integration_id, user_id,
    credential_type, access_token, refresh_token, token_expires_at, scopes,
    is_primary,
    created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())
RETURNING *;

-- name: UpdateCredentialTokens :one
UPDATE integration_credentials
SET 
    access_token = $2,
    refresh_token = $3,
    token_expires_at = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetCredentialAsPrimary :exec
UPDATE integration_credentials
SET is_primary = (id = $2), updated_at = now()
WHERE integration_id = $1 AND revoked_at IS NULL;

-- name: RevokeCredential :one
UPDATE integration_credentials
SET revoked_at = now(), is_primary = false, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: RevokeAllCredentialsByIntegration :exec
UPDATE integration_credentials
SET revoked_at = now(), is_primary = false, updated_at = now()
WHERE integration_id = $1 AND revoked_at IS NULL;

-- name: DeleteIntegrationCredential :exec
DELETE FROM integration_credentials WHERE id = $1;

