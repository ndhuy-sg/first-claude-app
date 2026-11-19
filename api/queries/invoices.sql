-- Invoices module: self-issued e-invoices (numbering, lookup, listing).

-- name: InvLockInvoiceSeries :one
SELECT * FROM invoice_series WHERE branch_id = $1 FOR UPDATE;

-- name: InvBumpInvoiceSeries :exec
UPDATE invoice_series SET next_number = next_number + 1 WHERE branch_id = $1;

-- name: InvLookupCodeExists :one
SELECT EXISTS (SELECT 1 FROM invoices WHERE lookup_code = $1);

-- name: InvInsertInvoice :one
INSERT INTO invoices (branch_id, order_id, kind, original_invoice_id, symbol, number, lookup_code,
  seller_name, seller_tax_code, seller_address, buyer_name, buyer_tax_code, buyer_address, buyer_email,
  payment_method, vat_rate_bp, amount_before_vat, vat_amount, total, status, issued_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, 'not_sent', $20)
RETURNING id;

-- name: InvInsertInvoiceLine :exec
INSERT INTO invoice_lines (invoice_id, line_no, name, qty, unit_price, amount)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InvGetInvoiceRow :one
SELECT i.*, o.code AS order_code,
  EXISTS (SELECT 1 FROM invoices c WHERE c.original_invoice_id = i.id AND c.kind = 'cancel') AS cancelled
FROM invoices i JOIN orders o ON o.id = i.order_id
WHERE i.id = $1;

-- name: InvGetInvoiceIDByLookup :one
SELECT id FROM invoices WHERE lookup_code = $1;

-- name: InvListInvoiceLines :many
SELECT * FROM invoice_lines WHERE invoice_id = $1 ORDER BY line_no;

-- name: InvGetNormalInvoiceByOrder :one
SELECT * FROM invoices WHERE order_id = $1 AND kind = 'normal' ORDER BY id DESC LIMIT 1;

-- name: InvListInvoiceIDs :many
SELECT i.id FROM invoices i
WHERE (sqlc.narg(branch_id)::bigint IS NULL OR i.branch_id = sqlc.narg(branch_id))
  AND (sqlc.narg(status)::text IS NULL OR i.status = sqlc.narg(status))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR i.issued_at >= sqlc.narg(from_at))
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR i.issued_at < sqlc.narg(to_at))
ORDER BY i.issued_at DESC, i.id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: InvCountInvoices :one
SELECT count(*)::int FROM invoices i
WHERE (sqlc.narg(branch_id)::bigint IS NULL OR i.branch_id = sqlc.narg(branch_id))
  AND (sqlc.narg(status)::text IS NULL OR i.status = sqlc.narg(status))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR i.issued_at >= sqlc.narg(from_at))
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR i.issued_at < sqlc.narg(to_at));

-- name: InvCounts :one
SELECT
  count(*) FILTER (WHERE i.issued_at >= sqlc.arg(day_start)::timestamptz AND i.issued_at < sqlc.arg(day_end)::timestamptz)::int AS issued_today,
  count(*) FILTER (WHERE i.status = 'not_sent')::int AS not_sent,
  count(*) FILTER (WHERE i.status = 'failed')::int AS failed,
  count(*) FILTER (WHERE i.kind = 'cancel')::int AS cancelled
FROM invoices i
WHERE (sqlc.narg(branch_id)::bigint IS NULL OR i.branch_id = sqlc.narg(branch_id));

-- name: InvLockInvoice :one
SELECT * FROM invoices WHERE id = $1 FOR UPDATE;

-- name: InvSetInvoiceStatus :exec
UPDATE invoices SET status = $2 WHERE id = $1;

-- name: InvMarkInvoiceSent :exec
UPDATE invoices SET status = 'sent', sent_at = $2 WHERE id = $1;

-- name: InvRequeueJob :exec
-- Creates the job, or puts an existing one (any status) back in the queue with a fresh attempt budget.
INSERT INTO jobs (kind, payload, dedupe_key, run_at, max_attempts)
VALUES ($1, $2, $3, $4, 10)
ON CONFLICT (dedupe_key) DO UPDATE
SET status = 'queued', attempts = 0, run_at = EXCLUDED.run_at, last_error = '', updated_at = now();
