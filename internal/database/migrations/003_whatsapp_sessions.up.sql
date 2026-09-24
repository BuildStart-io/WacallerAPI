CREATE TABLE IF NOT EXISTS whatsapp_sessions (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    jid             TEXT,
    phone_number    TEXT,
    status          TEXT NOT NULL DEFAULT 'disconnected',
    desired_state   TEXT NOT NULL DEFAULT 'connected',
    worker_id       TEXT,
    lease_version   BIGINT NOT NULL DEFAULT 0,
    last_heartbeat  TIMESTAMPTZ,
    last_error      TEXT,
    webhook_url     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_wa_sessions_org ON whatsapp_sessions(organization_id);
