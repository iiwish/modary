# Public Package Map

Consumers import public packages only. Paths below
`github.com/iiwish/modary/internal/` are framework-private.

## Core And Composition

| Package | Purpose |
|---|---|
| `module` | Manifests, graph, typed keys, migrations, startup, cleanup, Host lifecycle |
| `appkit` | Definition and opaque assembled Application facades |
| `appcmd` | Consumer command, server, and governed CLI orchestration |
| `httpkit` | Bounded explicit standard-library route composition |
| `starter` | Reusable create-only API for API/Admin/Governed Profiles |
| `cmd/modary` | Global create-only command |
| `projecttool` | Optional verify/generate/check/build workflow for manifest-based consumers |

## Contracts

| Package | Purpose |
|---|---|
| `database` | Provider-neutral `Store`, governed `Access`, Row/Rows, SQL errors |
| `identity` | Actor, session, password, and bearer authentication contracts |
| `authz` | Authorization request, phase, impact, decision, fingerprint |
| `scope` | Validated execution scopes |
| `action` | Descriptors, schemas, plans, Runtime, results, public errors |
| `audit` | Governed audit events and bounded references |
| `task` | Transactional enqueue and immutable runner contracts |

## Standard Components

| Package | Selection |
|---|---|
| `adapters/postgresdb` | Ordinary PostgreSQL Store; no River or Action persistence |
| `adapters/postgres` | Governed PostgreSQL persistence and River-backed tasks |
| `adapters/localidentity` | Explicit local principals, passwords, sessions, bearer tokens |
| `adapters/rbac` | Scope-aware roles, permissions, row limits, bindings |
| `adapters/sqlaudit` | SQL-backed governed audit |

## Transports

| Package | Purpose |
|---|---|
| `transport/httpapi` | Health, governed session Action API, MCP, immutable SPA handler |
| `transport/sessionhttp` | Standalone Admin login/session/logout and auth/CSRF middleware |

## Selection Rules

- API normally imports Core and consumer route packages.
- Admin uses `postgresdb`, local Identity, RBAC, and `sessionhttp`.
- Governed uses `postgres`, local Identity, RBAC, SQL Audit, Action transports,
  and `task`.
- Consumer feature packages import only the narrow contracts they use.
- Official Adapters do not import sibling Adapters; the composition root joins
  them.

Implementing a public interface does not grant installation into
framework-private Action persistence. New privileged transaction or task
adapters require framework conformance and security review.

Use `go doc` against the exact pinned version:

```bash
go doc github.com/iiwish/modary/module
go doc github.com/iiwish/modary/appkit
go doc github.com/iiwish/modary/database
go doc github.com/iiwish/modary/task
```
