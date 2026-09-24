# Phase 1 — Domain Model, Database & Architecture Foundation
## Implementation Plan & Execution Summary for WacallerAPI

**Phase Status:** ✅ COMPLETED & VERIFIED
**Phase Goal:** Establish the correct structural foundation — PostgreSQL, multi-tenant domain model, migration framework, repository pattern, and architectural separation of control plane vs. real-time plane — without breaking the existing working WhatsApp/VoIP engine.

---

### Implementation Completion Summary

All 11 tasks planned for Phase 1 have been successfully implemented and verified:

1. **Domain Model (`internal/domain/`)**:
   - `models.go`: Defined 18 core domain structs (`Organization`, `User`, `Membership`, `Identity`, `AuthSession`, `APICredential`, `WhatsAppSession`, `Call`, `Message`, `Subscription`, `Entitlement`, `UsageRecord`, `WebhookEndpoint`, `WebhookDelivery`, `OutboxEvent`, `AuditEvent`, `FeatureFlag`, `StripeEvent`).
   - `errors.go`: Defined domain-level error sentinel constants (`ErrNotFound`, `ErrUnauthorized`, `ErrForbidden`, `ErrConflict`, `ErrInvalidInput`, `ErrRateLimited`, `ErrInternal`).
   - `events.go`: Defined domain event type constants.

2. **PostgreSQL & Migration Framework (`internal/database/`)**:
   - `postgres.go`: Implemented PostgreSQL connection pool setup via `jackc/pgx/v5`, health check, and connection tuning.
   - `migrate.go`: Implemented embedded SQL migration runner using `golang-migrate/migrate/v4`.
   - `migrations/*.sql`: Created 8 versioned migration pairs (16 SQL files) creating 17 PostgreSQL tables with foreign key constraints and indexes.

3. **Configuration Update (`internal/config/config.go` & `.env.example`)**:
   - Added `PostgresDSN`, `RedisURL`, and `WhatsAppDBPath` fields and environment variable loaders (`POSTGRES_DSN`, `REDIS_URL`, `WACALLER_WA_DB`).

4. **Repository Pattern (`internal/database/repository/`)**:
   - Implemented 13 repository implementations and interfaces in domain-separated files (`org_repo.go`, `user_repo.go`, `membership_repo.go`, `auth_repo.go`, `credential_repo.go`, `session_repo.go`, `call_repo.go`, `message_repo.go`, `subscription_repo.go`, `webhook_repo.go`, `outbox_repo.go`, `audit_repo.go`, `feature_repo.go`, `interfaces.go`, `repositories.go`).

5. **Service Layer (`internal/service/`)**:
   - Implemented service structs encapsulating business logic, audit logging, password hashing, and domain rules (`user_service.go`, `org_service.go`, `auth_service.go`, `session_service.go`, `call_service.go`, `message_service.go`, `billing_service.go`, `webhook_service.go`, `audit_service.go`, `services.go`).

6. **Server Wiring & API Adaptation (`cmd/wacaller/main.go` & `internal/api/routes.go`)**:
   - Updated `main.go` to connect to PostgreSQL, run migrations on startup, initialize repositories and services, and attach services to `apiServer`.
   - Preserved `internal/store/store.go` for backwards compatibility and whatsmeow's SQLite device store.

7. **Docker Compose (`docker-compose.yml`)**:
   - Added PostgreSQL 16 Alpine and Redis 7 Alpine containers with healthchecks and volume persistence.

8. **SQLite → PostgreSQL Migration Tool (`cmd/migrate-sqlite-to-pg/main.go`)**:
   - Created automated CLI migration tool to transfer users, API keys, sessions, calls, messages from legacy SQLite `wacaller.db` into PostgreSQL tables under a default organization.

9. **Architecture Documentation (`docs/`)**:
   - Created `docs/architecture/overview.md`
   - Created `docs/adr/ADR-001-postgresql-migration.md`
   - Created `docs/adr/ADR-002-multi-tenancy.md`
   - Created `docs/adr/ADR-003-repository-pattern.md`

