-- Shifts module: open / summary / close (one open shift per POS device).

-- name: ShiftGetOpenShiftByDevice :one
SELECT s.*, u.name AS opened_by_name
FROM shifts s JOIN users u ON u.id = s.opened_by
WHERE s.device_id = $1 AND s.status = 'open';

-- name: ShiftGetShiftWithUser :one
SELECT s.*, u.name AS opened_by_name
FROM shifts s JOIN users u ON u.id = s.opened_by
WHERE s.id = $1;

-- name: ShiftLockShift :one
SELECT * FROM shifts WHERE id = $1 FOR UPDATE;

-- name: ShiftInsertShift :one
INSERT INTO shifts (branch_id, device_id, opened_by, opened_at, opening_cash)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: ShiftTotals :one
SELECT
  (SELECT coalesce(sum(p.amount), 0) FROM payments p WHERE p.shift_id = sqlc.arg(shift_id)::bigint AND p.status = 'confirmed' AND p.method = 'cash')::bigint   AS cash_sales,
  (SELECT coalesce(sum(p.amount), 0) FROM payments p WHERE p.shift_id = sqlc.arg(shift_id)::bigint AND p.status = 'confirmed' AND p.method = 'vietqr')::bigint AS qr_sales,
  (SELECT coalesce(sum(r.cash_amount), 0) FROM refunds r WHERE r.shift_id = sqlc.arg(shift_id)::bigint)::bigint AS cash_refunds,
  (SELECT count(*) FROM orders o WHERE o.paid_shift_id = sqlc.arg(shift_id)::bigint AND o.status IN ('paid', 'refunded'))::int AS order_count,
  (SELECT count(*) FROM orders o WHERE o.shift_id = sqlc.arg(shift_id)::bigint AND o.status = 'open')::int AS open_orders,
  (SELECT count(*) FROM payments p WHERE p.shift_id = sqlc.arg(shift_id)::bigint AND p.status = 'pending')::int AS pending_qr,
  (SELECT count(*) FROM invoices i JOIN orders o ON o.id = i.order_id
    WHERE o.paid_shift_id = sqlc.arg(shift_id)::bigint AND i.status IN ('not_sent', 'failed'))::int AS invoices_not_sent;

-- name: ShiftListOpenOrdersOfShift :many
SELECT o.id, o.code, o.total, t.name AS table_name
FROM orders o LEFT JOIN dining_tables t ON t.id = o.table_id
WHERE o.shift_id = $1 AND o.status = 'open'
ORDER BY o.id;

-- name: ShiftCloseShift :exec
UPDATE shifts SET status = 'closed', closed_by = $2, closed_at = $3, expected_cash = $4,
  counted_cash = $5, cash_diff = $6, close_note = $7
WHERE id = $1;
