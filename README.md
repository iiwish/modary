# Modary

Modary is a Go-first framework for governed modular applications. Consumers
define their own modules and typed business Actions; Modary runs every Action
through the same authorization, Preview/Execute binding, idempotency,
transaction, and audit semantics across CLI, HTTP, MCP, and custom surfaces.

Use Modary when an application needs explicit composition, inspectable module
boundaries, durable PostgreSQL transactions, recoverable background work, and
consistent policy enforcement.
Modary does not own product schemas, domain language, UI, deployment, identity
provisioning, or release artifacts.

## Stability

`v0.1.0-alpha.3` is the PostgreSQL and durable-task pre-v1 Alpha release.
Consumers must pin it exactly. Review the
[versioning policy](docs/releases/versioning.md), and read the
[known limitations](docs/f0-known-limitations.md) before production use. The F0
durable profile is one PostgreSQL control database with an application schema
and a separate River queue schema. Application and worker processes may scale
independently. It is not a hostile extension sandbox, distributed transaction
system, or public-internet IAM solution.

Go 1.26 or newer is required. Node.js is not required by framework or example
workflows.

## Quickstart

Run the public Counter example from the current source:

```bash
git clone https://github.com/iiwish/modary.git
cd modary
make bootstrap
```

Continue with the complete [quickstart](docs/getting-started/quickstart.md) to
start PostgreSQL 17, verify the independent consumer, and preview the governed
Counter Action. Then use
[Create Your First Independent Application](docs/getting-started/first-application.md)
to copy the example outside the framework checkout.

## Add Modary To An Application

```bash
go get github.com/iiwish/modary@v0.1.0-alpha.3
go mod tidy
```

An application is an ordinary independent Go module. It owns one pure
`appkit.Definition`, a `modary.yaml` project manifest, an application command,
a pinned project tool, consumer modules, migrations, generated contracts, and
tests. Module discovery is explicit Go composition, not source scanning or
runtime registration.

## Runtime Model

```text
consumer transport / appcmd
            |
            v
      action.Runtime
  intent authorization
  Preview + plan binding
  impact authorization
  idempotency reservation
  transaction + reauthorization
  handler + required audit
```

Every state-changing business path converges on `action.Runtime`. Consumers do
not receive raw transaction, lifecycle, registry, or Handler authority from the
assembled application.

## Public Packages

| Package | Responsibility |
|---|---|
| `action` | Typed Action descriptors, schemas, plans, errors, and Runtime |
| `module` | Definitions, dependency graph, typed capabilities, lifecycle |
| `appkit` | Pure application composition and opaque assembled facades |
| `appcmd` | Consumer application commands and server orchestration |
| `projecttool` | Verify, deterministic generate/check, and build workflows |
| `transport/httpapi` | Explicit health, Action HTTP, MCP, and static handlers |
| `task` | Framework-neutral transactional enqueue and worker contracts |
| `adapters/postgres` | PostgreSQL control store and River-backed task runtime |
| `adapters/localidentity`, `adapters/rbac`, `adapters/sqlaudit` | Standard Identity, authorization, and audit modules |
| `scope`, `identity`, `authz`, `audit`, `database` | Narrow framework contracts |

Consumer code imports only public packages. Everything below `internal/` is a
framework implementation detail.

## Documentation

Start at the [documentation index](docs/index.md):

- [简体中文上手教程](docs/zh-CN/index.md)
- [Installation and version pinning](docs/getting-started/installation.md)
- [Framework and consumer ownership](docs/concepts/consumer-boundary.md)
- [Add a consumer module](docs/how-to/add-module.md)
- [Expose a governed Action](docs/how-to/expose-action.md)
- [Troubleshooting](docs/how-to/troubleshooting.md)
- [Deployment and production checklist](docs/operations/deployment.md)
- [Complete F0 contract](docs/framework-f0.md)

## Framework Verification

Contributors run:

```bash
make bootstrap
make acceptance
make ci
```

These gates cover framework and copied-out consumer tests, vet, race,
repetition, fuzz smoke, generated drift, neutrality, source stability, native
builds, and Linux, Darwin, and Windows cross-build contracts.

## License And Security

Modary is licensed under the [Apache License 2.0](LICENSE). Third-party
attributions are recorded in [NOTICE](NOTICE). Report vulnerabilities through
the private channel in [SECURITY.md](SECURITY.md), not a public issue.
