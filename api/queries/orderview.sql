-- Shared read model for the Order JSON (used by orders and payments modules).

-- name: OrderViewHeader :one
SELECT o.*, t.name AS table_name,
       cu.name AS created_by_name,
       au.name AS approved_by_name
FROM orders o
LEFT JOIN dining_tables t ON t.id = o.table_id
JOIN users cu ON cu.id = o.created_by
LEFT JOIN users au ON au.id = o.discount_approved_by
WHERE o.id = $1;

-- name: OrderViewItems :many
SELECT * FROM order_items WHERE order_id = $1 ORDER BY id;

-- name: OrderViewItemModifiers :many
SELECT m.* FROM order_item_modifiers m
JOIN order_items i ON i.id = m.order_item_id
WHERE i.order_id = $1
ORDER BY m.order_item_id, m.modifier_id;

-- name: OrderViewPayments :many
SELECT * FROM payments WHERE order_id = $1 ORDER BY id;

-- name: OrderViewInvoice :one
SELECT * FROM invoices WHERE order_id = $1 AND kind = 'normal' ORDER BY id DESC LIMIT 1;
