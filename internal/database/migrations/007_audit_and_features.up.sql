CREATE TABLE IF NOT EXISTS audit_events (
    id              BIGSERIAL PRIMARY KEY,
    actor_id        TEXT,
    actor_type      TEXT NOT NULL,
    organization_id TEXT,
    action          TEXT NOT NULL,
    target_type     TEXT,
    target_id       TEXT,
    result          TEXT NOT NULL DEFAULT 'success',
    ip_address      TEXT,
    user_agent      TEXT,
    request_id      TEXT,
    metadata        JSONB,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_org ON audit_events(organization_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_events(actor_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS feature_flags (
    id              TEXT PRIMARY KEY,
    name            TEXT UNIQUE NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    description     TEXT,
    scope           TEXT NOT NULL DEFAULT 'global',
    target_org_id   TEXT REFERENCES organizations(id),
    updated_by      TEXT REFERENCES users(id),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
