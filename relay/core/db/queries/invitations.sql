-- name: CreateInvitation :one
INSERT INTO invitations (id, email, token, status, invited_by, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
RETURNING *;

-- name: GetInvitationByID :one
SELECT * FROM invitations WHERE id = $1;

-- name: GetInvitationByToken :one
SELECT * FROM invitations WHERE token = $1;

-- name: GetInvitationByEmail :one
SELECT * FROM invitations 
WHERE email = $1 AND status = 'pending'
ORDER BY created_at DESC
LIMIT 1;

-- name: GetValidInvitationByToken :one
SELECT * FROM invitations 
WHERE token = $1 
  AND status = 'pending' 
  AND expires_at > now();

-- name: ListInvitations :many
SELECT * FROM invitations 
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListPendingInvitations :many
SELECT * FROM invitations 
WHERE status = 'pending'
ORDER BY created_at DESC;

-- name: AcceptInvitation :one
UPDATE invitations 
SET status = 'accepted', 
    accepted_by = $2, 
    accepted_at = now()
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: RevokeInvitation :one
UPDATE invitations 
SET status = 'revoked'
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: ExpireOldInvitations :exec
UPDATE invitations 
SET status = 'expired'
WHERE status = 'pending' AND expires_at < now();
