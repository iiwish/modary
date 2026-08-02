# Public Package Map

Consumers import public packages only. Any `github.com/iiwish/modary/internal/`
path is framework-private and unsupported.

| Package | Consumer responsibility |
| --- | --- |
| `action` | Declare typed Action contracts and implement concurrent, cancellation-aware Handlers. |
| `module` | Declare pure Definitions, manifests, typed capabilities, migrations, startup, and cleanup. |
| `appkit` | Compose Definitions and use the opaque assembled application and read-only facades. |
| `appcmd` | Build consumer-owned application commands and HTTP server orchestration. |
| `projecttool` | Pin verify, generate, check, and build workflows in the consumer module. |
| `transport/httpapi` | Explicitly mount health, Action HTTP API, MCP, and consumer static assets. |
| `scope` | Create validated opaque execution scopes. |
| `identity` | Use normalized actor and authentication contracts. |
| `authz` | Use permissions, bounded impact, decisions, and policy fingerprints. |
| `audit` | Return bounded business references and implement audit contracts when extending the framework. |
| `database` | Use the narrow governed read/write surface inside consumer modules. |
| `task` | Enqueue durable work transactionally and construct immutable worker runners. |
| `adapters/postgres` | Install the PostgreSQL control store and River-backed task runtime. |
| `adapters/localidentity` | Explicitly provision principals independently from optional password and bearer credentials, plus local sessions. |
| `adapters/rbac` | Explicitly provision roles, permissions, row limits, and scoped bindings. |
| `adapters/sqlaudit` | Install SQL-backed required audit behavior. |

## Selection Rules

An application normally imports `appkit`, `appcmd`, `projecttool`, `module`, and
the official adapters from its composition root. Feature modules normally
import `action`, `module`, `database`, `scope`, and narrow domain-independent
contracts required by their behavior.

Consumers should not implement an alternative privileged database, transaction,
idempotency, task queue, or plan store through hidden framework contracts. The
official F0 durable boundary is PostgreSQL. A new privileged adapter is a framework
contribution with conformance and security review, not an ordinary consumer
plugin.

## API Documentation

Go package comments and exported symbol documentation are part of the source
contract. Use `go doc` against the pinned consumer version:

```bash
go doc github.com/iiwish/modary/action
go doc github.com/iiwish/modary/module
go doc github.com/iiwish/modary/appkit
go doc github.com/iiwish/modary/task
```

The [F0 contract](../framework-f0.md) defines cross-package invariants that API
documentation cannot express on one symbol.
