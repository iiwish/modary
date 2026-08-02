# Choose A Profile

A Profile is a create-only source preset. It decides the initial files and
component graph; it is not a hidden runtime mode. After creation, the project
owns the generated source and `appkit.Definition` remains authoritative.

## Decision Table

| Question | Start with |
|---|---|
| Do you need only health and product HTTP routes? | `api` |
| Do you need login, RBAC, PostgreSQL CRUD, and a usable admin work surface? | `admin` |
| Must a mutation be previewed, audited, idempotent, and commit a durable task atomically? | `governed` |

Prefer the first Profile that satisfies the current product. Do not select
Governed merely because background work might be useful later. River is valuable
when task insertion must share the governed PostgreSQL transaction; it is not a
general prerequisite for Modary.

## API

Initial graph:

```text
appkit + module.Host + health + consumer ping route
```

No database, Identity, RBAC, session, frontend, Action Runtime, Audit, River,
task service, or MCP handler is present. Add consumer routes and Modules using
ordinary Go.

Use API for public services, internal services, webhooks, and applications that
already own infrastructure choices.

## Admin

Initial graph:

```text
postgresdb + localidentity + rbac + sessionhttp + records + React Admin
```

The business database uses PostgreSQL through `database.Store`. Mutations run
inside a synchronous callback transaction owned by the repository path. Admin
does not install River, Action plan/idempotency tables, SQL Audit, task workers,
or MCP.

Use Admin for operational tools and business back offices where ordinary CRUD
is appropriate. Replace local Identity for production-facing deployments.

## Governed

Initial graph:

```text
postgres + River + localidentity + rbac + sqlaudit
+ limits.set Action + CLI/HTTP/MCP + worker
```

The generated Action requires Preview, optimistic versioning, idempotency, and
backend authorization. Its business write, idempotency record, audit record,
and River insertion share one framework-owned PostgreSQL transaction. The
worker uses the provider-neutral `task` API.

Use Governed for financially, operationally, or autonomously significant
commands where showing impact before execution and proving durable intent
matters.

## Combining Components

Profiles are starting points. A product may add `task` and governed operations
to an Admin project later by editing the explicit composition. Keep ordinary
CRUD ordinary; route only high-impact operations through `action.Runtime`.
There is no requirement that every write use Actions.

The F0 Starter does not patch or merge existing projects. Additions after
creation are normal consumer-owned code changes with tests and migrations.
