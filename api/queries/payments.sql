-- Payments module: payments, VietQR confirmation, refunds.

-- name: PayLockOrder :one
SELECT * FROM orders WHERE id = $1 FOR UPDATE;

-- name: PayCountOrderItems :one
SELECT count(*)::int FROM order_items WHERE order_id = $1;

-- name: PayOrderPaymentSums :one
SELECT
  coalesce(sum(amount) FILTER (WHERE status = 'confirmed'), 0)::bigint AS confirmed,
  coalesce(sum(amount) FILTER (WHERE status = 'pending'), 0)::bigint AS pending,
  coalesce(sum(amount) FILTER (WHERE status = 'confirmed' AND method = 'cash'), 0)::bigint AS confirmed_cash,
  coalesce(sum(amount) FILTER (WHERE status = 'confirmed' AND method = 'vietqr'), 0)::bigint AS confirmed_qr
FROM payments WHERE order_id = $1;

-- name: PayInsertPayment :one
INSERT INTO payments (order_id, branch_id, shift_id, method, status, amount, cash_received, change_amount,
  qr_payload, created_by, confirmed_by, created_at, confirmed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id;

-- name: PayGetPayment :one
SELECT * FROM payments WHERE id = $1;

-- name: PayLockPayment :one
SELECT * FROM payments WHERE id = $1 FOR UPDATE;

-- name: PayConfirmPayment :exec
UPDATE payments SET status = 'confirmed', confirmed_by = $2, confirmed_at = $3, shift_id = $4 WHERE id = $1;

-- name: PayCancelPayment :exec
UPDATE payments SET status = 'cancelled' WHERE id = $1;

-- name: PayMarkOrderPaid :exec
UPDATE orders SET status = 'paid', paid_at = $2, paid_shift_id = $3, updated_at = $2 WHERE id = $1;

-- name: PayMarkOrderRefunded :exec
UPDATE orders SET status = 'refunded', updated_at = $2 WHERE id = $1;

-- name: PayInsertRefund :one
INSERT INTO refunds (order_id, branch_id, shift_id, amount, cash_amount, reason, created_by, approved_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;
