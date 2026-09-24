# WacallerAPI Architecture Overview (Post-Phase 1)

## System Architecture

WacallerAPI is an enterprise-grade multi-tenant WhatsApp VoIP and Messaging Gateway built with Go.

```
                  ┌─────────────────────────────────────────┐
                  │            HTTP / REST Clients          │
                  └────────────────────┬────────────────────┘
                                       │
                                       ▼
                  ┌─────────────────────────────────────────┐
                  │       API Gateway Layer (Gin/Mux)       │
                  │  - Auth Middleware (JWT/API Keys/PAT)   │
                  │  - Rate Limiter, SSRF Protection        │
                  └────────────────────┬────────────────────┘
                                       │
                                       ▼
                  ┌─────────────────────────────────────────┐
                  │              Service Layer              │
                  │  - OrgService     - UserService         │
                  │  - AuthService    - SessionService      │
                  │  - CallService   - MessageService       │
                  │  - BillingService - WebhookService      │
                  └──────────┬──────────────────┬───────────┘
                             │                  │
        Control Plane Data   │                  │ Real-Time Audio / Socket Ops
                             ▼                  ▼
      ┌───────────────────────────┐    ┌───────────────────────────┐
      │     Repository Layer      │    │    WhatsApp & VoIP Engine │
      │  (PostgreSQL Primary DB)  │    │ (whatsmeow + Pion WebRTC) │
      └─────────────┬─────────────┘    └─────────────┬─────────────┘
                    │                                │
                    ▼                                ▼
         ┌─────────────────────┐          ┌────────────────────┐
         │ PostgreSQL 16 DB    │          │  SQLite Device DB  │
         │ (Application Data)  │          │ (whatsmeow store)  │
         └─────────────────────┘          └────────────────────┘
```

## Core Design Principles

1. **Dual-Database Strategy**:
   - **PostgreSQL 16**: Operates as the primary enterprise application database, housing multi-tenant domain models, users, organizations, calls, messages, subscriptions, entitlements, outbox events, and audit trails.
   - **SQLite**: Dedicated strictly to whatsmeow's internal device key and session state container (`sqlstore.Container`).

2. **Organization-First Multi-Tenancy**:
   - Every operational resource (WhatsApp sessions, calls, messages, webhooks, API credentials) is strictly scoped to an `Organization`.
   - Users belong to organizations via `Memberships` with role-based access controls (`owner`, `admin`, `developer`, `viewer`).

3. **Clean Architecture & Separation of Concerns**:
   - **Domain**: Pure structs, errors, and event definitions in `internal/domain/`.
   - **Database / Repository**: Versioned migrations via `golang-migrate` and repository interfaces in `internal/database/repository/`.
   - **Service Layer**: Business logic encapsulation in `internal/service/`.
   - **HTTP API**: Handlers consume services rather than running direct SQL queries.

4. **Real-Time Plane Preservation**:
   - The core WhatsApp Web Socket (`internal/wa/`) and VoIP Media Engine (`internal/voip/`) remain untouched, guaranteeing uninterrupted call quality and WebSocket streaming.
