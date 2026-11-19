-- Sales reports and dashboard.
-- Revenue counts only orders with status 'paid' (cancelled, merged and refunded
-- orders are excluded, R-01). The day of an order is the Vietnam calendar day of
-- paid_at (R-05): callers pass [from_ts, to_ts) as instants of VN midnights.
-- branch_id NULL means every branch.

-- name: ReportTotals :one
SELECT COALESCE(SUM(o.total), 0)::bigint           AS revenue,
       COUNT(*)::bigint                             AS order_count,
       COALESCE(SUM(o.discount_amount), 0)::bigint  AS discount
FROM orders o
WHERE o.status = 'paid'
  AND o.paid_at >= sqlc.arg(from_ts)::timestamptz AND o.paid_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg(branch_id)::bigint IS NULL OR o.branch_id = sqlc.narg(branch_id)::bigint);

-- name: ReportRefunds :one
SELECT COALESCE(SUM(r.amount), 0)::bigint AS refunds
FROM refunds r
WHERE r.created_at >= sqlc.arg(from_ts)::timestamptz AND r.created_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg(branch_id)::bigint IS NULL OR r.branch_id = sqlc.narg(branch_id)::bigint);

-- name: ReportByDay :many
SELECT to_char((o.paid_at AT TIME ZONE 'Asia/Ho_Chi_Minh')::date, 'YYYY-MM-DD')::text AS day,
       COALESCE(SUM(o.total), 0)::bigint AS revenue,
       COUNT(*)::bigint                  AS order_count
FROM orders o
WHERE o.status = 'paid'
  AND o.paid_at >= sqlc.arg(from_ts)::timestamptz AND o.paid_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg(branch_id)::bigint IS NULL OR o.branch_id = sqlc.narg(branch_id)::bigint)
GROUP BY 1
ORDER BY 1;

-- name: ReportByHour :many
SELECT EXTRACT(HOUR FROM (o.paid_at AT TIME ZONE 'Asia/Ho_Chi_Minh'))::int AS hour,
       COALESCE(SUM(o.total), 0)::bigint AS revenue,
       COUNT(*)::bigint                  AS order_count
FROM orders o
WHERE o.status = 'paid'
  AND o.paid_at >= sqlc.arg(from_ts)::timestamptz AND o.paid_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg(branch_id)::bigint IS NULL OR o.branch_id = sqlc.narg(branch_id)::bigint)
GROUP BY 1
ORDER BY 1;

-- Product revenue is the sum of line totals (before any order-level discount);
-- the order-level discount is reported once in totals.discount.
-- name: ReportByProduct :many
SELECT oi.product_id,
       p.name                              AS name,
       COALESCE(SUM(oi.qty), 0)::bigint        AS qty,
       COALESCE(SUM(oi.line_total), 0)::bigint AS revenue,
       COUNT(DISTINCT o.id)::bigint            AS order_count
FROM order_items oi
JOIN orders o   ON o.id = oi.order_id
JOIN products p ON p.id = oi.product_id
WHERE o.status = 'paid'
  AND o.paid_at >= sqlc.arg(from_ts)::timestamptz AND o.paid_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg(branch_id)::bigint IS NULL OR o.branch_id = sqlc.narg(branch_id)::bigint)
GROUP BY oi.product_id, p.name
ORDER BY qty DESC, revenue DESC, oi.product_id
LIMIT sqlc.arg(max_rows);

-- name: ReportByPaymentMethod :many
SELECT pm.method,
       COALESCE(SUM(pm.amount), 0)::bigint AS revenue,
       COUNT(DISTINCT o.id)::bigint        AS order_count
FROM payments pm
JOIN orders o ON o.id = pm.order_id
WHERE o.status = 'paid' AND pm.status = 'confirmed'
  AND o.paid_at >= sqlc.arg(from_ts)::timestamptz AND o.paid_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg(branch_id)::bigint IS NULL OR o.branch_id = sqlc.narg(branch_id)::bigint)
