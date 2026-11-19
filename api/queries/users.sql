-- Queries of the users module (staff management).

-- name: UsersList :many
SELECT * FROM users
WHERE sqlc.narg(branch_id)::bigint IS NULL OR branch_id = sqlc.narg(branch_id)::bigint
ORDER BY branch_id NULLS FIRST, id;

-- name: UsersGetForUpdate :one
SELECT * FROM users WHERE id = $1 FOR UPDATE;

-- name: UsersPinHashesInBranch :many
-- PIN hashes of the active staff of a branch (for the PIN uniqueness check, D-13).
SELECT id, pin_hash FROM users
WHERE branch_id = $1 AND disabled_at IS NULL AND pin_hash IS NOT NULL;

-- name: UsersCreate :one
INSERT INTO users (name, email, password_hash, pin_hash, role, branch_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UsersUpdate :one
UPDATE users
SET name = $2, email = $3, password_hash = $4, pin_hash = $5, pin_must_change = $6, role = $7, branch_id = $8
WHERE id = $1
RETURNING *;

-- name: UsersDisable :exec
UPDATE users SET disabled_at = sqlc.arg(now)::timestamptz, disabled_by = sqlc.arg(by) WHERE id = sqlc.arg(id);

-- name: UsersEnable :exec
UPDATE users SET disabled_at = NULL, disabled_by = NULL, failed_logins = 0, locked_until = NULL WHERE id = $1;

-- name: UsersSetPin :exec
UPDATE users SET pin_hash = $2, pin_must_change = $3 WHERE id = $1;
