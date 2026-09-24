# ADR-001: Migration from SQLite to PostgreSQL Primary Database

## Status
Accepted

## Context
WacallerAPI previously used a single SQLite database file (`wacaller.db`) to store all application data, users, sessions, call logs, messages, and whatsmeow device credentials. SQLite lacks native support for concurrent multi-writer web applications, fine-grained foreign key constraints, array columns, JSONB queries, and high-availability clustering required for multi-tenant enterprise production.

## Decision
1. Adopt **PostgreSQL 16** as the primary relational application database for all control plane data, multi-tenant models, billing, webhooks, audit logs, and domain events.
2. Retain **SQLite** exclusively for `whatsmeow`'s device credential store (`sqlstore.Container`), isolating device crypto state from core application domain data.
3. Manage database migrations using forward-only, embedded SQL files executed automatically at application startup via `golang-migrate/migrate/v4`.

## Consequences
### Positive
- Production-grade concurrent write performance and horizontal connection pooling (`jackc/pgx/v5`).
- Native transaction isolation, JSONB indexing, array columns, and CASCADE foreign key safety.
- Versioned, reproducible database schema migrations embedded directly inside the Go binary.

### Negative / Trade-offs
- Deployment requires a PostgreSQL 16 server (provided via Docker Compose for local development).
- Dual database setup requires maintaining PostgreSQL for domain data and SQLite for whatsmeow device keys.
