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
| `processkit` | Liveness, readiness, admission, drain, server, migration-command, and build-identity primitives |
| `starter` | Reusable create-only API for API/Admin/Governed Profiles |
| `cmd/modary` | Global create-only command |
| `projecttool` | Optional verify/generate/check/build workflow for manifest-based consumers |

## Contracts

| Package | Purpose |
|---|---|
| `database` | Provider-neutral `Store`, governed `Access`, Row/Rows, SQL errors |
| `identity` | Scope-independent Actor, Authentication, session manager, password, bearer, and resolver contracts |
| `authz` | Authorization request, phase, impact, decision, fingerprint |
| `scope` | Validated execution scopes |
| `action` | Descriptors, schemas, plans, Runtime, results, public errors |
| `audit` | Governed events plus scope-bound, metadata-only operational reading |
| `task` | Transactional enqueue, immutable runners, and metadata-only inspection |
| `observe` | Provider-neutral closed operation and outcome vocabulary for optional instrumentation |

`module.CapabilityIdentity`, `module.CapabilityPasswords`, and
`module.CapabilitySessions` intentionally model different installation
requirements. Password login requires Passwords plus Sessions. Contributions
using browser cookies or session middleware require Sessions only; principal
lookup or bearer authentication requires Identity. Task inspection uses the closed
provider-neutral `task.State` vocabulary rather than queue implementation
states.

## Standard Components

| Package | Selection |
|---|---|
| `components/postgres` | Ordinary PostgreSQL Store; no River or Action persistence |
| `components/governedpostgres` | Governed PostgreSQL persistence and River-backed tasks |
| `components/postgres/identitystore` | Explicit local principals, passwords, sessions, bearer tokens |
| `components/postgres/rbac` | Scope-aware roles, permissions, row limits, bindings |
| `components/postgres/sqlaudit` | SQL-backed governed audit |
| `components/oidc` | Optional OIDC relying party and revocable browser authentication |
| `components/oidc/oidchttp` | Explicit Authorization Code, PKCE, callback, and logout HTTP contribution |
| `components/otel` | Optional OTLP/HTTP traces and metrics without process-global providers |

## Transports

| Package | Purpose |
|---|---|
| `transport/httpapi` | Governed session Action API, MCP, and immutable SPA handling with bounded fallback exclusions |
| `transport/sessionhttp` | Standalone Admin login/session/logout and auth/CSRF middleware |

## Selection Rules

- API normally imports Core and consumer route packages.
- Admin uses `postgresdb`, local Identity, RBAC, and `sessionhttp`; `--with
  tasks` selects governed PostgreSQL, `--with audit` selects SQL Audit,
  `--with oidc` replaces password login, and `--with otel` selects OTLP
  telemetry.
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
go doc github.com/iiwish/modary/processkit
go doc github.com/iiwish/modary/components/oidc
go doc github.com/iiwish/modary/components/otel
```
