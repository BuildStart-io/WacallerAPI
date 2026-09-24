# ADR-002: Organization-Based Multi-Tenancy Model

## Status
Accepted

## Context
The legacy WacallerAPI system associated resources directly with individual user accounts (`user_id`). Enterprise clients require team management, shared WhatsApp numbers across developers, organization-level API credentials, billing, and role-based access delegation.

## Decision
1. Introduce **`Organization`** as the primary tenant and ownership boundary for all operational and billable assets:
   - `whatsapp_sessions`
   - `api_credentials`
   - `calls` & `messages`
   - `subscriptions` & `entitlements`
   - `webhook_endpoints` & `webhook_deliveries`
   - `audit_events` & `outbox_events`

2. Introduce **`Membership`** as an explicit join entity linking `Users` to `Organizations` with granular roles (`owner`, `admin`, `developer`, `viewer`).

3. Scope all data queries by `organization_id` to enforce strict isolation between tenants.

## Consequences
### Positive
- Enterprise-ready team management allowing users to collaborate across multiple organizations.
- Granular permission management via organizational membership roles.
- Organization-level billing aggregation and subscription tier enforcement.

### Negative / Trade-offs
- Queries must include `organization_id` foreign key filters.
- API endpoints require tenant context resolving (via API credential or session organization header).
