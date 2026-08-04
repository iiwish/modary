# Component Boundary Closure F0

- Version: 1.0
- Status: Confirmed
- Approval source: owner instruction on 2026-08-02
- Last updated: 2026-08-02
- Governing constitution: `../../memory/constitution.md`

## Objective

Close the remaining gap between Modary's lightweight component-framework
contract and its implementation. Optional PostgreSQL and River implementations
must be absent from unselected Go module graphs, HTTP and Admin contributions
must participate in explicit pre-start composition, and the optional React Admin
surface must provide permission-aware reusable primitives plus selected task and
audit visibility.

The work does not add downstream product behavior, a low-code system, runtime plugin
discovery, compatibility wrappers, or a maximal Admin Profile.

## Requirements

- FR-001: the root Modary module and a generated API consumer have no PostgreSQL
  driver, River, or database-component module in `go list -m all`.
- FR-002: ordinary PostgreSQL components and governed PostgreSQL/River components
  live in separately versioned Go modules. Selecting ordinary PostgreSQL does not
  add River to the consumer module graph.
- FR-003: repository, CI, release, documentation, copied-out consumers, and
  vulnerability checks validate every shipped module without making heavy
  modules dependencies of Core.
- FR-004: HTTP contributions have stable identity, declared capabilities, pure
  preflight validation, and deterministic route assembly. Missing providers and
  duplicate contribution identities fail before Module startup.
- FR-005: Admin contributions have stable identity, route, label, icon key,
  required permissions, and optional backend feature requirements. The complete
  selected registry is validated before startup and served as immutable metadata.
- FR-006: a consumer feature does not hide HTTP, authorization, session, task, or
  audit dependencies outside its declared contribution contract.
- FR-007: the React shell renders navigation and commands from selected Admin
  contributions and current actor grants. Backend authorization remains
  authoritative; frontend permission handling is usability only.
- FR-008: shared React primitives cover work-surface headers, data tables,
  pagination, and loading/empty/error states across real modules without
  becoming a general UI framework. Feature-specific filters, dialogs,
  confirmations, and status semantics remain with their owning feature until a
  second concrete use proves a shared contract.
- FR-009: Admin task visibility is included only when the task component is
  selected and uses a bounded public read contract rather than River internals.
- FR-010: Admin audit visibility is included only when the audit component is
  selected and uses a bounded public read contract rather than SQL tables from
  frontend or transport code.
- FR-011: Admin component selection is explicit at project creation and visible
  in generated Go and React source. The default Admin Profile remains free of
  River and audit.
- FR-012: task and audit list APIs are authenticated, permission checked,
  bounded, and read-only; audit reading is actor-scope-bound while task reading
  is application-queue operational metadata. Empty, populated, forbidden,
  invalid-query, and failure behavior is covered at the appropriate test layer.
- FR-013: generated API, default Admin, operational Admin, and Governed consumers
  build and test outside the checkout with `GOWORK=off`.
- FR-014: current F0 acceptance and release evidence is reopened until all
  requirements pass with no unresolved P0 through P2 finding.

## Non-Functional Requirements

- NFR-001: composition remains static Go source; no reflection-based package
  discovery, service locator, source patcher, or runtime component marketplace.
- NFR-002: Core keeps standard Go contracts and no frontend or concrete database
  implementation dependency.
- NFR-003: contribution preflight is side-effect free, deterministic, bounded,
  and produces stable diagnostics.
- NFR-004: frontend code remains strict TypeScript with focused React context and
  no general state or component framework dependency.
- NFR-005: every module passes format, tidy, test, vet, vulnerability, race where
  applicable, deterministic generation, module-graph absence, and cross-build
  checks.
- NFR-006: published submodule tags and root tags are immutable and share one
  documented release train for the v0.2 prerelease.

## Acceptance Criteria

- AC-001: generated API `go list -m all` contains neither PostgreSQL nor River;
  default Admin contains PostgreSQL but not River; task-enabled Admin and Governed
  contain River intentionally.
- AC-002: removing an HTTP/Admin contribution or its provider changes routes,
  metadata, source, assets, configuration, and module graph by construction.
- AC-003: missing contribution dependencies fail before database schemas,
  migrations, goroutines, or listeners are created.
- AC-004: browser acceptance proves the selected all-permission surface;
  permission-resolution and restricted-command tests prove fail-closed UI, while
  real generated-backend tests prove direct forbidden API requests fail.
- AC-005: task and audit views pass desktop/mobile, accessibility, loading,
  empty, populated, forbidden/failure, retry, filtering, and session-expiry
  checks at the appropriate unit, generated-consumer, and browser layers.
- AC-006: strict delivery artifacts, copied-out consumers, and final
  spec/engineering/UX/release reviews pass for the implementation handoff.
  Clean committed-candidate preflight remains the next owner-controlled release
  step and is not claimed from a dirty implementation worktree.

## Non-Goals

- MySQL, SQLite, or another official database implementation.
- A generic runtime plugin loader or component marketplace.
- Schema-driven CRUD generation or a low-code designer.
- A generic design system package for unrelated frontend applications.
- Downstream product modules, screens, schemas, or policies.
- Compatibility imports for the unreleased v0.2 candidate layout.

## Stop Conditions

Pause for owner review if module isolation requires changing the published
`v0.1.0-alpha.3` tag, if permission metadata would become authorization
authority, if task/audit visibility requires exposing raw River or SQL handles,
or if component selection requires rewriting existing consumer source.
