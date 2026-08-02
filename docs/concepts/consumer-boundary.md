# Framework And Consumer Boundary

Modary owns reusable mechanics. The application owns product meaning. The
boundary applies even when the Starter generated the initial source.

## Framework Ownership

Core owns:

- Module manifest and graph validation;
- typed capability identity and least-authority resolution;
- deterministic startup, revocation, draining, and cleanup;
- opaque application assembly and safe callback boundaries.

Optional framework components own bounded mechanics such as:

- PostgreSQL connection and migration control;
- development Identity, RBAC, sessions, and CSRF;
- governed plan, idempotency, audit, and transaction ordering;
- River-backed task insertion and runner lifecycle;
- health, Action HTTP, MCP, and static asset handlers;
- create-only Profile rendering.

## Consumer Ownership

The application owns:

- domain entities, validation, statuses, errors, and workflow;
- feature Modules, repositories, migrations, routes, and Action handlers;
- selected framework components and their explicit configuration;
- users, roles, bindings, scopes, bootstrap policy, and identity integration;
- frontend modules, navigation, wording, branding, and product accessibility;
- secrets, TLS, rate limiting, observability, backup, deployment, and releases.

The Admin records and Governed limits slices are generated examples. They are
consumer-owned after creation and are replacement points, not framework domain
models.

## Mechanical Boundary

Consumers import public packages only. They do not receive a mutable Action
registry, raw Handler table, Host internals, migration controller, raw
`*sql.DB`, `*sql.Tx`, or commit/rollback authority.

Composition is an explicit Go value. Modary does not discover Modules through
directories, reflection, database rows, plugins, or frontend metadata. This
makes package and migration absence observable when a component is omitted.

## Extension Rule

Keep an extension in the consumer when it contains product vocabulary or is
needed by one application. Add a framework component only when the contract is
domain-neutral, useful to multiple consumers, narrow enough to remove, and
testable without a product repository.
