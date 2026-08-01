# Modules, Capabilities, And Lifecycle

## Pure Definition

A module `Definition` declares a manifest, Action bindings, and migration
sources. Inspection validates declarations without opening migration files,
constructing handlers, starting resources, or touching persistent state.

The manifest provides a stable module ID and version, module type, dependencies,
capabilities provided, and capabilities required. The Host validates the full
graph before any startup side effect.

## Typed Capabilities

`module.Capability` is an open validated name. Modary defines standard database,
identity, authorization, and audit capabilities. Consumers may define
namespaced capabilities such as `example.clock`.

A capability contract package owns one package-level `module.Key[T]`. The
provider and every consumer import that exact key. Creating another key with
the same name does not recreate its identity and fails closed. This prevents
string-only service forgery.

## Startup Scope

A module's `Start` callback receives a sealed Scope limited to declared
capabilities. Providers publish services; consumers resolve only what their
manifest requires. The Scope is valid during startup and must not be retained or
used by a background goroutine after the callback returns.

Process resources register cleanup with `module.OnStop`. Cleanup invocation
starts LIFO within a module and in reverse dependency order across modules.
Callbacks must honor cancellation and stop using dependencies before returning.
A timed-out callback can overlap later cleanup, so this is invocation-start
order, not a completion-order guarantee.

## Handler Factory Resolver

Action handler factories receive a read-only Resolver after services are
installed. The HandlerFactory Resolver is valid only during the factory call.
Resolve and retain the typed service value, never the Resolver. Later Resolver
use returns `module.ErrInvalidResolver`.

Handlers may run concurrently. Retained services and handlers must be safe for
concurrent use and must honor request cancellation.

## Migrations

The Host applies declared migrations before constructing Action handlers. F0
supports forward-only SQLite migration sets with bounded files and history.
Lifecycle rollback covers process resources and service bindings; committed
schema changes are durable and are not reversed by a later startup failure.

## Shutdown

The assembled application owns one lifecycle gate. Shutdown marks it unavailable,
rejects new leases, cancels active leased contexts, drains cooperative work, and
then begins Host cleanup. Retained Runtime and identity facades fail closed after
revocation and cannot reach cleaned services.

See [ADR-001](../adr/ADR-001-explicit-composition-and-capability-lifecycle.md)
for the accepted decision and [add a module](../how-to/add-module.md) for the
implementation path.
