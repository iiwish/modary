# Lightweight Component Framework Refoundation

- Version: 1.0
- Status: Confirmed
- Approval source: owner approval on 2026-08-02
- Last updated: 2026-08-02
- Governing constitution: `../../memory/constitution.md`

## Objective

Deliver Modary v0.2 F0 as a lightweight, componentized Go framework for
business backends and administrative systems. A consumer starts from a small
independent application, selects only required components, and can grow into an
Admin or Governed application without adopting an all-in-one template.

The immutable `v0.1.0-alpha.3` tag remains unchanged. The v0.2 contract may
break pre-v1 source compatibility where required to restore progressive
disclosure and component independence.

## Product Requirements

- PR-001: the public description names business backends and administrative
  systems as the target, modular monolith as the architecture, and lightweight
  component selection as the primary value.
- PR-002: the framework distinguishes Core, Standard Components, Starter
  Tooling, optional Admin UI, and optional Governed Operations.
- PR-003: Gin-Vue-Admin research records both its fast-start strengths and
  recurring trimming, generation-ownership, initialization, and integration
  friction without treating historical issues as current unresolved defects.

## Core And Component Requirements

- FR-001: a Core application composes Module registrations, starts and stops
  deterministically, and exposes health through standard Go HTTP without
  requiring PostgreSQL, River, identity, RBAC, audit, Action, MCP, UI, or
  Node.js.
- FR-002: every optional component declares its required and provided typed
  capabilities. Unselected components add no routes, migrations, configuration
  requirements, goroutines, generated source, or UI contributions.
- FR-003: components receive dependencies through bounded typed resolution and
  do not depend on application-global database, logger, router, configuration,
  identity, or cache variables.
- FR-004: consumer feature components own their handlers, business types,
  migrations, tests, and optional Admin contribution. Framework source contains
  no consumer domain vocabulary.
- FR-005: PostgreSQL, River tasks, Identity, RBAC, SQL Audit, and Governed
  Actions remain optional official components. When selected, their accepted
  transaction, migration, authorization, and recovery guarantees remain covered
  by conformance tests.
- FR-006: ordinary HTTP reads and mutations can be implemented without Action
  Preview semantics. High-impact mutations opt into Governed Actions explicitly.
- FR-021: the PostgreSQL application component exposes a narrow provider-neutral
  SQL query and transaction capability for ordinary consumer repositories.
  Governed transaction authority remains a separate optional capability.
- FR-022: removing a component from a fresh assembly removes migration
  registration but never auto-drops previously applied database objects or
  consumer data.

## Starter And Profile Requirements

- FR-007: a first-party `modary new` command creates a new independent Go module
  only in an empty destination and refuses ambiguous or destructive writes.
- FR-008: the generator supports `api`, `admin`, and `governed` Profiles and
  emits explicit consumer composition. Profiles are presets visible in source,
  not runtime switches.
- FR-009: the `api` Profile starts with one command and no database. Its source
  imports no PostgreSQL, River, Identity, RBAC, Audit, Action, or MCP package.
- FR-010: the `admin` Profile includes PostgreSQL-backed development identity,
  RBAC, sessions, the optional Admin UI, and one consumer-owned business module.
- FR-011: the `governed` Profile composes the accepted PostgreSQL, River, SQL
  Audit, Action HTTP/CLI/MCP, and transactional-task capabilities explicitly.
- FR-012: generated projects contain no copied Modary implementation. Re-running
  creation never splices or merges source into an existing handwritten project.

## Admin UI Requirements

- FR-013: the optional Admin UI uses Vue 3, TypeScript, Vite, and pnpm for source
  development and ships a consumer-runnable production bundle.
- FR-014: F0 includes sign-in, sign-out, session expiry, current actor,
  responsive navigation, permission-aware commands, reusable list/form/detail
  primitives, and consistent loading, empty, error, and forbidden states.
- FR-015: feature modules use an explicit frontend module registry for routes,
  navigation, and permissions. The generator does not patch that registry after
  project creation.
