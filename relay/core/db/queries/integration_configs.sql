-- name: GetIntegrationConfig :one
SELECT * FROM integration_configs WHERE id = $1;

-- name: GetIntegrationConfigByKey :one
SELECT * FROM integration_configs 
WHERE integration_id = $1 AND key = $2;

-- name: ListIntegrationConfigs :many
SELECT * FROM integration_configs 
WHERE integration_id = $1
ORDER BY key ASC;

-- name: ListIntegrationConfigsByType :many
SELECT * FROM integration_configs 
WHERE integration_id = $1 AND config_type = $2
ORDER BY key ASC;

-- name: CreateIntegrationConfig :one
INSERT INTO integration_configs (
    id, integration_id, key, value, config_type,
    created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, now(), now())
RETURNING *;

-- name: UpdateIntegrationConfig :one
UPDATE integration_configs
SET value = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpsertIntegrationConfig :one
INSERT INTO integration_configs (id, integration_id, key, value, config_type, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, now(), now())
ON CONFLICT (id) DO UPDATE
SET value = EXCLUDED.value, updated_at = now()
RETURNING *;

-- name: DeleteIntegrationConfig :exec
DELETE FROM integration_configs WHERE id = $1;

-- name: DeleteIntegrationConfigsByIntegration :exec
DELETE FROM integration_configs WHERE integration_id = $1;