GROUP BY pm.method
ORDER BY revenue DESC, pm.method;

-- Staff = the user who created the order (orders.created_by).
-- name: ReportByStaff :many
SELECT u.id                              AS user_id,
       u.name                            AS name,
       COALESCE(SUM(o.total), 0)::bigint AS revenue,
       COUNT(*)::bigint                  AS order_count
FROM orders o
JOIN users u ON u.id = o.created_by
WHERE o.status = 'paid'
  AND o.paid_at >= sqlc.arg(from_ts)::timestamptz AND o.paid_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg(branch_id)::bigint IS NULL OR o.branch_id = sqlc.narg(branch_id)::bigint)
GROUP BY u.id, u.name
ORDER BY revenue DESC, u.id;

-- Shift = the shift that took the payment (paid_shift_id, else the order's shift).
-- name: ReportByShift :many
SELECT s.id                              AS shift_id,
       b.code                            AS branch_code,
       d.name                            AS device_name,
       u.name                            AS opened_by_name,
       s.opened_at,
       COALESCE(SUM(o.total), 0)::bigint AS revenue,
       COUNT(*)::bigint                  AS order_count
FROM orders o
JOIN shifts s   ON s.id = COALESCE(o.paid_shift_id, o.shift_id)
JOIN branches b ON b.id = s.branch_id
JOIN devices d  ON d.id = s.device_id
JOIN users u    ON u.id = s.opened_by
WHERE o.status = 'paid'
  AND o.paid_at >= sqlc.arg(from_ts)::timestamptz AND o.paid_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg(branch_id)::bigint IS NULL OR o.branch_id = sqlc.narg(branch_id)::bigint)
GROUP BY s.id, b.code, d.name, u.name, s.opened_at
ORDER BY s.opened_at, s.id;

-- Every active branch in scope is listed, also those without sales (owner comparison, R-03).
-- name: ReportByBranch :many
SELECT b.id                              AS branch_id,
       b.code                            AS branch_code,
       b.name                            AS branch_name,
       COALESCE(SUM(o.total), 0)::bigint AS revenue,
       COUNT(o.id)::bigint               AS order_count
FROM branches b
LEFT JOIN orders o ON o.branch_id = b.id
    AND o.status = 'paid'
    AND o.paid_at >= sqlc.arg(from_ts)::timestamptz AND o.paid_at < sqlc.arg(to_ts)::timestamptz
WHERE (b.is_active OR o.id IS NOT NULL)
  AND (sqlc.narg(branch_id)::bigint IS NULL OR b.id = sqlc.narg(branch_id)::bigint)
GROUP BY b.id, b.code, b.name
ORDER BY b.id;

-- name: CountInvoicesIssued :one
SELECT COUNT(*)::bigint
FROM invoices i
WHERE i.kind = 'normal'
  AND i.issued_at >= sqlc.arg(from_ts)::timestamptz AND i.issued_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg(branch_id)::bigint IS NULL OR i.branch_id = sqlc.narg(branch_id)::bigint);

-- name: CountInvoicesFailed :one
SELECT COUNT(*)::bigint
FROM invoices i
WHERE i.status = 'failed'
  AND (sqlc.narg(branch_id)::bigint IS NULL OR i.branch_id = sqlc.narg(branch_id)::bigint);

-- name: ShiftsWithCashDiff :many
SELECT s.id, s.cash_diff, s.closed_at, b.code AS branch_code, d.name AS device_name
FROM shifts s
JOIN branches b ON b.id = s.branch_id
JOIN devices d  ON d.id = s.device_id
WHERE s.status = 'closed'
  AND s.closed_at >= sqlc.arg(from_ts)::timestamptz AND s.closed_at < sqlc.arg(to_ts)::timestamptz
  AND abs(COALESCE(s.cash_diff, 0)) > sqlc.arg(threshold)::bigint
  AND (sqlc.narg(branch_id)::bigint IS NULL OR s.branch_id = sqlc.narg(branch_id)::bigint)
ORDER BY s.closed_at, s.id;
