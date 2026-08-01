BEGIN;

-- The runtime store keeps the domain snapshot transactionally in PostgreSQL.
-- Normalized tables from prior migrations remain available for reporting and
-- a future online decomposition without changing the API contract.
CREATE TABLE IF NOT EXISTS platform_state (
    id          SMALLINT PRIMARY KEY CHECK (id = 1),
    data        JSONB NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;
