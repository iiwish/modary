# Lightweight Component Framework Refoundation Work Graph

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-08-02
- Approval source: explicit owner approval on 2026-08-02

## T028: Product Contract And Research

Status: Completed
Priority: P0
Depends on: T027
Blocks: T029
Story / Requirement: PR-001, PR-002, and PR-003
Parallel: No
Conflicts with: every feature 007 implementation task

Goal: Establish the canonical lightweight component-framework product contract
from owner input, repository evidence, and relevant Gin-Vue-Admin Issues.

Allowed files: `.ai-platform/memory/constitution.md`,
`.ai-platform/docs/product-design.md`, `.ai-platform/docs/technology-decision-record.md`,
`.ai-platform/docs/tasks.md`, `.ai-platform/specs/007-component-framework-refoundation/**`,
`.ai-platform/evidence/T028/**`, and documentation-checker expectations required
for the new canonical artifacts.

Test targets: canonical product-scope consistency, primary research-source
integrity, requirements completeness, documentation links, and strict delivery
artifact shape.

Deliverables: confirmed constitution, product contract, competitor research,
feature spec, technical plan, work graph, checklist, analysis, execution packet,
and T028 evidence.

Acceptance criteria: all artifacts agree that Core is database-free, Admin and
Governed capabilities are optional, generation is create-only, omitted
components are absent, and Alpha 3 is immutable; no Critical or High ambiguity
or unresolved P0 through P2 review finding remains.

Validation commands:
- `./scripts/check-docs.sh`
- `./scripts/check-doc-links.sh`
- strict T028 artifact validation
- `git diff --check`

Definition of Done: target users, lightweight value, component boundaries,
Profiles, Admin scope, compatibility posture, research evidence, non-goals,
stop conditions, and measurable acceptance agree across canonical artifacts
with no unresolved Critical or High ambiguity.

TDD plan: documentation-only exception; strict artifact, documentation, link,
and diff validation replace behavior TDD.

Packet path: `.ai-platform/specs/007-component-framework-refoundation/packets/T028.yaml`

