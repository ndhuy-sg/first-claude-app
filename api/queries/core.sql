-- Core queries shared by every module (sessions, users, devices, settings, branches).
-- Each module keeps its own queries in queries/<module>.sql.

-- name: GetSettings :one
SELECT * FROM app_settings WHERE id = 1;

-- name: GetBranch :one
SELECT * FROM branches WHERE id = $1;

-- name: ListBranches :many
SELECT * FROM branches ORDER BY id;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(sqlc.arg(email)::text);

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, device_id, kind, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetActiveSession :one
SELECT s.token_hash, s.user_id, s.device_id, s.kind, s.last_seen_at, s.expires_at,
       u.name AS user_name, u.role AS user_role, u.branch_id AS user_branch_id
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND u.disabled_at IS NULL;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE token_hash = $1;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeUserSessions :exec
UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;

-- name: GetActiveDeviceByToken :one
SELECT * FROM devices WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: TouchDevice :exec
UPDATE devices SET last_seen_at = now() WHERE id = $1;

-- name: InsertAuditLog :exec
INSERT INTO audit_logs (actor_id, approver_id, branch_id, action, entity_type, entity_id, before, after, note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: EnqueueJob :one
INSERT INTO jobs (kind, payload, dedupe_key, run_at, max_attempts)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (dedupe_key) DO UPDATE SET dedupe_key = EXCLUDED.dedupe_key
RETURNING id;
