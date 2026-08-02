# Lightweight Component Framework Refoundation Plan

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-08-02

## Technical Decisions

1. Retain `module` and `appkit` as the Core rather than replacing the accepted
   graph, typed capability, and lifecycle implementation. Remove their product
   positioning dependency on Governed Actions and durable storage.
2. Define a component as a normal package that returns `module.Registration`.
   Avoid a second dependency-injection container or runtime plugin model.
3. Add optional route and application-service capabilities instead of giving
   modules a global router. The smallest server uses `net/http`; components
   register through a bounded composition surface.
4. Keep official adapters in one repository for F0. Go only links imported
   packages, while copied-out Profile tests prove that unselected packages do
   not appear in source, runtime initialization, routes, or configuration.
   Multi-module distribution is deferred until measured module-download weight
   justifies its release complexity.
5. Implement `modary new` as create-only deterministic template expansion. It
   refuses non-empty destinations, validates module paths and names, and never
   provides a regenerate-over-business-source operation.
6. Make Profiles template presets whose output contains explicit imports and
   registration lists. There is no hidden Profile switch after generation.
7. Use Vue 3, TypeScript, Vite, Pinia, and pnpm for the optional Admin source.
   Consumers can run a released production bundle without Node.js; Node.js is
   needed only to customize or rebuild the frontend.
8. Keep normal HTTP handlers available for ordinary product workflows. Reuse
   the existing Action Runtime unchanged where possible as the advanced
   governed-operation component.
9. Retain PostgreSQL as the only official durable database in F0. Split the
   conceptual general SQL store, governed control storage, tasks, and governed
   persistence selections so Core and API generation never require a connection
   string. Ordinary repositories receive a narrow provider-neutral query and
   transaction callback contract; governed transaction authority stays sealed.
10. Use copied-out generated applications as the primary acceptance objects.
    In-repository examples are fixtures, not privileged consumers.

## Delivery Sequence

1. Confirm the product contract from owner input and competitor/Issue research.
2. Establish database-free Core HTTP composition and component-absence tests.
3. Implement create-only CLI generation and the API Profile.
4. Compose the Admin backend Profile from optional PostgreSQL, Identity, RBAC,
   session, and business components.
5. Build and visually verify the Admin UI and explicit frontend module registry.
6. Express accepted Action, Audit, River, CLI, HTTP, and MCP capabilities as the
   Governed Profile without weakening Alpha 3 conformance.
7. Generate every Profile outside the repository with `GOWORK=off`, run its
   complete tests, and record dependency and absence evidence.
8. Update canonical documentation, compatibility guidance, and final review
   evidence, then accept v0.2 F0 without moving the Alpha 3 tag.

## Component Defaults

| Capability | Core | API | Admin | Governed |
|---|---:|---:|---:|---:|
| Module graph and lifecycle | yes | yes | yes | yes |
| HTTP and health | no | yes | yes | yes |
| PostgreSQL | no | no | yes | yes |
| Identity and session | no | no | yes | yes |
| RBAC | no | no | yes | yes |
| Admin UI | no | no | yes | optional |
| SQL audit | no | no | optional | yes |
| River tasks | no | no | optional | yes |
| Governed Actions | no | no | no | yes |
| MCP | no | no | no | yes |

The table defines generator presets, not permanent restrictions. Consumers edit
their explicit composition after creation.

## Admin Architecture

- The frontend is a normal consumer-owned Vue application produced by the Admin
  Profile and backed by a versioned Modary Admin client contract.
- Framework-maintained UI packages own the application shell and reusable
  primitives. The generated consumer owns branding and the explicit module
  route registry.
- Backend Admin endpoints derive actor and permissions from selected Identity
  and RBAC capabilities. UI visibility is a convenience; backend authorization
  remains authoritative.
- A generic example `records` component proves list, create, edit, and delete
  without introducing downstream product vocabulary into framework source.
- Task and audit pages are contributed only when their components are selected.

## Migration Strategy

- Preserve the `v0.1.0-alpha.3` tag and its evidence unchanged.
- Prefer additive packages and composition first. Break or remove an Alpha 3
  public API only when its existence forces database, Action, or runtime
  authority into Core.
- Maintain conformance tests for retained module lifecycle, PostgreSQL
  migrations, governed transaction, River recovery, Identity, RBAC, Audit,
  HTTP, CLI, MCP, and deterministic project inspection.
- Publish one upgrade guide classifying Alpha 3 APIs as retained, optionalized,
  replaced, or removed.

## Risk Controls

- Every implementation task includes a copied-out or absence test so an option
  is not merely dormant.
- Starter templates compile continuously rather than only at final acceptance.
- Admin source and prebuilt assets have one reproducible build and drift check.
- UI review uses Playwright desktop and mobile screenshots, keyboard flows, and
  overflow checks.
- Existing security hardening remains unless it conflicts with the confirmed
  lightweight boundary; simplification requires focused regression tests.
- No business framework work begins in the Rulary repository during this goal.
  A narrow external slice is used only after the framework Profiles are stable.
- Component removal never executes destructive down migrations. Absence tests
  use fresh assembly and fresh database state; retained-data cleanup is an
  explicit operator migration outside automatic composition.