- FR-016: task and audit views appear only when their backend components and
  corresponding UI contributions are selected.
- FR-017: the Admin UI is accessible by keyboard, does not expose development
  generators in production, and passes desktop and mobile visual QA without
  overlap or clipped controls.

## Tooling And Ownership Requirements

- FR-018: project verification reports the selected component graph, missing or
  duplicate providers, component configuration requirements, and generated
  drift without installing runtime components.
- FR-019: complete generated artifacts are confined to dedicated paths and are
  deterministic. Handwritten Go, Vue, TypeScript, configuration, and migration
  files are never regeneration targets.
- FR-020: component removal is proven by absence tests covering routes,
  migrations, startup requirements, long-running processes, UI navigation, and
  imports in copied-out consumers. Migration absence is tested on a fresh
  database; uninstall data deletion is not implied.

## Non-Functional Requirements

- NFR-001: the API Profile remains usable with Go alone and starts without
  external services.
- NFR-002: application startup and shutdown are deterministic, context-aware,
  and free of leaked component goroutines under cooperative dependencies.
- NFR-003: public contracts use standard Go types and narrow interfaces;
  provider implementation types do not leak through consumer-facing APIs.
- NFR-004: generated applications are valid with `GOWORK=off`, build outside
  the Modary checkout, and pin an explicit Modary version during release
  verification.
- NFR-005: source and generated outputs are formatted, documented, race tested,
  deterministic, and stable under repeated verification.
- NFR-006: Admin runtime dependencies do not become dependencies of the API
  Profile. Frontend tooling is required only to customize or rebuild Admin
  source.
- NFR-007: no new compatibility wrapper preserves an architecture rejected by
  this contract. Retained Alpha 3 behavior is tested directly; replaced APIs
  receive an upgrade guide.

## Success Criteria

- SC-001: `modary new example --profile api` produces a copied-out application
  that builds, tests, starts, answers health and its example endpoint, and has no
  database or Node.js prerequisite.
- SC-002: `modary new example --profile admin` produces a copied-out
  application that initializes PostgreSQL, signs in through the Admin UI, and
  creates, lists, edits, and deletes records owned by its example business
  component.
- SC-003: the same Admin application can omit optional task and audit components
  and proves their routes, migrations, processes, and navigation are absent.
- SC-004: the Governed Profile previews and executes one high-impact example
  Action, writes its required audit record, transactionally enqueues work, and
  consumes that work after producer restart.
- SC-005: component and generated-code ownership tests prove that no framework
  tool overwrites handwritten consumer source.
- SC-006: framework, copied-out consumers, frontend type/lint/build tests,
  desktop and mobile visual QA, docs, vet, race, repeat, build, and strict
  delivery validation pass with no unresolved P0 through P2 finding.

## Non-Goals

- Matching the complete Gin-Vue-Admin feature catalog.
- A low-code page, form, workflow, or database-schema designer.
- Runtime binary plugins, hot module loading, or a plugin marketplace.
- Microservice discovery, infrastructure provisioning, or a hosted cloud
  control plane.
- MySQL, SQLite, MongoDB, Oracle, or SQL Server official components in v0.2 F0.
- A generic ORM or repository abstraction over every business query.
- Full enterprise IAM, multi-organization SaaS policy, or distributed
  transactions.
- Repeated CRUD generation into handwritten source.

## Stop Conditions

Execution pauses for owner review if the Admin F0 requires a low-code builder,
if Core cannot be made database-free without discarding the accepted module
kernel, if component removal requires runtime flags instead of composition, or
if a proposed convenience requires source rewriting across handwritten files.

## Acceptance Boundary

Acceptance requires independent copied-out Profile consumers, current
implementation and visual evidence, complete onboarding documentation, an
explicit Alpha 3 to v0.2 compatibility report, and two current review passes
with no unresolved P0, P1, or P2 finding. A full Rulary product implementation
is not an acceptance dependency; only a domain-neutral external vertical slice
is required.