Evidence required: `.ai-platform/evidence/T028/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

## T029: Lightweight Core And Component Surfaces

Status: Completed
Priority: P0
Depends on: T028
Blocks: T030, T031
Story / Requirement: FR-001, FR-002, FR-003, FR-006, NFR-001, NFR-002, and NFR-003
Parallel: No
Conflicts with: T030 through T034

Goal: make Core and the standard HTTP application surface database-free while
preserving explicit graph, capability, and lifecycle guarantees.

Allowed files: `module/**`, `appkit/**`, `transport/httpapi/health*`, focused
`appcmd/**` tests if required, feature 007 governance, T029 evidence, and
documentation-checker expectations.

Test targets: Host assembly without Actions, AppKit startup without governance
providers, empty catalog and nil optional Runtime behavior, health readiness,
explicit consumer HTTP routing, missing-governance failure for an application
that does declare Actions, lifecycle shutdown, race detection, and package
import direction.

Deliverables: optional Action Runtime assembly, database-free public startup
tests, retained governed failure semantics, updated package documentation,
packet, test results, diff manifest, and review.

Acceptance criteria: a focused independent consumer starts health and one feature route
without PostgreSQL, Action, Identity, RBAC, Audit, River, MCP, or Node.js;
the assembled Application has no Action Runtime; declaring an Action without
its governed providers still fails closed; component absence and lifecycle
tests pass.

Validation commands:
- `go test ./module ./appkit ./transport/httpapi ./appcmd`
- `go test -race ./module ./appkit ./transport/httpapi`
- `go vet ./module ./appkit ./transport/httpapi ./appcmd`
- strict T029 artifact validation
- `git diff --check`

Definition of Done: database-free startup is a supported public contract,
governed applications retain fail-closed assembly, focused and race tests pass,
and review contains no unresolved P0 through P2 finding.

TDD plan: RED adds public database-free startup tests that fail on required
governance resolution; GREEN makes Action Runtime assembly conditional on
declared Actions; REFACTOR updates lifecycle descriptions and runs focused race
and vet gates.

Packet path: `.ai-platform/specs/007-component-framework-refoundation/packets/T029.yaml`

Evidence required: `.ai-platform/evidence/T029/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

## T030: Create-Only CLI And API Profile

Status: Completed
Priority: P0
Depends on: T029
Blocks: T031, T034
Story / Requirement: FR-007, FR-008, FR-009, FR-012, FR-018, FR-019, NFR-001, NFR-004, and NFR-005
Parallel: No
Conflicts with: T031 through T034

Goal: implement a create-only project API and `modary new` command that render
the database-free API Profile with explicit application, component, route, and
lifecycle composition.

Allowed files: `starter/**`, `cmd/modary/**`, `httpkit/**`, focused
`internal/quality/**`, `scripts/**`, feature 007 governance, T030 evidence, and
current task/documentation-checker state.

Test targets: strict argument and module-path parsing, unavailable Profiles,
empty and nonexistent destinations, non-empty and symlink rejection,
cancellation, deterministic output, create-only repeat behavior, bounded route
composition, copied-out `GOWORK=off` tests and build, source import absence,
process startup, shutdown, and package import direction.

Deliverables: reusable Starter API, global CLI entry point, API Profile
templates, bounded standard HTTP route composer, copied-out acceptance tests,
packet, evidence, and review.

Acceptance criteria: `modary new example --profile api` renders visible Go
composition into a new or empty destination; the source imports no PostgreSQL,
River, Identity, RBAC, Audit, Action, or MCP package; the copied-out project
passes tests and build with `GOWORK=off`, answers health and ping, shuts down,
and a second creation cannot alter any file.

Validation commands:
- `go test ./starter ./httpkit ./cmd/modary`
- `go test -race ./starter ./httpkit`
- `go vet ./starter ./httpkit ./cmd/modary`
- `go test ./... -count=1`
- strict T030 artifact validation
- `git diff --check`

Definition of Done: generation and HTTP composition are deterministic,
create-only, context-aware, copied-out verified, and reviewed with no unresolved
P0 through P2 finding.

TDD plan: RED adds public creation, safety, CLI, route, and copied-out tests;
GREEN implements the smallest reusable generator and API Profile; REFACTOR
hardens filesystem rollback, comments, architecture inventory, and race gates.

Packet path: `.ai-platform/specs/007-component-framework-refoundation/packets/T030.yaml`

Evidence required: `.ai-platform/evidence/T030/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

## T031: Optional Admin Backend Profile

Status: Completed
Priority: P0
Depends on: T029, T030
Blocks: T032
Story / Requirement: FR-004, FR-005, FR-010, FR-020, FR-021, FR-022, NFR-002, and NFR-003
Parallel: No
Conflicts with: T032 through T034

Goal: compose PostgreSQL, development Identity, RBAC, session HTTP, ordinary
business transactions, and a consumer-owned records component without moving
database, authorization, authentication, or product routes into Core.

Allowed files: `database/**`, `module/**`, `appkit/**`,
`internal/databasecontrol/**`, `internal/moduleassembly/**`,
`adapters/postgres/**`, focused `adapters/**`, `authz/**`,
`internal/actionruntime/**`, `transport/**`, `starter/**`,
`internal/quality/**`, `scripts/**`, generated Admin backend fixtures, feature
007 governance, and T031 evidence.

Test targets: ordinary Store transaction authority versus governed Access,
general PostgreSQL startup without queue/River/Action persistence, optional
database and Authorizer facades, session-only login/current/logout and
middleware, generated Admin backend build, migrations, RBAC allow/deny, CRUD,
scope isolation, CSRF, absent task/audit/Action surfaces, restart, lifecycle,
race, and import direction.

Deliverables: provider-neutral business Store, general PostgreSQL component,
session HTTP component, AppKit optional standard facades, Admin backend Profile,
records vertical slice, packet, evidence, and review.

Acceptance criteria: a copied-out Admin backend uses PostgreSQL but initializes
no River schema or worker, signs in with explicit development credentials,
authorizes and completes scoped records create/list/update/delete through RBAC,
persists across restart, and exposes no selected task, audit, governed Action,
or MCP route/service. Governed database Access remains unable to begin its own
transaction.

Validation commands:
- focused database, module, AppKit, PostgreSQL, HTTP, Starter, and adapter tests
- focused race and vet
- `go test ./... -count=1`
- strict T031 artifact validation
- `git diff --check`

Definition of Done: ordinary and governed transaction authority are separated,
the Admin backend is copied-out verified with real PostgreSQL, optional
components are absent by construction, and review has no unresolved P0 through
P2 finding.

TDD plan: RED adds Store authority, session HTTP, general PostgreSQL, and copied-out
Admin tests; GREEN implements the narrow contracts and Profile; REFACTOR removes
Action-specific naming from general authorization paths, tightens lifecycle and
filesystem boundaries, and runs race/full gates.

Packet path: `.ai-platform/specs/007-component-framework-refoundation/packets/T031.yaml`

Evidence required: `.ai-platform/evidence/T031/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

## T032: Admin UI F0

Status: Completed
Priority: P0
Depends on: T031
Blocks: T034
Story / Requirement: FR-011, FR-013, FR-014, FR-015, FR-016, FR-017, NFR-001, NFR-004, and NFR-005
Parallel: No
Conflicts with: T033 and T034

Goal: deliver the optional Vue 3 Admin work surface, explicit frontend module
registry, authentication state, responsive records workflow, and reproducible
prebuilt production assets without adding unselected product surfaces.

Allowed files: `starter/templates/admin/**`, `starter/**`, focused
`transport/httpapi/spa*`, `httpkit/**`, `scripts/**`, `.gitignore`, feature 007
governance, T032 evidence, and current task/documentation-checker state.

Test targets: frontend module registry, unauthenticated routing, login and
session restoration, record loading/filtering/create/update/delete, CSRF/error
handling, logout, modal keyboard behavior, accessible names and contrast,
responsive navigation/table behavior, production build, deterministic asset
drift, generated Go embedding, copied-out build/test, and unselected navigation
absence.

Deliverables: Vue 3/TypeScript/Vite/Pinia Admin source, quiet work-focused shell,
records vertical-slice UI, explicit module registry, Go-embedded prebuilt assets,
backend metadata and SPA composition, frontend unit/a11y/visual evidence,
packet, evidence, and review.

Acceptance criteria: a copied-out Admin project runs with no Node.js after Go
build, restores or requests a session, supports the complete records workflow,
and remains usable by keyboard at desktop and mobile widths. The source build
is reproducible with pnpm, generated assets match the canonical build, and the
registry contains only the selected records module; task, audit, Action, MCP,
plugin marketplace, and runtime UI generation are absent.

Validation commands:
- `pnpm install --frozen-lockfile`, `pnpm lint`, `pnpm typecheck`, `pnpm test`,
  `pnpm build`, and `pnpm assets:check` in the Admin web source
- focused Starter, SPA, and copied-out Admin tests
- Playwright desktop and mobile screenshots plus keyboard and overflow checks
- strict T032 artifact validation
- `git diff --check`

Definition of Done: frontend behavior, accessibility, responsive layout,
prebuilt asset reproducibility, generated embedding, and component absence are
proven with no unresolved P0 through P2 finding.

TDD plan: RED adds registry/auth/records and generated-asset expectations;
GREEN implements the smallest complete Admin workflow; REFACTOR performs design,
accessibility, responsive, visual, asset-drift, copied-out, and review gates.

Packet path: `.ai-platform/specs/007-component-framework-refoundation/packets/T032.yaml`

Evidence required: `.ai-platform/evidence/T032/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, and desktop/mobile screenshots.

## T033: Governed Profile Optionalization

Status: Completed
Priority: P0
Depends on: T029, T030
Blocks: T034
Story / Requirement: FR-005, FR-008, FR-011, FR-018, FR-019, NFR-002, NFR-003, NFR-004, and NFR-005
Parallel: No
Conflicts with: T034

Goal: express the accepted PostgreSQL, River, SQL Audit, Governed Action, CLI,
HTTP, and MCP guarantees as a visible optional Profile with a narrow
consumer-owned high-impact feature and worker, without weakening Alpha 3
conformance or introducing those dependencies into API or Admin Profiles.

Allowed files: `starter/**`, `cmd/modary/**`, focused `appcmd/**`,
`transport/httpapi/**`, `internal/quality/**`, `scripts/**`, feature 007
governance, T033 evidence, and current task/documentation-checker state.

Test targets: Governed Profile selection and deterministic output, explicit
configuration, module graph, Action schema/Preview/Execute, RBAC default deny,
optimistic stale-plan behavior, idempotency, SQL Audit, transactional River
enqueue, shutdown/restart persistence, worker consumption, CLI/HTTP/MCP route
composition, copied-out `GOWORK=off` build/test/vet, and API/Admin dependency
absence regression.

Deliverables: available `governed` Starter Profile, PostgreSQL/River composition,
consumer-owned governed limit example, dedicated worker binary, CLI/HTTP/MCP
composition, copied-out real-PostgreSQL acceptance, packet, evidence, and
review.

Acceptance criteria: `modary new example --profile governed` produces an
inspectable Go-only project. Outside the framework checkout it previews and
executes a required-preview Action, commits business state and one River task
atomically, persists plans/idempotency/audit across restart, consumes the work
with the generated task handler, and fails closed for an unbound actor. API and
Admin generated dependency graphs remain free of River and governed adapters.

Validation commands:
- focused Starter and generated-profile tests against real PostgreSQL
- copied-out `GOWORK=off` tidy, test, build, vet, and dependency inspection
- focused race and vet
- existing governed adapter, Runtime, transport, and Counter conformance gates
- strict T033 artifact validation
- `git diff --check`

Definition of Done: the governed stack is a selectable Profile rather than a
Core default, the generated composition and worker teach the accepted
transaction boundary, copied-out restart/consumption passes, previous security
and recovery guarantees remain green, and review has no unresolved P0 through
P2 finding.

TDD plan: RED makes ProfileGoverned creation and copied-out transactional-task
acceptance fail as unavailable; GREEN implements the smallest explicit
composition and consumer Action; REFACTOR tightens configuration, worker
lifecycle, copied-out isolation, absence regression, documentation, and full
accepted conformance.

Packet path: `.ai-platform/specs/007-component-framework-refoundation/packets/T033.yaml`

Evidence required: `.ai-platform/evidence/T033/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

## T034: External Acceptance, Documentation, And Review

Status: Completed
Priority: P0
Depends on: T030, T032, T033
Blocks: Modary v0.2 F0 acceptance
Story / Requirement: all feature 007 FRs, NFRs, and success criteria
Parallel: No
Conflicts with: release mutation and Rulary product implementation

Goal: prove all Profiles outside the framework checkout, treat the generated
limits feature as the narrow domain-neutral governed external vertical slice,
publish canonical English and Chinese onboarding/architecture/operations
documentation, document the Alpha 3 to v0.2 breaking boundary, and complete
independent product and engineering review evidence.

Allowed files: current framework source and tests required by final findings,
`starter/**`, `cmd/**`, `scripts/**`, `README.md`, `SECURITY.md`, `docs/**`,
current `.ai-platform/**` canonical state and T034 evidence. The immutable
`v0.1.0-alpha.3` tag and any Rulary product repository are read-only.

Test targets: clean copied-out API/Admin/Governed creation, source and package
graph selection/absence, Go-only runtime, real PostgreSQL Admin and Governed
flows, frontend frozen install/build/tests/assets, worker restart consumption,
full framework tests, race, vet, repeat, generated stability, docs links,
examples/counter conformance, Alpha 3 tag identity, and current-source diff.

Deliverables: canonical product README, profile selection and quickstart,
component authoring, Admin and Governed tutorials, persistence/task model,
deployment/security/reference updates, Chinese onboarding, Alpha 3 migration
guide, Rulary bootstrap guide without product implementation, current v0.2 F0
acceptance/limitations report, external acceptance evidence, two review passes,
and final target evidence.

Acceptance criteria: a new developer can choose and create the right Profile,
understand what is absent, add a consumer module, run Admin or Governed locally,
and identify production replacement points from documentation alone. All three
Profiles pass outside the checkout; generated outputs remain stable; no P0
through P2 finding remains; Alpha 3 tag identity is unchanged; Rulary remains
only an external adoption plan.

Validation commands:
- full framework and copied-out Profile tests
- frontend frozen install, lint, typecheck, unit, build, and asset comparison
- focused and full race/vet/build, repeat, generated, neutrality, source, and
  documentation gates
- existing Counter external consumer conformance
- Alpha 3 tag/ref/tree verification
- strict T034 artifact validation and `git diff --check`

Definition of Done: architecture and documents describe the same current
product, every Profile is independently reproducible and absent-by-construction,
external consumers are proven without workspace resolution, all review findings
are resolved, and the v0.2 F0 target is accepted but not misrepresented as a
published release.

TDD plan: acceptance/documentation stage exception; executable copied-out,
source-stability, link, strict artifact, and review gates replace behavior RED.
Any behavioral finding must receive a focused regression test before repair.

Packet path: `.ai-platform/specs/007-component-framework-refoundation/packets/T034.yaml`

Evidence required: `.ai-platform/evidence/T034/summary.md`, `diff.patch`,
`test-results.md`, `review-product.md`, `review-engineering.md`, and
`external-acceptance.md`.
