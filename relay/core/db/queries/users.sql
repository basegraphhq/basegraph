-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: CreateUser :one
INSERT INTO users (id, name, email, avatar_url, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET name = $2, email = $3, avatar_url = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: UpsertUser :one
INSERT INTO users (id, name, email, avatar_url, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
ON CONFLICT (email) DO UPDATE SET
  name = EXCLUDED.name,
  avatar_url = EXCLUDED.avatar_url,
  updated_at = now()
RETURNING *;

