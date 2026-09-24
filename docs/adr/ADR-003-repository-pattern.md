# ADR-003: Repository Pattern & Service Layer Architectural Separation

## Status
Accepted

## Context
In the original implementation, database queries were embedded directly inside HTTP handler functions in `routes.go` and in a monolithic `store.go` file (over 1,200 LOC). This created tight coupling between transport protocols, database drivers, and business logic.

## Decision
1. Adopt the **Repository Pattern** (`internal/database/repository/`):
   - Define Go interface contracts for each domain aggregate (`OrgRepository`, `UserRepository`, `SessionRepository`, `CallRepository`, etc.).
   - Encapsulate all raw SQL execution, row scanning, and driver error translation behind concrete repository implementations.

2. Introduce an explicit **Service Layer** (`internal/service/`):
   - Wrap repositories with domain rules, permission validation, password hashing, ID generation, and audit logging.
   - HTTP handlers interact exclusively with service instances.

## Consequences
### Positive
- Clear separation of concerns (Transport → Service → Repository → Database).
- High testability: services and handlers can be unit tested using mock interfaces without requiring a live database connection.
- Elimination of duplicate SQL statements across API handlers.

### Negative / Trade-offs
- Slight increase in code volume (interface files, repository structs, service structs).
