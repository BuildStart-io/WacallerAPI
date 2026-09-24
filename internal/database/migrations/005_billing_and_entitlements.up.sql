CREATE TABLE IF NOT EXISTS subscriptions (
    id                      TEXT PRIMARY KEY,
    organization_id         TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    plan                    TEXT NOT NULL DEFAULT 'basic',
    status                  TEXT NOT NULL DEFAULT 'active',
    stripe_customer_id      TEXT,
    stripe_subscription_id  TEXT,
    stripe_session_id       TEXT,
    amount_cents            INTEGER NOT NULL DEFAULT 0,
    currency                TEXT NOT NULL DEFAULT 'usd',
    current_period_start    TIMESTAMPTZ,
    current_period_end      TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_subscriptions_org ON subscriptions(organization_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_stripe ON subscriptions(stripe_subscription_id);

CREATE TABLE IF NOT EXISTS entitlements (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    feature         TEXT NOT NULL,
    limit_value     INTEGER NOT NULL,
    source          TEXT NOT NULL DEFAULT 'subscription',
    override_reason TEXT,
    override_by     TEXT REFERENCES users(id),
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(organization_id, feature, source)
);

CREATE TABLE IF NOT EXISTS usage_records (
    id              BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    metric          TEXT NOT NULL,
    count           INTEGER NOT NULL DEFAULT 1,
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_usage_org_period ON usage_records(organization_id, metric, period_start);
