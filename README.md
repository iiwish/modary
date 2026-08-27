# Modary

**Build Go backends that start small and stay explicit.**

A component-oriented framework for business systems and administrative
backends. Start with a database-free core, then add only the infrastructure
your product needs.

[![CI](https://github.com/iiwish/modary/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/iiwish/modary/actions/workflows/ci.yml)
[![Release v0.3.0-alpha.2](https://img.shields.io/badge/release-v0.3.0--alpha.2-2f6f4e)](https://github.com/iiwish/modary/tree/v0.3.0-alpha.2)
[![Go reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/iiwish/modary)
[![Apache 2.0 license](https://img.shields.io/badge/license-Apache--2.0-5a6472)](LICENSE)

[Why Modary](#why-modary) · [Quick start](#quick-start) ·
[Profiles](#choose-a-profile) · [Architecture](#architecture) ·
[Documentation](docs/index.md) · [Contributing](CONTRIBUTING.md) ·
[简体中文](README.zh-CN.md)

---

Modary gives Go teams a small modular-monolith core and a deliberate path from
ordinary CRUD to high-impact operations that need Preview, authorization,
idempotency, audit, and durable work. There is no package scanning, global
service locator, or framework-owned domain model. The application lists its
Modules explicitly and owns the resulting code.

> [!IMPORTANT]
> Modary is pre-v1 Alpha software. `v0.3.0-alpha.2` is the current component-framework release.
> It requires Go 1.26.7 or newer. Pin exact versions and review the
> [changelog](CHANGELOG.md) before upgrading.

## Why Modary

Large admin starters work well when their full feature set matches the product.
Otherwise, their database models, permission systems, jobs, menus, and UI
conventions quickly become accidental architecture. Modary makes those choices
visible and removable.

| Principle | What it means in practice |
|---|---|
| **Small by default** | Core has no database, task queue, identity, Action Runtime, MCP server, or frontend dependency. |
| **Explicit composition** | One `appkit.Definition` declares the exact Modules and capabilities in the process. |
| **Consumer ownership** | Domain code, schema, routes, policy, branding, deployment, and release stay in the application repository. |
| **Proportional rigor** | Use ordinary transactions for CRUD; opt into governed Actions only where Preview, audit, idempotency, or durable tasks matter. |
| **Absence you can prove** | Unselected adapters contribute no migration, route, config, goroutine, service, source module, or production bundle code. |

Modary is designed for teams building internal tools, operational consoles,
business APIs, and governed workflows that want framework support without
giving up a normal Go codebase.

## Quick Start

Create the smallest, database-free API Profile from the released Starter:

```bash
go run github.com/iiwish/modary/cmd/modary@v0.3.0-alpha.2 \
  new sample-api \
  --profile api \
  --module example.com/acme/sample-api

cd sample-api
go mod tidy
go test ./...
go run ./cmd/sample-api
```

In another terminal:

```bash
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/api/ping
# {"message":"pong"}
```

The generated project contains ordinary Go source. Its composition root is
small enough to inspect at a glance:

```go
func Definition() appkit.Definition {
	return appkit.Definition{
		Metadata: appkit.Metadata{
			ID:      "sample-api",
			Name:    "sample-api",
			Version: "0.1.0",
		},
		Modules: []module.Registration{
			ping.Registration(),
		},
	}
}
```

Continue with the [five-minute quickstart](docs/getting-started/quickstart.md)
or [create your first consumer Module](docs/how-to/add-module.md).

## Choose A Profile

Profiles are create-only starting points, not runtime modes. The Starter copies
source into a new project and never patches an existing one. From that point
on, the files and the Module list belong to the application.

| Profile | Best for | Selected baseline | Deliberately absent |
|---|---|---|---|
| [`api`](docs/getting-started/first-application.md) | Services and lightweight APIs | Core, process probes, example route | Database, identity, UI, tasks, audit, Actions, MCP, OTel |
| [`admin`](docs/getting-started/admin-profile.md) | Internal tools and back offices | PostgreSQL Store, local identity, RBAC, sessions, embedded React Admin | Governed Actions and MCP; tasks, audit, OIDC, and OTel are opt-in |
| [`governed`](docs/getting-started/governed-profile.md) | High-impact commands and durable workflows | PostgreSQL, River, identity, RBAC, SQL Audit, Actions over CLI/HTTP/MCP, worker | Admin UI and ordinary records slice |

Admin projects can select `tasks`, `audit`, `oidc`, and `otel` at creation time.
See [Choose a Profile](docs/getting-started/choose-profile.md) for the exact
decision path and component boundaries.

## Architecture

```text
consumer command / HTTP server / worker
                 |
                 v
        appkit.Definition
     explicit Module registrations
                 |
                 v
      module.Host + typed capabilities
        /              |              \
       /               |               \
database-free API   Admin CRUD    governed Action
                     Store          Runtime
                                      |
                           PostgreSQL + River
```

Core owns Module validation, dependency ordering, typed capabilities,
lifecycle, and opaque application assembly. Optional components add
persistence, identity, authorization, sessions, tasks, audit, observability,
and transports. Consumer Modules own business behavior.

### Governed Actions

For operations whose impact should be reviewed before it is committed, the
optional Action Runtime uses one disciplined execution path:

```text
authorize intent -> Preview -> bind plan -> authorize impact
-> transaction -> reauthorize -> idempotency
-> mutation + durable task + audit
```

This path is optional. Ordinary Admin CRUD does not need Preview or River. It
uses the bounded `database.Store`; only selected operations use governed
Actions.

### Admin UI

The optional Admin Profile generates a consumer-owned React 19 and TypeScript
work surface with session restoration, CSRF protection, permission-aware
navigation, responsive scoped CRUD, and accessible dialogs. Its production
bundle is embedded in the Go binary, so deployed applications do not require
Node.js. It is a reference work surface, not a low-code schema or dynamic menu
engine.

## Package Map

| Layer | Main packages |
|---|---|
| Core and composition | `module`, `appkit`, `appcmd`, `httpkit`, `processkit` |
| Narrow contracts | `database`, `identity`, `authz`, `scope`, `task`, `action`, `audit`, `observe` |
| Standard components | `components/postgres`, `components/governedpostgres`, `components/oidc`, `components/otel` |
| Transports | `transport/httpapi`, `transport/sessionhttp` |
| Project creation | `starter`, `cmd/modary` |
| Optional project tooling | `projecttool` |

The ordinary PostgreSQL component does not depend on River or governed Action
persistence. The Governed PostgreSQL component adds them intentionally. See the
[public package map](docs/reference/packages.md) for import guidance.

## Documentation

| Goal | Start here |
|---|---|
| Understand the model | [Components and Profiles](docs/concepts/components-and-profiles.md), [Modules and capabilities](docs/concepts/modules-and-capabilities.md) |
| Build an application | [Quickstart](docs/getting-started/quickstart.md), [Add a Module](docs/how-to/add-module.md), [Expose an Action](docs/how-to/expose-action.md) |
| Run in production | [Deployment](docs/operations/deployment.md), [Observability](docs/operations/observability.md), [Security boundaries](docs/operations/security.md) |
| Check exact support | [Support matrix](docs/reference/support-matrix.md), [Known limitations](docs/f0-known-limitations.md), [F0 contract](docs/framework-f0.md) |
| Read in Chinese | [简体中文文档](docs/zh-CN/index.md) |

The complete documentation map, including tutorials, operations guides, ADRs,
and release notes, lives at [`docs/index.md`](docs/index.md).

## Production Boundaries

- PostgreSQL is the only official durable database at F0. MySQL and embedded
  database adapters are not provided.
- Local Identity is intended for development and controlled internal
  deployments, not as a complete public-internet IAM system.
- Durable jobs are at least once. Handlers must be idempotent and
  cancellation-aware.
- Modary is not an operating-system sandbox, distributed transaction
  coordinator, database operator, or deployment security boundary.

Read the [support matrix](docs/reference/support-matrix.md),
[security policy](SECURITY.md), and
[known limitations](docs/f0-known-limitations.md) before production use.

## Contributing

Contributions are welcome when they preserve the framework/consumer boundary
and keep optional infrastructure genuinely optional. Start with
[`CONTRIBUTING.md`](CONTRIBUTING.md), then run the normal acceptance suite:

```bash
make bootstrap
make acceptance
make race
```

Documentation-only changes can be checked with `make docs-check`. Security
issues must use the [private reporting process](SECURITY.md), not a public
issue.

## License

Modary is available under the [Apache License 2.0](LICENSE).
