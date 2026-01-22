-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE LOWER(email) = LOWER(sqlc.arg(email));

-- name: GetUserByWorkOSID :one
SELECT * FROM users WHERE workos_id = $1;

-- name: CreateUser :one
INSERT INTO users (id, name, email, avatar_url, workos_id, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(name), LOWER(sqlc.arg(email)), sqlc.arg(avatar_url), sqlc.arg(workos_id), now(), now())
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET name = sqlc.arg(name), email = LOWER(sqlc.arg(email)), avatar_url = sqlc.arg(avatar_url), workos_id = sqlc.arg(workos_id), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: UpsertUser :one
INSERT INTO users (id, name, email, avatar_url, workos_id, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(name), LOWER(sqlc.arg(email)), sqlc.arg(avatar_url), sqlc.arg(workos_id), now(), now())
ON CONFLICT (email) DO UPDATE SET
  name = EXCLUDED.name,
  avatar_url = EXCLUDED.avatar_url,
  workos_id = EXCLUDED.workos_id,
  updated_at = now()
RETURNING *;

-- name: UpsertUserByWorkOSID :one
INSERT INTO users (id, name, email, avatar_url, workos_id, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(name), LOWER(sqlc.arg(email)), sqlc.arg(avatar_url), sqlc.arg(workos_id), now(), now())
ON CONFLICT (workos_id) DO UPDATE SET
  name = EXCLUDED.name,
  email = LOWER(EXCLUDED.email),
  avatar_url = EXCLUDED.avatar_url,
  updated_at = now()
RETURNING *;

