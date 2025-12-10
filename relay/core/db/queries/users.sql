-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByWorkOSID :one
SELECT * FROM users WHERE workos_id = $1;

-- name: CreateUser :one
INSERT INTO users (id, name, email, avatar_url, workos_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, now(), now())
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET name = $2, email = $3, avatar_url = $4, workos_id = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: UpsertUser :one
INSERT INTO users (id, name, email, avatar_url, workos_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, now(), now())
ON CONFLICT (email) DO UPDATE SET
  name = EXCLUDED.name,
  avatar_url = EXCLUDED.avatar_url,
  workos_id = EXCLUDED.workos_id,
  updated_at = now()
RETURNING *;

-- name: UpsertUserByWorkOSID :one
INSERT INTO users (id, name, email, avatar_url, workos_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, now(), now())
ON CONFLICT (workos_id) DO UPDATE SET
  name = EXCLUDED.name,
  email = EXCLUDED.email,
  avatar_url = EXCLUDED.avatar_url,
  updated_at = now()
RETURNING *;

