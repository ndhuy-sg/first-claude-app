-- Audit log (append-only; there are deliberately no UPDATE/DELETE queries).
-- branch_id NULL = no branch filter; chain_only = only entries without a branch
-- (warehouse manager scope).

-- name: ListAuditLogs :many
SELECT a.id, a.at, a.actor_id, ua.name AS actor_name, a.approver_id, up.name AS approver_name,
       a.branch_id, a.action, a.entity_type, a.entity_id, a.before, a.after, a.note
FROM audit_logs a
LEFT JOIN users ua ON ua.id = a.actor_id
LEFT JOIN users up ON up.id = a.approver_id
WHERE (sqlc.narg(branch_id)::bigint IS NULL OR a.branch_id = sqlc.narg(branch_id)::bigint)
  AND (NOT sqlc.arg(chain_only)::bool OR a.branch_id IS NULL)
  AND (sqlc.narg(action)::text IS NULL OR a.action = sqlc.narg(action)::text)
ORDER BY a.at DESC, a.id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountAuditLogs :one
SELECT COUNT(*)::bigint
FROM audit_logs a
WHERE (sqlc.narg(branch_id)::bigint IS NULL OR a.branch_id = sqlc.narg(branch_id)::bigint)
  AND (NOT sqlc.arg(chain_only)::bool OR a.branch_id IS NULL)
  AND (sqlc.narg(action)::text IS NULL OR a.action = sqlc.narg(action)::text);
