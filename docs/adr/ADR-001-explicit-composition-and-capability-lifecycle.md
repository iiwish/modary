# ADR-001: Explicit Composition And Capability-Scoped Lifecycle

- Status: Accepted
- Date: 2026-07-31
- Scope: Module composition, services, and lifecycle

## Context

A modular framework needs definitions that tooling can inspect without starting
resources, and runtime installation that cannot reach undeclared services or
forge ownership of governed Actions. Startup and shutdown must remain
deterministic when callbacks fail, panic, block, or race with cancellation.

## Decision

Consumer Go code is the only Module composition source. Each
`module.Registration` contains a pure Definition and separate Start callback.
The Definition declares a versioned manifest, Action descriptors and factories,
and migration sources; inspection does not invoke factories or open sources.

The Host validates the complete graph and static Action catalog before Start.
Capabilities are open validated `module.Capability` values. Framework services
use standard database, identity, authorization, and audit constants; consumers
may define namespaced capabilities for their own typed services. Service keys
carry stable names, capability ownership, and unforgeable identity. Providers
and consumers share one package-level typed key; recreating the same name and
type does not recreate that identity.
Each Start callback receives a sealed Module scope that can resolve only
declared services, provide only declared capabilities, and register cleanup only
during that callback. The Host applies declared migrations through private
database control. Action handler factories receive a separate read-only
Resolver after startup mutation is sealed. The Host assigns Action ownership and
owns the internal mutable registry and Runtime engine. Public assembly returns
only the governed `action.Runtime` interface and public identity facades.
The HandlerFactory Resolver is valid only during the factory call. A factory
resolves and retains service values rather than retaining the Resolver; expired
use fails with `module.ErrInvalidResolver`.

Hosts are constructor-only values with a self-bound initialization marker.
Nil, zero, forged, or copied values are unavailable and fail closed. Registration
is defensively copied. After the single terminal Start attempt, the Host releases
its Start callbacks, handler factories, and migration filesystem references
without mutating the caller-owned Registration. Consumers remain responsible for
the lifetime of their original Definition and captured credentials.

Startup proceeds in dependency order. Failure revokes bindings and attempts all
cleanup for the current and previously started Modules. Cleanup guarantees
invocation-start order: LIFO within a Module and reverse dependency order across
Modules. Shutdown synchronously revokes new execution, drains in-flight calls,
and is exactly-once. Caller
deadlines bound waiting. A timed-out cleanup callback may overlap later
callbacks and provider cleanup; trusted callbacks honor cancellation and stop
using dependencies before returning.

Runtime, Handler, authorization, audit, identity, database, Clock, and
`AuditFailure` contracts permit concurrent calls and require cooperative context
handling. Each `AuditFailure` receives an independent deadline context and
cannot replace the primary Runtime result.

## Consequences

- Project inspection and generation are side-effect-free and deterministic.
- Module dependencies are executable least-authority contracts.
- Consumers cannot bypass governance through the Host, a mutable registry, raw
  Handler bindings, privileged database control, or service-container access.
- A running Application does not retain Module startup closures or migration
  filesystems; consumer-owned composition values remain consumer-owned.
- Static composition requires a rebuild to change the Module set.
- Dynamic plugin loading and runtime discovery remain outside the framework.

## Rejected Alternatives

- Filesystem or reflection-based package discovery obscures the composition
  source and cannot safely load arbitrary project code.
- A global mutable service locator allows undeclared dependencies and makes
  parallel application instances interfere.
- Letting Modules register raw handlers directly permits forged ownership and
  makes partial-start revocation incomplete.
