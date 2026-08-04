# Modary

Modary is a lightweight, component-oriented Go framework for business systems
and administrative backends. It provides a small modular-monolith Core,
explicit optional components, create-only project Profiles, and one disciplined
path for operations that need Preview, idempotency, audit, and durable work.

The framework is deliberately not an all-in-one admin product. A project owns
its domain modules, schema, routes, policy, branding, deployment, and release.
Selecting a Profile copies ordinary Go and optional React source into that
project; unselected concrete adapters contribute no migration, route,
navigation item, configuration requirement, goroutine, or runtime service.
Small public contract packages shared by Core remain implementation-neutral.

## Why Modary

Large admin templates are productive when their complete feature set matches a
product. They become expensive when an application needs only a small subset:
framework tables, menu models, generators, permissions, background jobs, and UI
conventions arrive as one coupled system.

Modary takes the opposite approach:

- **Start small.** Core needs only Go and has no database, task queue, identity,
  Action Runtime, MCP server, or frontend dependency.
- **Compose visibly.** One `appkit.Definition` lists the exact Modules used by an
  application. There is no package scanning or global service locator.
- **Own product code.** Feature handlers, migrations, repositories, routes, and
  frontend modules stay in the consumer project.
- **Choose mutation semantics.** Ordinary CRUD uses a bounded business
  `database.Store`. High-impact operations may opt into governed Actions.
- **Remove by omission.** API, Admin, and Governed generated source and
  dependency graphs prove that unselected concrete adapters and infrastructure
  libraries are absent.

## Profiles

| Profile | Selects | Does not select |
|---|---|---|
| `api` | Core, process probes, one example route | database, identity, UI, River, Actions, audit, MCP, OTel |
| `admin` | PostgreSQL business Store, local development Identity, RBAC, sessions, React Admin, records slice | River, governed Actions, SQL Audit, MCP |
| `governed` | PostgreSQL, River, Identity, RBAC, SQL Audit, governed Action, CLI/HTTP/MCP, worker | Admin UI and ordinary records slice |

Profiles are creation presets, not runtime modes. After creation the generated
files belong to the application and the Module list remains the source of truth.
The Starter never patches an existing project.

## Install v0.3 Alpha 1

`v0.3.0-alpha.1` is the current component-framework release.
`v0.2.0-alpha.1` remains the immutable React component-framework baseline.

Go 1.26.5 or newer is required. Create a database-free API project directly
from the released Starter:

```bash
go run github.com/iiwish/modary/cmd/modary@v0.3.0-alpha.1 \
  new sample-api --profile api --module example.com/acme/sample-api
cd sample-api
go mod tidy
go test ./...
go run ./cmd/sample-api
```

The API starts on `127.0.0.1:8080` and exposes local `/livez`, bounded
dependency `/readyz`, and `/api/ping`.
Continue with the [Profile quickstart](docs/getting-started/quickstart.md).

## Architecture

```text
consumer command / HTTP server / worker
                 |
                 v
        appkit.Definition (explicit Modules)
                 |
                 v
      module.Host (graph + lifecycle + capabilities)
          |              |                 |
      API feature   ordinary Admin    governed operation
      no database   database.Store    action.Runtime
                                      PostgreSQL + River
```

Core owns Module validation, typed capabilities, lifecycle, and opaque
application assembly. Standard components add persistence, identity,
authorization, sessions, tasks, audit, and transports. Consumer Modules own
business behavior.

Governed Actions add a stricter transaction path when a product needs it:

```text
authorize intent -> Preview -> bind plan -> authorize impact
-> transaction -> reauthorize -> idempotency -> mutation + task + audit
```

This path is optional. Ordinary Admin CRUD does not need Preview or River.

## Admin UI

The optional Admin Profile contains React 19, TypeScript, Vite, React Router,
Lucide React, small context-based state providers, and an explicit frontend
module registry. A prebuilt production bundle is
embedded in the generated Go binary, so deployment does not require Node.js.
Node.js and pnpm are required only when changing the generated frontend source.

The F0 UI includes local-password or OIDC redirect login, session restoration,
logout, permission-aware
navigation and commands, responsive scoped CRUD, and optional read-only task
and audit operations. `--with tasks`, `--with audit`, `--with oidc`, and
`--with otel` select those components at generation time. OIDC replaces the
local password surface; OTel adds no UI. Omitted components contribute no Go
dependency, route, configuration, source module, or production bundle code. It
is a reference work surface, not a framework-owned low-code schema or dynamic
menu engine.

## Public Layers

| Layer | Main packages |
|---|---|
| Core | `module`, `appkit`, `appcmd`, `httpkit`, `processkit` |
| Contracts | `database`, `identity`, `authz`, `scope`, `task`, `action`, `audit`, `observe` |
| Standard components | `components/postgres`, `components/governedpostgres`, `components/oidc`, `components/otel`, `components/postgres/identitystore`, `components/postgres/rbac`, `components/postgres/sqlaudit` |
| Transports | `transport/httpapi`, `transport/sessionhttp` |
| Tooling | `starter`, `cmd/modary`, `projecttool` |

`components/postgres` is the ordinary PostgreSQL component. It has no River or
governed persistence dependency. `components/governedpostgres` is the Governed component
that installs Action persistence and River-backed tasks. They are separate on
purpose.

## Documentation

Start at the [documentation index](docs/index.md):

- [Choose a Profile](docs/getting-started/choose-profile.md)
- [Quickstart](docs/getting-started/quickstart.md)
- [Write a consumer component](docs/how-to/add-module.md)
- [Admin Profile tutorial](docs/getting-started/admin-profile.md)
- [Governed Profile tutorial](docs/getting-started/governed-profile.md)
- [Persistence and tasks](docs/concepts/persistence-and-tasks.md)
- [Rulary adoption plan](docs/guides/rulary-bootstrap.md)
- [简体中文教程](docs/zh-CN/index.md)
- [v0.2 Alpha 1 to v0.3 Alpha 1 migration](docs/releases/upgrade-v0.2-to-v0.3.md)

## Verification

Framework contributors run:

```bash
make bootstrap
make acceptance
make race
```

The F0 evidence additionally covers copied-out API/Admin/Governed projects,
real PostgreSQL, disposable OIDC and OTLP endpoints, non-root OCI images,
frontend asset reproducibility, browser desktop/mobile checks, active-request
drain, the external Counter conformance consumer, source stability, and
cross-builds.

## Stability, License, And Security

Modary is pre-v1. Pin exact versions. PostgreSQL is the only official durable
database at F0; MySQL and embedded databases are not implemented. Local
Identity is for development and controlled internal deployments, not a complete
public-internet IAM system.

Modary is licensed under the [Apache License 2.0](LICENSE). Report security
issues through the private process in [SECURITY.md](SECURITY.md), not a public
issue.
