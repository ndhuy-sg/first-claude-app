-- Orders module (tables floor plan, orders, items, discount, move/merge/split/cancel).
-- Query names start with "Ord" so they never clash with other modules in package store.

-- name: OrdOpenShiftForDevice :one
SELECT * FROM shifts WHERE device_id = $1 AND status = 'open';

-- name: OrdBranchCode :one
SELECT code FROM branches WHERE id = $1;

-- name: OrdLockTable :one
SELECT * FROM dining_tables WHERE id = $1 FOR UPDATE;

-- name: OrdCountOpenOrdersOnTable :one
SELECT count(*)::int FROM orders WHERE table_id = $1 AND status = 'open';

-- name: OrdNextSeq :one
INSERT INTO order_counters (branch_id, business_date, last_seq) VALUES ($1, $2, 1)
ON CONFLICT (branch_id, business_date) DO UPDATE SET last_seq = order_counters.last_seq + 1
RETURNING last_seq;

-- name: OrdCreate :one
INSERT INTO orders (branch_id, code, business_date, seq, order_type, table_id, shift_id, split_from_order_id,
                    created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
RETURNING id;

-- name: OrdGetForUpdate :one
SELECT * FROM orders WHERE id = $1 FOR UPDATE;

-- name: OrdGet :one
SELECT * FROM orders WHERE id = $1;

-- name: OrdConfirmedPaid :one
SELECT COALESCE(sum(amount), 0)::bigint FROM payments WHERE order_id = $1 AND status = 'confirmed';

-- name: OrdCancelPendingPayments :exec
UPDATE payments SET status = 'cancelled' WHERE order_id = $1 AND status = 'pending';

-- name: OrdGetProduct :one
SELECT * FROM products WHERE id = $1;

-- name: OrdProductSizes :many
SELECT * FROM product_sizes WHERE product_id = $1 ORDER BY sort_order, id;

-- name: OrdModifiersByIDs :many
SELECT * FROM modifiers WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: OrdIsSoldOut :one
SELECT EXISTS (SELECT 1 FROM branch_sold_out WHERE branch_id = $1 AND product_id = $2);

-- name: OrdInsertItem :one
INSERT INTO order_items (order_id, product_id, product_name, size_code, size_name, unit_price, qty, line_total, note, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: OrdInsertItemModifier :exec
INSERT INTO order_item_modifiers (order_item_id, modifier_id, name, price) VALUES ($1, $2, $3, $4);

-- name: OrdGetItem :one
SELECT * FROM order_items WHERE id = $1 AND order_id = $2;

-- name: OrdUpdateItem :exec
UPDATE order_items SET qty = $2, note = $3, line_total = $4 WHERE id = $1;

-- name: OrdDeleteItem :exec
DELETE FROM order_items WHERE id = $1;

-- name: OrdItemsSubtotal :one
SELECT COALESCE(sum(line_total), 0)::bigint FROM order_items WHERE order_id = $1;

-- name: OrdCountItems :one
SELECT count(*)::int FROM order_items WHERE order_id = $1;

-- name: OrdUpdateTotals :exec
UPDATE orders SET subtotal = $2, discount_amount = $3, total = $4, updated_at = $5 WHERE id = $1;

-- name: OrdSetDiscount :exec
UPDATE orders SET discount_type = $2, discount_value = $3, discount_reason = $4, discount_by = $5,
                  discount_approved_by = $6, updated_at = $7
WHERE id = $1;

-- name: OrdSetBuyer :exec
UPDATE orders SET buyer_name = $2, buyer_tax_code = $3, buyer_address = $4, buyer_email = $5, updated_at = $6
WHERE id = $1;

-- name: OrdSetTable :exec
UPDATE orders SET table_id = $2, order_type = 'dine_in', updated_at = $3 WHERE id = $1;

-- name: OrdMoveAllItems :exec
UPDATE order_items SET order_id = sqlc.arg(to_order_id) WHERE order_id = sqlc.arg(from_order_id);

-- name: OrdMoveItems :execrows
UPDATE order_items SET order_id = sqlc.arg(to_order_id)
WHERE order_id = sqlc.arg(from_order_id) AND id = ANY(sqlc.arg(item_ids)::bigint[]);

-- name: OrdMarkMerged :exec
UPDATE orders SET status = 'merged', merged_into_order_id = $2, subtotal = 0, discount_amount = 0, total = 0, updated_at = $3
WHERE id = $1;

-- name: OrdCancel :exec
UPDATE orders SET status = 'cancelled', cancel_reason = $2, updated_at = $3 WHERE id = $1;

-- name: OrdListIDs :many
SELECT o.id FROM orders o
LEFT JOIN dining_tables t ON t.id = o.table_id
WHERE (sqlc.narg(branch_id)::bigint IS NULL OR o.branch_id = sqlc.narg(branch_id))
  AND o.business_date = sqlc.arg(business_date)
  AND (sqlc.narg(status)::text IS NULL OR o.status = sqlc.narg(status))
  AND (sqlc.narg(shift_id)::bigint IS NULL OR o.shift_id = sqlc.narg(shift_id) OR o.paid_shift_id = sqlc.narg(shift_id))
  AND (sqlc.narg(q)::text IS NULL
       OR o.code ILIKE '%' || sqlc.narg(q) || '%'
       OR t.name ILIKE '%' || sqlc.narg(q) || '%'
       OR o.total::text = sqlc.narg(q))
ORDER BY o.id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: OrdListCount :one
SELECT count(*) FROM orders o
LEFT JOIN dining_tables t ON t.id = o.table_id
WHERE (sqlc.narg(branch_id)::bigint IS NULL OR o.branch_id = sqlc.narg(branch_id))
  AND o.business_date = sqlc.arg(business_date)
  AND (sqlc.narg(status)::text IS NULL OR o.status = sqlc.narg(status))
  AND (sqlc.narg(shift_id)::bigint IS NULL OR o.shift_id = sqlc.narg(shift_id) OR o.paid_shift_id = sqlc.narg(shift_id))
  AND (sqlc.narg(q)::text IS NULL
       OR o.code ILIKE '%' || sqlc.narg(q) || '%'
       OR t.name ILIKE '%' || sqlc.narg(q) || '%'
       OR o.total::text = sqlc.narg(q));

-- name: OrdFloorAreas :many
SELECT * FROM areas WHERE branch_id = $1 ORDER BY sort_order, id;

-- name: OrdFloorTables :many
SELECT * FROM dining_tables WHERE branch_id = $1 AND is_active ORDER BY sort_order, id;

-- name: OrdFloorOpenOrders :many
SELECT o.id, o.code, o.table_id, o.total, o.created_at, o.split_from_order_id,
       COALESCE((SELECT sum(i.qty) FROM order_items i WHERE i.order_id = o.id), 0)::int AS item_count,
       EXISTS (SELECT 1 FROM payments p WHERE p.order_id = o.id AND p.method = 'vietqr' AND p.status = 'pending') AS has_pending_qr
FROM orders o
WHERE o.branch_id = $1 AND o.status = 'open' AND o.table_id IS NOT NULL
ORDER BY o.id;

-- name: OrdGetUser :one
SELECT id, name, role, branch_id, pin_hash, disabled_at FROM users WHERE id = $1;
