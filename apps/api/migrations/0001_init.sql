BEGIN;

CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY,
    email           TEXT UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    vip_expires_at  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS seal_configs (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            VARCHAR(100) NOT NULL,
    schema_version  INTEGER NOT NULL,
    config          JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_seal_configs_user_updated ON seal_configs(user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS generation_jobs (
    id              UUID PRIMARY KEY,
    user_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    config_hash     CHAR(64) NOT NULL,
    config          JSONB NOT NULL,
    status          VARCHAR(30) NOT NULL,
    watermark       BOOLEAN NOT NULL,
    output_format   VARCHAR(10) NOT NULL,
    output_size     INTEGER NOT NULL,
    file_key        TEXT,
    failure_reason  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_generation_jobs_user_created ON generation_jobs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_generation_jobs_hash ON generation_jobs(config_hash);

CREATE TABLE IF NOT EXISTS orders (
    id              UUID PRIMARY KEY,
    order_no        VARCHAR(64) UNIQUE NOT NULL,
    user_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    generation_id   UUID REFERENCES generation_jobs(id) ON DELETE SET NULL,
    amount_cents    INTEGER NOT NULL CHECK (amount_cents >= 0),
    status          VARCHAR(30) NOT NULL,
    payment_channel VARCHAR(30),
    paid_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS download_tokens (
    token_hash      CHAR(64) PRIMARY KEY,
    user_id         UUID REFERENCES users(id) ON DELETE CASCADE,
    generation_id   UUID NOT NULL REFERENCES generation_jobs(id) ON DELETE CASCADE,
    expires_at      TIMESTAMPTZ NOT NULL,
    consumed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS audit_events (
    id              BIGSERIAL PRIMARY KEY,
    user_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    event_type      VARCHAR(80) NOT NULL,
    request_id      VARCHAR(64),
    ip_hash         CHAR(64),
    details         JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_events_type_created ON audit_events(event_type, created_at DESC);

COMMIT;
