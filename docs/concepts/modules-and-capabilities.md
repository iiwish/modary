# Modules, Capabilities, And Lifecycle

## Definition And Registration

A `module.Registration` separates a pure Definition from its optional Start
callback. The Definition contains a versioned manifest, migration sources, and
Action bindings. Constructing or inspecting it performs no database, network,
random, hashing, migration-read, or handler-construction work.

The Host validates the complete graph and static Action catalog before any
Module starts. Registration order is not dependency order.

## Typed Capabilities

`module.Capability` is an open validated name. Standard capabilities include
database, task enqueue and inspection, Identity, browser sessions,
authorization, and audit. Consumers may define namespaced capabilities such as
`example.clock`.

Identity resolution and authentication transports are separate dependencies.
`module.CapabilityIdentity` owns principal lookup and password/token identity;
`module.CapabilitySessions` owns browser-session authentication. An HTTP or
Admin contribution that authenticates a session declares
`module.CapabilitySessions` directly. Requiring broad Identity is not a
substitute and cannot make the contribution pass preflight.

A contract package owns one package-level `module.Key[T]`. Provider and
consumers import that exact key. Recreating the same string and Go type produces
a different identity and fails closed.

Two database keys intentionally expose different authority:

- `module.Database()` resolves `database.Store` for ordinary business work;
- `module.ActionDatabase()` resolves `database.Access` inside governed Action
  handlers.

Only `Store` can open a synchronous callback transaction. Neither surface
exposes raw transaction control.

## Startup

A Start callback receives a sealed Scope. It may resolve only declared
requirements, provide only declared capabilities, and register cleanup only
during the callback. It must not retain the Scope.

Migration sets are applied after the selected database component starts and
before that Module's Action handlers are constructed. A database-free Module
declares no PostgreSQL migration.

Action handler factories receive a separate read-only Resolver. Resolve and
retain service values during factory execution; retaining the Resolver causes
later use to fail with `module.ErrInvalidResolver`.

## Application Assembly

`appkit.Start` returns an opaque Application with only the facades installed by
the selected graph. Optional accessors fail explicitly when absent. An API
Profile therefore has nil Runtime/Tasks and unavailable Database/Authorizer;
this is a supported state, not a partially initialized application.

## Shutdown

The assembled application owns one lifecycle gate. Shutdown rejects new calls,
cancels active leased contexts, waits for cooperative work, and invokes cleanup
exactly once. Cleanup starts LIFO within a Module and in reverse dependency
order across Modules.

Each callback has a bounded context. A callback that ignores cancellation may
overlap later cleanup; the guarantee is invocation-start order, not completion
order. Retained Runtime, database, identity, session, and authorization facades
fail closed after revocation.

See [ADR-001](../adr/ADR-001-explicit-composition-and-capability-lifecycle.md).
