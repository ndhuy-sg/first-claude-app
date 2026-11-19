-- +goose Up
-- A split bill (O-12) creates a second open order on the same table. Keep the
-- "one open order per table" rule for the table's main order only; split-off
-- orders (split_from_order_id IS NOT NULL) may share the table until paid.
DROP INDEX orders_one_open_per_table;
CREATE UNIQUE INDEX orders_one_open_per_table ON orders (table_id)
    WHERE status = 'open' AND table_id IS NOT NULL AND split_from_order_id IS NULL;
CREATE INDEX orders_branch_business_date_idx ON orders (branch_id, business_date);

-- +goose Down
DROP INDEX IF EXISTS orders_branch_business_date_idx;
DROP INDEX orders_one_open_per_table;
CREATE UNIQUE INDEX orders_one_open_per_table ON orders (table_id) WHERE status = 'open' AND table_id IS NOT NULL;
