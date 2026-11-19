-- Menu module: categories, products, sizes, modifiers and per-branch sold-out flags.

-- name: MenuListCategories :many
SELECT * FROM categories ORDER BY sort_order, id;

-- name: MenuCreateCategory :one
INSERT INTO categories (name, sort_order) VALUES ($1, $2) RETURNING *;

-- name: MenuUpdateCategory :one
UPDATE categories SET name = $2, sort_order = $3 WHERE id = $1 RETURNING *;

-- name: MenuCategoryExists :one
SELECT EXISTS (SELECT 1 FROM categories WHERE id = $1);

-- name: MenuListProducts :many
SELECT * FROM products
WHERE (NOT sqlc.arg(active_only)::boolean OR is_active)
ORDER BY sort_order, id;

-- name: MenuGetProductForUpdate :one
SELECT * FROM products WHERE id = $1 FOR UPDATE;

-- name: MenuGetProduct :one
SELECT * FROM products WHERE id = $1;

-- name: MenuProductExists :one
SELECT EXISTS (SELECT 1 FROM products WHERE id = $1);

-- name: MenuInsertProduct :one
INSERT INTO products (category_id, name, description, base_price, is_active, is_featured, sort_order, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: MenuUpdateProduct :one
UPDATE products
SET category_id = $2, name = $3, description = $4, base_price = $5,
    is_active = $6, is_featured = $7, sort_order = $8, updated_at = $9
WHERE id = $1
RETURNING *;

-- name: MenuListSizes :many
SELECT * FROM product_sizes
WHERE (sqlc.narg(product_id)::bigint IS NULL OR product_id = sqlc.narg(product_id))
ORDER BY product_id, sort_order, id;

-- name: MenuDeleteSizes :exec
DELETE FROM product_sizes WHERE product_id = $1;

-- name: MenuInsertSize :exec
INSERT INTO product_sizes (product_id, code, name, price_delta, sort_order) VALUES ($1, $2, $3, $4, $5);

-- name: MenuListModifiers :many
SELECT * FROM modifiers
WHERE (sqlc.narg(product_id)::bigint IS NULL OR product_id = sqlc.narg(product_id))
  AND (NOT sqlc.arg(active_only)::boolean OR is_active)
ORDER BY product_id, sort_order, id;

-- name: MenuInsertModifier :one
INSERT INTO modifiers (product_id, name, price, is_active, sort_order) VALUES ($1, $2, $3, true, $4) RETURNING id;

-- name: MenuUpdateModifier :exec
UPDATE modifiers SET name = $2, price = $3, sort_order = $4, is_active = true WHERE id = $1;

-- name: MenuDeactivateModifier :exec
UPDATE modifiers SET is_active = false WHERE id = $1;

-- name: MenuListSoldOut :many
SELECT branch_id, product_id FROM branch_sold_out
WHERE (sqlc.narg(branch_id)::bigint IS NULL OR branch_id = sqlc.narg(branch_id))
ORDER BY product_id, branch_id;

-- name: MenuMarkSoldOut :execrows
INSERT INTO branch_sold_out (branch_id, product_id, marked_by, marked_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (branch_id, product_id) DO NOTHING;

-- name: MenuUnmarkSoldOut :execrows
DELETE FROM branch_sold_out WHERE branch_id = $1 AND product_id = $2;

-- name: MenuBranchExists :one
SELECT EXISTS (SELECT 1 FROM branches WHERE id = $1);
