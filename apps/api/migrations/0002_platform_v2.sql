BEGIN;

ALTER TABLE users ADD COLUMN IF NOT EXISTS membership_level VARCHAR(30) NOT NULL DEFAULT 'free';
ALTER TABLE users ADD COLUMN IF NOT EXISTS status VARCHAR(30) NOT NULL DEFAULT 'active';
ALTER TABLE users ADD COLUMN IF NOT EXISTS role VARCHAR(30) NOT NULL DEFAULT 'user';
ALTER TABLE generation_jobs ADD COLUMN IF NOT EXISTS renderer_version VARCHAR(30) NOT NULL DEFAULT '2.0.0';
ALTER TABLE generation_jobs ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS product VARCHAR(40) NOT NULL DEFAULT 'single_export';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS transaction_id VARCHAR(128);

CREATE TABLE IF NOT EXISTS assets (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mime            VARCHAR(40) NOT NULL,
    file_key        TEXT UNIQUE NOT NULL,
    width           INTEGER NOT NULL CHECK (width > 0),
    height          INTEGER NOT NULL CHECK (height > 0),
    sha256          CHAR(64) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_assets_user_created ON assets(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS templates (
    id              UUID PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    category        VARCHAR(60) NOT NULL,
    tags            TEXT[] NOT NULL DEFAULT '{}',
    config          JSONB NOT NULL,
    thumbnail_key   TEXT,
    premium         BOOLEAN NOT NULL DEFAULT FALSE,
    status          VARCHAR(30) NOT NULL DEFAULT 'draft',
    version         INTEGER NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS resource_versions (
    id              UUID PRIMARY KEY,
    resource_type   VARCHAR(30) NOT NULL CHECK (resource_type IN ('font', 'texture')),
    resource_key    VARCHAR(100) NOT NULL,
    version         VARCHAR(40) NOT NULL,
    license_summary TEXT NOT NULL DEFAULT '',
    premium         BOOLEAN NOT NULL DEFAULT FALSE,
    status          VARCHAR(30) NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(resource_type, resource_key, version)
);

CREATE TABLE IF NOT EXISTS user_sessions (
    token_hash      CHAR(64) PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_agent_hash CHAR(64),
    ip_hash         CHAR(64),
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_created ON user_sessions(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_orders_user_created ON orders(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_download_tokens_expiry ON download_tokens(expires_at) WHERE consumed_at IS NULL;

COMMIT;
