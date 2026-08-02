# Modary Delivery Constitution

- Version: 4.0
- Status: Confirmed
- Last updated: 2026-08-02
- Approval source: owner-approved lightweight component framework refoundation

## Product Boundary

Modary is a lightweight, componentized Go application framework for business
backends and administrative systems. It helps consumers build maintainable
modular monoliths from explicit, optional components without copying a complete
platform into every application.

The framework owns composition contracts, lifecycle, standard component
interfaces, starter tooling, and optional official profiles. Consumers own
domain language, business schemas, product routes, UI customization, policy,
deployment, and releases.

## Principles

- The empty Core has no database, queue, identity, authorization, audit,
  governed Action, MCP, or UI requirement. A capability exists only when the
  consumer selects and composes a component that provides it.
- Components are ordinary Go packages with explicit manifests, typed
  capabilities, deterministic dependencies, bounded lifecycle, and no global
  mutable service locator. Composition is static Go code, not runtime package
  discovery.
- Profiles are transparent composition presets, not hidden application modes.
  A generated application can inspect and replace every selected component.
- Unselected components contribute no routes, migrations, configuration,
  background processes, generated source, or runtime initialization.
- Framework tooling creates consumer-owned projects and fails closed rather
  than repeatedly patching handwritten business files. Generated contracts are
  deterministic and checked for drift.
- A normal HTTP route or business command does not require Governed Action
  semantics. Consumers opt high-impact mutations into the governed Runtime when
  they need Preview binding, impact authorization, idempotency, transaction,
  audit, or MCP exposure.
- The PostgreSQL and River implementation is an optional official durable
  component. Selecting it retains the accepted Alpha 3 transaction, migration,
  task, retry, and recovery contracts. A Core or API-only application starts
  without PostgreSQL.
- Business components selected into a database-backed Profile receive a narrow
  SQL store and transaction capability suitable for ordinary repositories.
  Governed Action transaction authority remains a separate capability and is
  not imposed on normal CRUD handlers.
- Business data belongs to the consumer. Official adapters do not create
  product records, roles, permissions, users, menus, or secrets unless a
  selected starter explicitly identifies development bootstrap data.
- The official Admin UI is an optional profile and separate frontend surface.
  Headless consumers require Go alone. The framework does not turn UI choices
  into Core dependencies.
- Public APIs expose narrow capabilities and lifecycle-safe facades, not raw
  registries, transaction control, River internals, or application-global
  variables.
- Published tags and migrations are immutable. `v0.1.0-alpha.3` remains the
  frozen accepted release; the v0.2 development contract may break pre-v1 APIs
  deliberately and documents every retained or replaced surface.

## Product Quality Gates

Framework acceptance starts with consumer outcomes:

1. A developer can create and run a headless API application without a
   database or Node.js.
2. A developer can create an Admin application with only the components in its
   selected profile, sign in, and use one consumer-owned business module.
3. Removing a component from composition removes its routes, migrations,
   configuration, processes, and UI contribution from a fresh assembly without
   editing framework internals. It never silently drops previously applied
   database objects or product data.
4. Generated projects remain ordinary independent Go modules and contain no
   copied framework implementation.
5. Governed applications retain the accepted authorization, transaction,
   audit, and durable-task guarantees through optional components.

Behavior changes use focused RED/GREEN/REFACTOR tests. Acceptance also requires
copied-out consumer tests, deterministic generation, dependency and component
absence checks, documentation checks, vet, race, build, source stability, and
current review evidence.

## Git And Review Policy

Preserve user work, avoid destructive commands, retain historical release
evidence as read-only records, and complete spec-compliance and engineering
quality reviews before acceptance.

## Change Process

Constitution changes require explicit owner approval. The active product and
acceptance contract is
`.ai-platform/specs/007-component-framework-refoundation/spec.md`.
