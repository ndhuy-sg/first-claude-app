-- +goose Up
-- Schema for phase 1 (POS + e-invoice). Money is BIGINT VND. Times are timestamptz (UTC).
-- Business dates use Asia/Ho_Chi_Minh and are computed in Go.

CREATE TYPE user_role AS ENUM ('owner', 'warehouse_manager', 'branch_manager', 'cashier', 'barista');

CREATE TABLE app_settings (
    id                      SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    company_name            TEXT    NOT NULL DEFAULT '',
    company_tax_code        TEXT    NOT NULL DEFAULT '',
    invoice_email_from      TEXT    NOT NULL DEFAULT '',
    tax_submission_enabled  BOOLEAN NOT NULL DEFAULT false,
    tax_gateway             TEXT    NOT NULL DEFAULT 'off' CHECK (tax_gateway IN ('off', 'direct', 'tvan', 'fake')),
    discount_max_pct        INT     NOT NULL DEFAULT 20 CHECK (discount_max_pct BETWEEN 0 AND 100),
    pos_idle_lock_minutes   INT     NOT NULL DEFAULT 15 CHECK (pos_idle_lock_minutes > 0),
    login_max_failures      INT     NOT NULL DEFAULT 5,
    login_lock_minutes      INT     NOT NULL DEFAULT 15,
    shift_note_threshold    BIGINT  NOT NULL DEFAULT 50000,
    prices_include_vat      BOOLEAN NOT NULL DEFAULT true,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO app_settings (id) VALUES (1);

CREATE TABLE branches (
    id                   BIGSERIAL PRIMARY KEY,
    code                 TEXT   NOT NULL UNIQUE,              -- 'CN1'
    name                 TEXT   NOT NULL,
    address              TEXT   NOT NULL DEFAULT '',
    opening_hours        TEXT   NOT NULL DEFAULT '',
    tax_code             TEXT   NOT NULL DEFAULT '',          -- empty = use company tax code
    vat_rate_bp          INT    NOT NULL DEFAULT 800 CHECK (vat_rate_bp BETWEEN 0 AND 10000), -- basis points: 800 = 8%
    vietqr_bank_bin      TEXT   NOT NULL DEFAULT '',
    vietqr_account_no    TEXT   NOT NULL DEFAULT '',
    vietqr_account_name  TEXT   NOT NULL DEFAULT '',
    is_active            BOOLEAN NOT NULL DEFAULT true,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT      NOT NULL,
    email           TEXT      UNIQUE,                         -- NULL for PIN-only staff
    password_hash   TEXT,
    pin_hash        TEXT,
    pin_must_change BOOLEAN   NOT NULL DEFAULT false,
    role            user_role NOT NULL,
    branch_id       BIGINT    REFERENCES branches(id),
    failed_logins   INT       NOT NULL DEFAULT 0,
    locked_until    TIMESTAMPTZ,                              -- temporary lock after failed logins
    disabled_at     TIMESTAMPTZ,                              -- account locked by a manager
    disabled_by     BIGINT    REFERENCES users(id),
    last_active_at  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (role IN ('owner', 'warehouse_manager') OR branch_id IS NOT NULL)
);

CREATE TABLE devices (
    id            BIGSERIAL PRIMARY KEY,
    branch_id     BIGINT NOT NULL REFERENCES branches(id),
    name          TEXT   NOT NULL,
    token_hash    TEXT   NOT NULL UNIQUE,                     -- sha256 hex of the device cookie
    activated_by  BIGINT NOT NULL REFERENCES users(id),
    revoked_at    TIMESTAMPTZ,
    last_seen_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token_hash    TEXT PRIMARY KEY,                           -- sha256 hex of the session cookie
    user_id       BIGINT NOT NULL REFERENCES users(id),
    device_id     BIGINT REFERENCES devices(id),
    kind          TEXT   NOT NULL CHECK (kind IN ('password', 'pin')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ NOT NULL,
    revoked_at    TIMESTAMPTZ
);
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE revoked_at IS NULL;

-- Menu (one menu and one price list for the whole chain)
CREATE TABLE categories (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    sort_order  INT  NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE products (
    id           BIGSERIAL PRIMARY KEY,
    category_id  BIGINT  NOT NULL REFERENCES categories(id),
    name         TEXT    NOT NULL,
    description  TEXT    NOT NULL DEFAULT '',
    base_price   BIGINT  NOT NULL CHECK (base_price >= 0),
    is_active    BOOLEAN NOT NULL DEFAULT true,
    is_featured  BOOLEAN NOT NULL DEFAULT false,
    sort_order   INT     NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (category_id, name)
);

CREATE TABLE product_sizes (
    id           BIGSERIAL PRIMARY KEY,
    product_id   BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    code         TEXT   NOT NULL,                             -- 'M', 'L'
    name         TEXT   NOT NULL,
    price_delta  BIGINT NOT NULL DEFAULT 0 CHECK (price_delta >= 0),
    sort_order   INT    NOT NULL DEFAULT 0,
    UNIQUE (product_id, code)
);

CREATE TABLE modifiers (
    id          BIGSERIAL PRIMARY KEY,
    product_id  BIGINT  NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    price       BIGINT  NOT NULL DEFAULT 0 CHECK (price >= 0),
    is_active   BOOLEAN NOT NULL DEFAULT true,
    sort_order  INT     NOT NULL DEFAULT 0,
    UNIQUE (product_id, name)
);

CREATE TABLE branch_sold_out (
    branch_id   BIGINT NOT NULL REFERENCES branches(id),
    product_id  BIGINT NOT NULL REFERENCES products(id),
    marked_by   BIGINT NOT NULL REFERENCES users(id),
    marked_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (branch_id, product_id)
);

-- Floor plan
CREATE TABLE areas (
    id          BIGSERIAL PRIMARY KEY,
    branch_id   BIGINT NOT NULL REFERENCES branches(id),
    name        TEXT   NOT NULL,
    sort_order  INT    NOT NULL DEFAULT 0,
    UNIQUE (branch_id, name)
);

CREATE TABLE dining_tables (
    id          BIGSERIAL PRIMARY KEY,
    branch_id   BIGINT NOT NULL REFERENCES branches(id),
    area_id     BIGINT NOT NULL REFERENCES areas(id),
    name        TEXT   NOT NULL,
    sort_order  INT    NOT NULL DEFAULT 0,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    UNIQUE (branch_id, name)
);

-- Shifts (one open shift per POS device)
CREATE TABLE shifts (
    id             BIGSERIAL PRIMARY KEY,
    branch_id      BIGINT NOT NULL REFERENCES branches(id),
    device_id      BIGINT NOT NULL REFERENCES devices(id),
    status         TEXT   NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    opened_by      BIGINT NOT NULL REFERENCES users(id),
    opened_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    opening_cash   BIGINT NOT NULL CHECK (opening_cash >= 0),
    closed_by      BIGINT REFERENCES users(id),
    closed_at      TIMESTAMPTZ,
    expected_cash  BIGINT,
    counted_cash   BIGINT,
    cash_diff      BIGINT,
    close_note     TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX shifts_one_open_per_device ON shifts (device_id) WHERE status = 'open';

-- Orders
CREATE TABLE order_counters (
    branch_id      BIGINT NOT NULL REFERENCES branches(id),
    business_date  DATE   NOT NULL,
    last_seq       INT    NOT NULL DEFAULT 0,
    PRIMARY KEY (branch_id, business_date)
);

CREATE TABLE orders (
    id                    BIGSERIAL PRIMARY KEY,
    branch_id             BIGINT NOT NULL REFERENCES branches(id),
    code                  TEXT   NOT NULL UNIQUE,              -- CN1-20261119-0001
    business_date         DATE   NOT NULL,
    seq                   INT    NOT NULL,
    order_type            TEXT   NOT NULL CHECK (order_type IN ('dine_in', 'takeaway')),
    table_id              BIGINT REFERENCES dining_tables(id),
    shift_id              BIGINT NOT NULL REFERENCES shifts(id),
    status                TEXT   NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'paid', 'cancelled', 'merged', 'refunded')),
    subtotal              BIGINT NOT NULL DEFAULT 0,
    discount_type         TEXT   CHECK (discount_type IN ('percent', 'amount')),
    discount_value        BIGINT NOT NULL DEFAULT 0,
    discount_amount       BIGINT NOT NULL DEFAULT 0,
    discount_reason       TEXT   NOT NULL DEFAULT '',
    discount_by           BIGINT REFERENCES users(id),
    discount_approved_by  BIGINT REFERENCES users(id),
    total                 BIGINT NOT NULL DEFAULT 0 CHECK (total >= 0),
    buyer_name            TEXT   NOT NULL DEFAULT '',
    buyer_tax_code        TEXT   NOT NULL DEFAULT '',
    buyer_address         TEXT   NOT NULL DEFAULT '',
    buyer_email           TEXT   NOT NULL DEFAULT '',
    cancel_reason         TEXT   NOT NULL DEFAULT '',
    merged_into_order_id  BIGINT REFERENCES orders(id),
    split_from_order_id   BIGINT REFERENCES orders(id),
    created_by            BIGINT NOT NULL REFERENCES users(id),
    paid_shift_id         BIGINT REFERENCES shifts(id),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at               TIMESTAMPTZ,
    UNIQUE (branch_id, business_date, seq),
    CHECK (order_type = 'takeaway' OR table_id IS NOT NULL OR status <> 'open')
);
CREATE UNIQUE INDEX orders_one_open_per_table ON orders (table_id) WHERE status = 'open' AND table_id IS NOT NULL;
CREATE INDEX orders_branch_status_idx ON orders (branch_id, status);
CREATE INDEX orders_branch_paid_at_idx ON orders (branch_id, paid_at);

CREATE TABLE order_items (
    id            BIGSERIAL PRIMARY KEY,
    order_id      BIGINT NOT NULL REFERENCES orders(id),
    product_id    BIGINT NOT NULL REFERENCES products(id),
    product_name  TEXT   NOT NULL,                            -- snapshot at time of sale
    size_code     TEXT   NOT NULL DEFAULT '',
    size_name     TEXT   NOT NULL DEFAULT '',
    unit_price    BIGINT NOT NULL CHECK (unit_price >= 0),    -- base + size + modifiers, snapshot
    qty           INT    NOT NULL CHECK (qty > 0),
    line_total    BIGINT NOT NULL CHECK (line_total >= 0),
    note          TEXT   NOT NULL DEFAULT '',
    created_by    BIGINT NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX order_items_order_idx ON order_items (order_id);

CREATE TABLE order_item_modifiers (
    order_item_id  BIGINT NOT NULL REFERENCES order_items(id) ON DELETE CASCADE,
    modifier_id    BIGINT NOT NULL REFERENCES modifiers(id),
    name           TEXT   NOT NULL,                           -- snapshot
    price          BIGINT NOT NULL,                           -- snapshot
    PRIMARY KEY (order_item_id, modifier_id)
);

-- Payments
CREATE TABLE payments (
    id             BIGSERIAL PRIMARY KEY,
    order_id       BIGINT NOT NULL REFERENCES orders(id),
    branch_id      BIGINT NOT NULL REFERENCES branches(id),
    shift_id       BIGINT NOT NULL REFERENCES shifts(id),
    method         TEXT   NOT NULL CHECK (method IN ('cash', 'vietqr')),
    status         TEXT   NOT NULL CHECK (status IN ('pending', 'confirmed', 'cancelled')),
    amount         BIGINT NOT NULL CHECK (amount > 0),
    cash_received  BIGINT,
    change_amount  BIGINT,
    qr_payload     TEXT   NOT NULL DEFAULT '',
    created_by     BIGINT NOT NULL REFERENCES users(id),
    confirmed_by   BIGINT REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at   TIMESTAMPTZ,
    CHECK (method <> 'cash' OR (cash_received IS NOT NULL AND cash_received >= amount))
);
CREATE INDEX payments_order_idx ON payments (order_id);
CREATE INDEX payments_shift_idx ON payments (shift_id);

CREATE TABLE refunds (
    id           BIGSERIAL PRIMARY KEY,
    order_id     BIGINT NOT NULL UNIQUE REFERENCES orders(id),
    branch_id    BIGINT NOT NULL REFERENCES branches(id),
    shift_id     BIGINT REFERENCES shifts(id),
    amount       BIGINT NOT NULL CHECK (amount > 0),
    cash_amount  BIGINT NOT NULL DEFAULT 0,                   -- part given back in cash (affects drawer)
    reason       TEXT   NOT NULL,
    created_by   BIGINT NOT NULL REFERENCES users(id),
    approved_by  BIGINT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- E-invoices (self-issued)
CREATE TABLE invoice_series (
    branch_id    BIGINT PRIMARY KEY REFERENCES branches(id),
    symbol       TEXT   NOT NULL,                             -- e.g. 1C26MAA
    next_number  BIGINT NOT NULL DEFAULT 1 CHECK (next_number > 0)
);

CREATE TABLE invoices (
    id                   BIGSERIAL PRIMARY KEY,
    branch_id            BIGINT NOT NULL REFERENCES branches(id),
    order_id             BIGINT NOT NULL REFERENCES orders(id),
    kind                 TEXT   NOT NULL DEFAULT 'normal' CHECK (kind IN ('normal', 'cancel', 'adjust')),
    original_invoice_id  BIGINT REFERENCES invoices(id),
    symbol               TEXT   NOT NULL,
    number               BIGINT NOT NULL,
    lookup_code          TEXT   NOT NULL UNIQUE,
    seller_name          TEXT   NOT NULL,
    seller_tax_code      TEXT   NOT NULL,
    seller_address       TEXT   NOT NULL,
    buyer_name           TEXT   NOT NULL,                     -- 'Khách lẻ' by default
    buyer_tax_code       TEXT   NOT NULL DEFAULT '',
    buyer_address        TEXT   NOT NULL DEFAULT '',
    buyer_email          TEXT   NOT NULL DEFAULT '',
    payment_method       TEXT   NOT NULL,                     -- 'Tiền mặt', 'Chuyển khoản', 'Tiền mặt/Chuyển khoản'
    vat_rate_bp          INT    NOT NULL,
    amount_before_vat    BIGINT NOT NULL,
    vat_amount           BIGINT NOT NULL,
    total                BIGINT NOT NULL,
    status               TEXT   NOT NULL DEFAULT 'not_sent' CHECK (status IN ('not_sent', 'sending', 'sent', 'failed')),
    issued_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at              TIMESTAMPTZ,
    UNIQUE (branch_id, symbol, number)
);
CREATE INDEX invoices_branch_status_idx ON invoices (branch_id, status);
CREATE INDEX invoices_order_idx ON invoices (order_id);

CREATE TABLE invoice_lines (
    invoice_id  BIGINT NOT NULL REFERENCES invoices(id),
    line_no     INT    NOT NULL,
    name        TEXT   NOT NULL,
    qty         INT    NOT NULL,
    unit_price  BIGINT NOT NULL,
    amount      BIGINT NOT NULL,
    PRIMARY KEY (invoice_id, line_no)
);

CREATE TABLE tax_submissions (
    id          BIGSERIAL PRIMARY KEY,
    invoice_id  BIGINT NOT NULL REFERENCES invoices(id),
    attempt     INT    NOT NULL,
    gateway     TEXT   NOT NULL,
    success     BOOLEAN NOT NULL,
    response    TEXT   NOT NULL DEFAULT '',
    error       TEXT   NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Background jobs (worked with FOR UPDATE SKIP LOCKED)
CREATE TABLE jobs (
    id            BIGSERIAL PRIMARY KEY,
    kind          TEXT  NOT NULL,                             -- 'invoice.submit', 'invoice.email', ...
    payload       JSONB NOT NULL DEFAULT '{}'::jsonb,
    dedupe_key    TEXT  UNIQUE,                               -- e.g. 'invoice.submit:123'
    status        TEXT  NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    attempts      INT   NOT NULL DEFAULT 0,
    max_attempts  INT   NOT NULL DEFAULT 10,
    run_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error    TEXT  NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX jobs_ready_idx ON jobs (run_at) WHERE status = 'queued';

-- Idempotency for write requests (Idempotency-Key header)
CREATE TABLE idempotency_keys (
    user_id          BIGINT NOT NULL REFERENCES users(id),
    key              TEXT   NOT NULL,
    method           TEXT   NOT NULL,
    path             TEXT   NOT NULL,
    response_status  INT    NOT NULL,
    response_body    BYTEA  NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, key)
);

-- Append-only audit log
CREATE TABLE audit_logs (
    id           BIGSERIAL PRIMARY KEY,
    at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_id     BIGINT REFERENCES users(id),
    approver_id  BIGINT REFERENCES users(id),
    branch_id    BIGINT REFERENCES branches(id),
    action       TEXT   NOT NULL,                             -- e.g. 'order.discount', 'order.refund', 'product.price'
    entity_type  TEXT   NOT NULL,
    entity_id    TEXT   NOT NULL,
    before       JSONB,
    after        JSONB,
    note         TEXT   NOT NULL DEFAULT ''
);
CREATE INDEX audit_logs_branch_at_idx ON audit_logs (branch_id, at DESC);

-- +goose StatementBegin
CREATE FUNCTION audit_logs_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_logs_no_update BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION audit_logs_append_only();

-- +goose Down
DROP TABLE IF EXISTS audit_logs, idempotency_keys, jobs, tax_submissions, invoice_lines, invoices,
    invoice_series, refunds, payments, order_item_modifiers, order_items, orders, order_counters,
    shifts, dining_tables, areas, branch_sold_out, modifiers, product_sizes, products, categories,
    sessions, devices, users, branches, app_settings CASCADE;
DROP FUNCTION IF EXISTS audit_logs_append_only();
DROP TYPE IF EXISTS user_role;
