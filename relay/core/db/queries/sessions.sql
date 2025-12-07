-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1;

-- name: GetValidSession :one
SELECT * FROM sessions 
WHERE id = $1 AND expires_at > now();

-- name: ListSessionsByUser :many
SELECT * FROM sessions 
WHERE user_id = $1 AND expires_at > now()
ORDER BY created_at DESC;

-- name: CreateSession :one
INSERT INTO sessions (id, user_id, created_at, expires_at)
VALUES ($1, $2, now(), $3)
RETURNING *;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < now();

-- name: DeleteSessionsByUser :exec
DELETE FROM sessions WHERE user_id = $1;

