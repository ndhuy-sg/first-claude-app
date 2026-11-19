-- Queries of the branches module (settings, branches, areas, tables).

-- name: BranchesUpdateSettings :one
UPDATE app_settings SET
    company_name = $1, company_tax_code = $2, invoice_email_from = $3, tax_submission_enabled = $4,
    tax_gateway = $5, discount_max_pct = $6, pos_idle_lock_minutes = $7, login_max_failures = $8,
    login_lock_minutes = $9, shift_note_threshold = $10, prices_include_vat = $11,
    updated_at = sqlc.arg(now)::timestamptz
WHERE id = 1
RETURNING *;

-- name: BranchesList :many
SELECT b.*, COALESCE(s.symbol, '')::text AS invoice_symbol, COALESCE(s.next_number, 1)::bigint AS next_invoice_number
FROM branches b LEFT JOIN invoice_series s ON s.branch_id = b.id
WHERE sqlc.narg(branch_id)::bigint IS NULL OR b.id = sqlc.narg(branch_id)::bigint
ORDER BY b.id;

-- name: BranchesGetForUpdate :one
SELECT * FROM branches WHERE id = $1 FOR UPDATE;

-- name: BranchesCodeTaken :one
SELECT EXISTS (SELECT 1 FROM branches WHERE lower(code) = lower(sqlc.arg(code)::text) AND id <> sqlc.arg(except_id)::bigint);

-- name: BranchesCreate :one
INSERT INTO branches (code, name, address, opening_hours, tax_code, vat_rate_bp, vietqr_bank_bin, vietqr_account_no, vietqr_account_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: BranchesUpdate :exec
UPDATE branches SET code = $2, name = $3, address = $4, opening_hours = $5, tax_code = $6, vat_rate_bp = $7,
    vietqr_bank_bin = $8, vietqr_account_no = $9, vietqr_account_name = $10
WHERE id = $1;

-- name: BranchesGetSeriesForUpdate :one
SELECT * FROM invoice_series WHERE branch_id = $1 FOR UPDATE;

-- name: BranchesUpsertSeries :exec
-- A new symbol continues after the highest number already issued under it (normally 1).
INSERT INTO invoice_series (branch_id, symbol, next_number)
VALUES (sqlc.arg(branch_id), sqlc.arg(symbol)::text,
        COALESCE((SELECT max(number) FROM invoices WHERE branch_id = sqlc.arg(branch_id) AND symbol = sqlc.arg(symbol)::text), 0) + 1)
ON CONFLICT (branch_id) DO UPDATE SET symbol = EXCLUDED.symbol, next_number = EXCLUDED.next_number;

-- name: BranchesCreateArea :one
INSERT INTO areas (branch_id, name, sort_order)
VALUES ($1, $2, COALESCE((SELECT max(sort_order) + 1 FROM areas WHERE branch_id = $1), 0))
ON CONFLICT (branch_id, name) DO NOTHING
RETURNING *;

-- name: BranchesGetArea :one
SELECT * FROM areas WHERE id = $1;

-- name: BranchesCreateTable :one
INSERT INTO dining_tables (branch_id, area_id, name, sort_order)
VALUES ($1, $2, $3, COALESCE((SELECT max(sort_order) + 1 FROM dining_tables WHERE branch_id = $1), 1))
ON CONFLICT (branch_id, name) DO NOTHING
RETURNING *;