10. **Verification**:
   - `go build ./...` — PASS
   - `go test ./...` — PASS
   - `go vet ./...` — PASS

---

## 1. Strategic Context

### What Exists Today (Post-Phase 0)

```
cmd/wacaller/main.go          → Single entry point, monolith
internal/config/config.go      → Flat config from .env/flags
internal/store/store.go        → SQLite monolith (1,265 LOC), all tables + all queries in one file
internal/api/routes.go         → HTTP handler monolith (2,291 LOC), all endpoints in one file
internal/api/middleware/       → Auth, CORS, rate limiting, body size
internal/session/              → WhatsApp session lifecycle, types, call context, bridge
internal/voip/                 → VoIP engine (PRESERVED, DO NOT TOUCH)
internal/wa/                   → WhatsApp socket (PRESERVED, DO NOT TOUCH)
internal/audio/                → WAV encode/decode
internal/webhook/              → Best-effort webhook dispatcher
internal/safenet/              → SSRF protection (Phase 0)
internal/web/                  → Embedded HTML dashboard
```

### Key Architectural Problems This Phase Solves

| Problem | Current State | Target State |
|---|---|---|
| **Database** | SQLite, single file, no migrations | PostgreSQL with versioned migrations |
| **Data Model** | User → Sessions (flat) | User → Organization → Resources (multi-tenant) |
| **Store Layer** | 1,265 LOC monolith, raw SQL in methods | Repository pattern, domain-separated files |
| **API Layer** | 2,291 LOC single file, SQL in handlers | Service layer between handlers and repositories |
| **WhatsApp Store** | Commingled with app data in same SQLite | Separated into its own store |
| **Config** | Flat struct, SQLite-only | PostgreSQL connection string, Redis URL, feature flags |
| **No Organizations** | User-scoped ownership | Organization-scoped multi-tenancy |
| **No Memberships** | One user = one set of resources | Users can belong to multiple orgs with roles |
| **No Audit Trail** | Zero audit logging | Immutable audit events table |
| **No Outbox** | Webhooks fired inline, can be lost | Transactional outbox for durable event delivery |

---

## 2. Architecture Decisions

### 2.1 Database Strategy: Dual-Database

```
PostgreSQL (primary application database)
├── organizations
├── memberships
├── users
├── identities
├── auth_sessions
├── api_credentials
├── whatsapp_sessions (metadata only)
├── calls
├── messages
├── subscriptions
├── entitlements
├── usage_records
├── webhook_endpoints
├── webhook_deliveries
├── outbox_events
├── audit_events
├── feature_flags
└── stripe_events

SQLite (WhatsApp device store — managed by whatsmeow)
└── whatsmeow device/session credentials
```

**Rationale:** The enterprise architecture plan mandates PostgreSQL as the primary application database for horizontal scalability, proper foreign keys, and production-grade constraints. The whatsmeow library uses its own `sqlstore.Container` which currently wraps our SQLite DB — we keep that separate because whatsmeow manages its own schema.

### 2.2 Migration Strategy: Forward-Only Versioned Migrations

Using `golang-migrate/migrate` with embedded SQL migration files:

```
internal/database/migrations/
├── 001_initial_schema.up.sql       / 001_initial_schema.down.sql
├── 002_organizations.up.sql        / 002_organizations.down.sql
├── 003_auth_and_identity.up.sql    / 003_auth_and_identity.down.sql
├── 004_whatsapp_sessions.up.sql    / 004_whatsapp_sessions.down.sql
├── 005_messaging_and_calls.up.sql  / 005_messaging_and_calls.down.sql
├── 006_billing_and_entitlements.up.sql / 006_billing_and_entitlements.down.sql
├── 007_webhooks_and_outbox.up.sql  / 007_webhooks_and_outbox.down.sql
└── 008_audit_and_features.up.sql   / 008_audit_and_features.down.sql
```

