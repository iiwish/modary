# Component Boundary Closure Plan

- Version: 1.0
- Status: Confirmed
- Approval source: owner instruction on 2026-08-02
- Last updated: 2026-08-02

## Technical Decisions

1. Keep `github.com/iiwish/modary` as the lightweight Core/contracts/tooling
   module. Move PostgreSQL-backed standard components to
   `github.com/iiwish/modary/components/postgres` and governed PostgreSQL/River
   to `github.com/iiwish/modary/components/governedpostgres`.
2. Test cross-module behavior from an unshipped repository integration module.
   Root `go.mod` never requires either heavy component module.
3. Give every module its own tidy, test, vet, vulnerability, and release checks.
   The release train uses matching prerelease versions and subdirectory tags.
4. Add pure `httpkit.Plan` preflight over Module manifests and explicit
   `httpkit.Contribution` values. Builders run only after preflight and Module
   startup, and assembly failure performs bounded shutdown.
5. Add an Admin contribution descriptor to HTTP metadata. Static generated
   source owns the React component registry; backend metadata supplies only the
   selected descriptors and current actor grants, never executable UI.
6. Extend narrow task and audit contracts with bounded read models. Concrete
   components translate River/SQL state behind those contracts.
7. Add repeatable Admin creation selections for task and audit. Conditional
   templates include backend contribution source, frontend modules, dependencies,
   configuration, and assets only when selected.
8. Extract small Admin primitives only where records, tasks, and audit share real
   behavior. Do not introduce a third-party component framework or generic schema
   renderer.

## Delivery Sequence

1. Split modules and make module-graph absence tests fail then pass.
2. Implement contribution preflight and migrate generated API/Admin routing.
3. Add grant metadata, shared primitives, task/audit contracts, conditional
   generation, APIs, and React views.
4. Generate clean consumers, perform browser QA, run every module/release gate,
   and replace premature acceptance evidence with current results.

## Risk Controls

- Nested-module release drift: one release script validates all module versions
  and required submodule tags.
- Test coverage loss during moves: tests move with their implementation; public
  cross-module conformance remains in the integration module.
- Contribution dependency escape: negative tests prove missing providers fail
  before any Start callback or migration read.
- Permission confusion: grants affect presentation only and every protected API
  retains backend authorization tests.
- Operational data leakage: task/audit readers enforce scope and fixed limits and
  return provider-neutral DTOs.
