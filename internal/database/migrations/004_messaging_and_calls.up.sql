CREATE TABLE IF NOT EXISTS calls (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES organizations(id),
    session_id       TEXT NOT NULL REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,
    direction        TEXT NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    peer_number      TEXT NOT NULL,
    status           TEXT NOT NULL,
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    started_at       TIMESTAMPTZ NOT NULL,
    connected_at     TIMESTAMPTZ,
    ended_at         TIMESTAMPTZ,
    end_reason       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_calls_session ON calls(session_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_calls_org ON calls(organization_id, started_at DESC);

CREATE TABLE IF NOT EXISTS messages (
    id              BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    session_id      TEXT NOT NULL REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,
    message_id      TEXT NOT NULL,
    direction       TEXT NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    peer_number     TEXT NOT NULL,
    msg_type        TEXT NOT NULL,
    content         TEXT,
    media_url       TEXT,
    status          TEXT NOT NULL,
    timestamp       TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_messages_org ON messages(organization_id, timestamp DESC);