**Failed migrations abort startup.** No swallowed errors.

### 2.3 Repository Pattern: Domain-Separated Store Files

Replace the monolithic `internal/store/store.go` with domain-specific repositories behind interfaces:

```
internal/database/
├── postgres.go              → Connection pool, health check, statement timeout
├── migrate.go               → Migration runner
├── migrations/              → SQL migration files (embedded)
└── repository/
    ├── user_repo.go         → User CRUD
    ├── org_repo.go          → Organization CRUD
    ├── membership_repo.go   → Membership CRUD
    ├── auth_repo.go         → Auth sessions, identities
    ├── credential_repo.go   → API credentials (replaces api_keys)
    ├── session_repo.go      → WhatsApp session metadata
    ├── call_repo.go         → Call records
    ├── message_repo.go      → Message records
    ├── subscription_repo.go → Subscriptions + entitlements
    ├── webhook_repo.go      → Webhook endpoints + deliveries
    ├── outbox_repo.go       → Transactional outbox events
    ├── audit_repo.go        → Audit event logging
    └── feature_repo.go      → Feature flags
```

### 2.4 Service Layer: Decouple HTTP Handlers from Data Access

Per the enterprise architecture rule: *"No direct SQL from HTTP handlers"*. Introduce application services:

```
internal/service/
├── user_service.go           → User lifecycle, profile updates
├── org_service.go            → Organization create/update/delete, member management
├── auth_service.go           → Login, register, session management (prepares for Phase 2)
├── session_service.go        → WhatsApp session CRUD metadata (delegates real-time ops to manager)
├── call_service.go           → Call lifecycle metadata
├── message_service.go        → Message lifecycle metadata
├── billing_service.go        → Subscription/entitlement queries
├── webhook_service.go        → Webhook endpoint CRUD
└── audit_service.go          → Audit event recording
```

**Important:** Services wrap repositories with business logic. HTTP handlers call services. Services call repositories. This phase establishes the pattern; not all services need full implementation yet.

---

## 3. Domain Model (PostgreSQL Tables)

### `organizations`
The primary tenant boundary. All billable/operational resources are owned by an organization.

