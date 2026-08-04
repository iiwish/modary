# Public Package Map

Consumers import public packages only. Paths below
`github.com/iiwish/modary/internal/` are framework-private.

## Core And Composition

| Package | Purpose |
|---|---|
| `module` | Manifests, graph, typed keys, migrations, startup, cleanup, Host lifecycle |
| `appkit` | Definition and opaque assembled Application facades |
| `appcmd` | Consumer command, server, and governed CLI orchestration |
| `httpkit` | Bounded routes plus pure, capability-aware HTTP/Admin contribution planning |
| `starter` | Reusable create-only API for API/Admin/Governed Profiles |
| `cmd/modary` | Global create-only command |
| `projecttool` | Optional verify/generate/check/build workflow for manifest-based consumers |

## Contracts

| Package | Purpose |
|---|---|
| `database` | Provider-neutral `Store`, governed `Access`, Row/Rows, SQL errors |
| `identity` | Actor, session value, password, bearer, and browser-session authentication contracts |
| `authz` | Authorization request, phase, impact, decision, fingerprint |
| `scope` | Validated execution scopes |
| `action` | Descriptors, schemas, plans, Runtime, results, public errors |
| `audit` | Governed events plus scope-bound, metadata-only operational reading |
| `task` | Transactional enqueue, immutable runners, and metadata-only inspection |

`module.CapabilityIdentity` and `module.CapabilitySessions` intentionally model
different installation requirements. Contributions using browser cookies or
session middleware require `CapabilitySessions`; principal lookup or token
authentication does not imply that capability. Task inspection uses the closed
provider-neutral `task.State` vocabulary rather than queue implementation
states.

## Standard Components

| Package | Selection |
|---|---|
| `components/postgres` | Ordinary PostgreSQL Store; no River or Action persistence |
| `components/governedpostgres` | Governed PostgreSQL persistence and River-backed tasks |
| `components/postgres/localidentity` | Explicit local principals, passwords, sessions, bearer tokens |
| `components/postgres/rbac` | Scope-aware roles, permissions, row limits, bindings |
| `components/postgres/sqlaudit` | SQL-backed governed audit |

## Transports

| Package | Purpose |
|---|---|
| `transport/httpapi` | Health, governed session Action API, MCP, immutable SPA handling with bounded fallback exclusions |
| `transport/sessionhttp` | Standalone Admin login/session/logout and auth/CSRF middleware |

## Selection Rules

- API normally imports Core and consumer route packages.
- Admin uses `postgresdb`, local Identity, RBAC, and `sessionhttp`; `--with
  tasks` selects governed PostgreSQL and `--with audit` selects SQL Audit.
- Governed uses `governedpostgres`, local Identity, RBAC, SQL Audit, Action
  transports, and `task`.
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
