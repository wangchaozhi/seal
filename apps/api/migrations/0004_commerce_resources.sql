BEGIN;

CREATE TABLE IF NOT EXISTS refund_requests (
    id          TEXT PRIMARY KEY,
    order_id    TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    reason      TEXT NOT NULL,
    status      VARCHAR(20) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL,
    decided_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_refunds_status_created ON refund_requests(status, created_at DESC);

CREATE TABLE IF NOT EXISTS invoices (
    id          TEXT PRIMARY KEY,
    order_id    TEXT NOT NULL UNIQUE,
    user_id     TEXT NOT NULL,
    title       VARCHAR(120) NOT NULL,
    tax_number  VARCHAR(30),
    email       TEXT NOT NULL,
    status      VARCHAR(20) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS managed_resources (
    id              TEXT PRIMARY KEY,
    resource_type   VARCHAR(20) NOT NULL,
    resource_key    VARCHAR(100) NOT NULL,
    name            VARCHAR(100) NOT NULL,
    version         VARCHAR(40) NOT NULL,
    license_scope   TEXT NOT NULL,
    vip_only        BOOLEAN NOT NULL,
    export_allowed  BOOLEAN NOT NULL,
    status          VARCHAR(20) NOT NULL,
    config          JSONB,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    UNIQUE(resource_type, resource_key, version)
);

COMMIT;