```sql
CREATE TABLE organizations (
    id          TEXT PRIMARY KEY,        -- org_xxxxxxxxxxxxx
    name        TEXT NOT NULL,
    slug        TEXT UNIQUE NOT NULL,    -- URL-safe identifier
    status      TEXT NOT NULL DEFAULT 'active',  -- active, suspended, deleted
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `users`

```sql
CREATE TABLE users (
    id              TEXT PRIMARY KEY,        -- usr_xxxxxxxxxxxxx
    email           TEXT UNIQUE NOT NULL,
    name            TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'developer',  -- developer, superadmin
    password_hash   TEXT NOT NULL,
    password_version INTEGER NOT NULL DEFAULT 2,
    email_verified  BOOLEAN NOT NULL DEFAULT FALSE,
    status          TEXT NOT NULL DEFAULT 'active',     -- active, suspended, deleted
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `memberships`

```sql
CREATE TABLE memberships (
    id              TEXT PRIMARY KEY,        -- mem_xxxxxxxxxxxxx
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL DEFAULT 'developer',  -- owner, admin, developer, viewer
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, organization_id)
);
```

### `identities`
Supports multiple auth providers per user (email/password, Google OIDC in Phase 2).

```sql
CREATE TABLE identities (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    TEXT NOT NULL,            -- 'email', 'google'
    provider_id TEXT NOT NULL,            -- email address or Google sub
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_id)
);
```

### `auth_sessions`
Server-side browser sessions (replaces PAT-in-localStorage pattern).

```sql
CREATE TABLE auth_sessions (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash      TEXT UNIQUE NOT NULL,
    ip_address      TEXT,
    user_agent      TEXT,
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_auth_sessions_token ON auth_sessions(token_hash);
CREATE INDEX idx_auth_sessions_user ON auth_sessions(user_id);
```

### `api_credentials`
Scoped, expiring API keys (replaces flat `api_keys` table).

```sql
CREATE TABLE api_credentials (
    id              TEXT PRIMARY KEY,        -- cred_xxxxxxxxxxxxx
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    key_hash        TEXT UNIQUE NOT NULL,
    key_prefix      TEXT NOT NULL,            -- first 8 chars for identification
    scopes          TEXT[] NOT NULL DEFAULT '{}',
    created_by      TEXT NOT NULL REFERENCES users(id),
    expires_at      TIMESTAMPTZ,
    last_used_at    TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `whatsapp_sessions`
Metadata about WhatsApp sessions (actual device credentials stay in whatsmeow's SQLite).

```sql
CREATE TABLE whatsapp_sessions (
    id              TEXT PRIMARY KEY,        -- was_xxxxxxxxxxxxx
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    jid             TEXT,
    phone_number    TEXT,
    status          TEXT NOT NULL DEFAULT 'disconnected',
    desired_state   TEXT NOT NULL DEFAULT 'connected',
    worker_id       TEXT,
    lease_version   INTEGER NOT NULL DEFAULT 0,
    last_heartbeat  TIMESTAMPTZ,
    last_error      TEXT,
    webhook_url     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_wa_sessions_org ON whatsapp_sessions(organization_id);
```

### `calls`

```sql
CREATE TABLE calls (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    session_id      TEXT NOT NULL REFERENCES whatsapp_sessions(id) ON DELETE CASCADE,
    direction       TEXT NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    peer_number     TEXT NOT NULL,
    status          TEXT NOT NULL,
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ NOT NULL,
    connected_at    TIMESTAMPTZ,
    ended_at        TIMESTAMPTZ,
    end_reason      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_calls_session ON calls(session_id, started_at DESC);
CREATE INDEX idx_calls_org ON calls(organization_id, started_at DESC);
```

### `messages`

```sql
CREATE TABLE messages (
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
CREATE INDEX idx_messages_session ON messages(session_id, timestamp DESC);
CREATE INDEX idx_messages_org ON messages(organization_id, timestamp DESC);
```

### `subscriptions`

```sql
CREATE TABLE subscriptions (
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
CREATE INDEX idx_subscriptions_org ON subscriptions(organization_id);
CREATE INDEX idx_subscriptions_stripe ON subscriptions(stripe_subscription_id);
```

### `entitlements`

```sql
CREATE TABLE entitlements (
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
```

### `usage_records`

```sql
CREATE TABLE usage_records (
    id              BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    metric          TEXT NOT NULL,
    count           INTEGER NOT NULL DEFAULT 1,
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_usage_org_period ON usage_records(organization_id, metric, period_start);
```

### `webhook_endpoints`

```sql
CREATE TABLE webhook_endpoints (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    url             TEXT NOT NULL,
    secret          TEXT NOT NULL,
    events          TEXT[] NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `webhook_deliveries`

```sql
CREATE TABLE webhook_deliveries (
    id              TEXT PRIMARY KEY,
    endpoint_id     TEXT NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    event_id        TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    attempt_count   INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 5,
    next_attempt_at TIMESTAMPTZ,
    last_status_code INTEGER,
    last_error      TEXT,
    payload         JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at    TIMESTAMPTZ
);
CREATE INDEX idx_webhook_deliveries_pending ON webhook_deliveries(status, next_attempt_at)
    WHERE status IN ('pending', 'retry_wait');
```

### `outbox_events`

```sql
CREATE TABLE outbox_events (
    id              BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL,
    aggregate_type  TEXT NOT NULL,
    aggregate_id    TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    schema_version  INTEGER NOT NULL DEFAULT 1,
    payload         JSONB NOT NULL,
    published       BOOLEAN NOT NULL DEFAULT FALSE,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ
);
CREATE INDEX idx_outbox_unpublished ON outbox_events(published, occurred_at)
    WHERE published = FALSE;
```

### `audit_events`

```sql
CREATE TABLE audit_events (
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
CREATE INDEX idx_audit_org ON audit_events(organization_id, occurred_at DESC);
CREATE INDEX idx_audit_actor ON audit_events(actor_id, occurred_at DESC);
```

### `feature_flags`

```sql
CREATE TABLE feature_flags (
    id              TEXT PRIMARY KEY,
    name            TEXT UNIQUE NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    description     TEXT,
    scope           TEXT NOT NULL DEFAULT 'global',
    target_org_id   TEXT REFERENCES organizations(id),
    updated_by      TEXT REFERENCES users(id),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `stripe_events` (carried from Phase 0)

```sql
CREATE TABLE stripe_events (
    event_id        TEXT PRIMARY KEY,
    event_type      TEXT NOT NULL,
    processed_at    TIMESTAMPTZ NOT NULL
);
```

---

## 4. Files to Create (New)

| File | Purpose |
|---|---|
| `internal/domain/models.go` | Core domain model structs |
| `internal/domain/errors.go` | Domain-level error types |
| `internal/domain/events.go` | Domain event type definitions |
| `internal/database/postgres.go` | PostgreSQL connection pool, health checks |
| `internal/database/migrate.go` | Migration runner with embedded SQL |
| `internal/database/migrations/*.sql` | 8 versioned migration pairs (16 files) |
| `internal/database/repository/user_repo.go` | User CRUD |
| `internal/database/repository/org_repo.go` | Organization CRUD |
| `internal/database/repository/membership_repo.go` | Membership CRUD |
| `internal/database/repository/auth_repo.go` | Auth session CRUD |
| `internal/database/repository/credential_repo.go` | API credential management |
| `internal/database/repository/session_repo.go` | WhatsApp session metadata CRUD |
| `internal/database/repository/call_repo.go` | Call record CRUD |
| `internal/database/repository/message_repo.go` | Message record CRUD |
| `internal/database/repository/subscription_repo.go` | Subscription/entitlement CRUD |
| `internal/database/repository/webhook_repo.go` | Webhook endpoint/delivery CRUD |
| `internal/database/repository/outbox_repo.go` | Outbox event writes and polling |
| `internal/database/repository/audit_repo.go` | Audit event insertion (append-only) |
| `internal/database/repository/feature_repo.go` | Feature flag CRUD |
| `internal/service/user_service.go` | User business logic |
| `internal/service/org_service.go` | Organization + membership business logic |
| `internal/service/session_service.go` | WhatsApp session metadata service |
| `internal/service/billing_service.go` | Subscription/entitlement queries |
| `internal/service/audit_service.go` | Audit event recording |
| `docs/architecture/overview.md` | Architecture overview |
| `docs/adr/ADR-001-postgresql-migration.md` | ADR: SQLite → PostgreSQL |
| `docs/adr/ADR-002-multi-tenancy.md` | ADR: Organization-based multi-tenancy |
| `docs/adr/ADR-003-repository-pattern.md` | ADR: Repository/service layer separation |

---

## 5. Files to Modify (Existing)

| File | Changes |
|---|---|
| `cmd/wacaller/main.go` | Add PostgreSQL initialization, migration CLI, service wiring |
| `internal/config/config.go` | Add `PostgresDSN`, `RedisURL`, `WhatsAppDBPath` |
| `internal/store/store.go` | Preserve but deprecate — keep for whatsmeow SQLite only |
| `internal/api/routes.go` | Update `API` struct to accept service layer |
| `internal/api/routes_test.go` | Update test setup for PostgreSQL |
| `internal/session/manager.go` | Accept session repository interface |
| `internal/session/session.go` | Use session repository interface |
| `.env.example` | Add `POSTGRES_DSN`, `REDIS_URL`, `WACALLER_WA_DB` |
| `go.mod` | Add `jackc/pgx/v5`, `golang-migrate/migrate/v4` |
| `docker-compose.yml` | Add PostgreSQL + Redis services |

---

## 6. Files NOT Modified (Preserved)

Per the enterprise architecture Rule 1: **"Do not rewrite the VoIP engine unnecessarily."**

| Package | Reason |
|---|---|
| `internal/voip/**` | Core VoIP engine — no changes needed |
| `internal/wa/**` | WhatsApp socket layer — no changes needed |
| `internal/audio/**` | WAV encode/decode — no changes needed |
| `internal/safenet/**` | SSRF protection — no changes needed |
| `internal/web/**` | Embedded dashboard — replaced in Phase 8 |

---

## 7. Implementation Tasks (Ordered)

### Task 1 — Create Domain Model (`internal/domain/`)
Define all core domain structs, error types, and event types as Go structs.

### Task 2 — Create PostgreSQL Connection & Migration Framework (`internal/database/`)
Set up PostgreSQL connection pooling, health checks, and the migration runner.

### Task 3 — Update Config (`internal/config/config.go`)
Add PostgreSQL DSN, Redis URL, and separate WhatsApp DB path.

### Task 4 — Implement Core Repositories (`internal/database/repository/`)
Implement repository structs against PostgreSQL for all domain entities.

### Task 5 — Implement Service Layer (`internal/service/`)
Create service structs wrapping repositories with business logic.

### Task 6 — Wire Up `main.go`
Initialize PostgreSQL, run migrations, construct repos → services → API.

### Task 7 — Adapt API Layer (`internal/api/routes.go`)
Update the API struct to accept the service layer.

### Task 8 — Update Docker Compose
Add PostgreSQL and Redis services.

### Task 9 — Data Migration Strategy
Create a one-time SQLite → PostgreSQL data migration tool.

### Task 10 — Architecture Documentation
Create ADRs and architecture overview documents.

### Task 11 — Tests & Verification
Build, test, vet — all must pass.

---

## 8. Dependency Changes

| Dependency | Purpose |
|---|---|
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/golang-migrate/migrate/v4` | Migration framework |
| `github.com/google/uuid` | ID generation (promote from indirect) |

---

## 9. Backward Compatibility Strategy

1. PostgreSQL becomes the primary application database
2. SQLite remains **only** for whatsmeow's device store
3. The existing `internal/store/store.go` remains compilable but deprecated
4. New code uses `internal/database/repository/` exclusively
5. A data migration tool handles one-time SQLite → PostgreSQL transfer
6. Full SQLite app-data removal in a later phase

---

## 10. Acceptance Gate

Before proceeding to Phase 2:

- [ ] PostgreSQL migrations run cleanly on a fresh database
- [ ] All 17+ tables created with correct FKs, indexes, and constraints
- [ ] Domain model structs match database schema
- [ ] Repository CRUD operations work against PostgreSQL
- [ ] Service layer wraps repositories correctly
- [ ] `cmd/wacaller/main.go` starts successfully with PostgreSQL
- [ ] WhatsApp device store remains in separate SQLite
- [ ] Data migration tool works for existing deployments
- [ ] Docker Compose starts PostgreSQL + Redis + API cleanly
- [ ] `go build ./...` succeeds
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] Architecture documentation (ADRs) created

---

## 11. Rollback Strategy

If Phase 1 causes issues:
1. Set `WACALLER_DB` back to the original SQLite path
2. The old `internal/store/store.go` still works against SQLite
3. Remove PostgreSQL from Docker Compose
4. Application falls back to Phase 0 state
5. No data loss — SQLite was never deleted

---

## 12. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Data loss during migration | Medium | Critical | Migration runs on copy first; backup SQLite |
| WA device store corruption | Low | Critical | Device store stays in its own SQLite |
| PG connection failures | Low | Medium | Retry logic + clear error messages |
| Existing tests break | Medium | Low | Keep `store.Store` compilable during transition |
