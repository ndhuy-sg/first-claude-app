-- Queries of the authapi module (login, PIN, devices).

-- name: AuthGetUserForUpdate :one
SELECT * FROM users WHERE id = $1 FOR UPDATE;

-- name: AuthRecordLoginFailure :one
-- Adds one failure; when the limit is reached the account is locked until lock_until and the counter restarts.
UPDATE users
SET failed_logins = CASE WHEN failed_logins + 1 >= sqlc.arg(max_failures)::int THEN 0 ELSE failed_logins + 1 END,
    locked_until  = CASE WHEN failed_logins + 1 >= sqlc.arg(max_failures)::int THEN sqlc.arg(lock_until)::timestamptz ELSE locked_until END
WHERE id = sqlc.arg(id)
RETURNING failed_logins, locked_until;

-- name: AuthRecordLoginSuccess :exec
UPDATE users SET failed_logins = 0, locked_until = NULL, last_active_at = sqlc.arg(now)::timestamptz WHERE id = sqlc.arg(id);

-- name: AuthCreateDevice :one
INSERT INTO devices (branch_id, name, token_hash, activated_by, last_seen_at)
VALUES ($1, $2, $3, $4, sqlc.arg(now)::timestamptz)
RETURNING id;

-- name: AuthGetDevice :one
SELECT d.id, d.branch_id, d.name, d.last_seen_at, d.revoked_at, b.code AS branch_code
FROM devices d JOIN branches b ON b.id = d.branch_id
WHERE d.id = $1;

-- name: AuthListDevices :many
SELECT d.id, d.branch_id, d.name, d.last_seen_at, d.revoked_at, b.code AS branch_code
FROM devices d JOIN branches b ON b.id = d.branch_id
WHERE sqlc.narg(branch_id)::bigint IS NULL OR d.branch_id = sqlc.narg(branch_id)::bigint
ORDER BY d.branch_id, d.id;

-- name: AuthRevokeDevice :exec
UPDATE devices SET revoked_at = sqlc.arg(now)::timestamptz WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: AuthRevokeDeviceSessions :exec
UPDATE sessions SET revoked_at = sqlc.arg(now)::timestamptz WHERE device_id = sqlc.arg(device_id) AND revoked_at IS NULL;

-- name: AuthListPinStaff :many
-- Active staff of a branch who can log in with a PIN on its devices.
SELECT id, name, role FROM users
WHERE branch_id = $1 AND disabled_at IS NULL AND pin_hash IS NOT NULL
ORDER BY CASE role WHEN 'cashier' THEN 0 WHEN 'barista' THEN 1 ELSE 2 END, name, id;
