# Component Boundary Closure Work Graph

- Version: 1.0
- Status: Confirmed
- Approval source: owner instruction on 2026-08-02
- Last updated: 2026-08-02

## T038: Heavy Module Isolation

Status: Completed
Priority: P0
Story / Requirement: FR-001 through FR-003, NFR-002, NFR-005, NFR-006
Depends on: T037
Blocks: T039, T040, T041
Parallel: No
Conflicts with: all other 009 tasks

Goal: remove PostgreSQL and River modules from Core/API module graphs while
retaining separately tested official components.

Test targets: root and generated API module graphs, ordinary Admin River
absence, governed component presence, all nested modules, copied-out Counter,
import direction, release version alignment, and cross-module conformance.

Deliverables: lightweight root module; independently versioned ordinary and
governed PostgreSQL modules; integration test module; updated Profiles, example,
CI, Make gates, docs, and T038 evidence.

Acceptance criteria: root/API contain no PostgreSQL or River module; default
Admin contains PostgreSQL but no River; governed consumers retain River; every
shipped module is independently tidy, testable, vettable, and releasable.

Allowed files: root and nested `go.mod`/`go.sum`, `adapters/**`,
`components/**`, integration tests, generated Profile imports/templates,
Makefile/scripts/CI/release metadata, feature 009 governance, and T038 evidence.

Validation commands: module-graph RED/GREEN tests; all-module tidy/test/vet/race
and vulnerability gates; generated API/Admin/Governed dependency graphs;
cross-module conformance; strict T038 artifacts; `git diff --check`.

Definition of Done: API has no PostgreSQL/River module, default Admin has no
River module, heavy modules are independently versioned and tested, and no
P0-P2 review finding remains.

TDD plan: RED makes generated API reject PostgreSQL/River in `go list -m all`.
GREEN moves concrete implementations to nested modules and rewires explicit
consumers. REFACTOR consolidates multi-module quality and release gates.

Packet path: `.ai-platform/specs/009-component-boundary-closure/packets/T038.yaml`

Evidence required: `.ai-platform/evidence/T038/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

## T039: HTTP And Admin Contribution Contracts

Status: Completed
Priority: P0
Story / Requirement: FR-004 through FR-006, NFR-001, NFR-003
Depends on: T038
Blocks: T040, T041
Parallel: No
Conflicts with: all other 009 tasks

Goal: make route and Admin dependencies explicit, pure-preflighted, and
deterministically assembled before startup side effects.

Test targets: immutable application contracts, contribution identifiers,
capability requirements, route conflicts and drift, Admin metadata and
permission inventory, callback deferral, pre-start failure, and generated API
and Admin composition.

Deliverables: pure `appkit.Preflight`, immutable `appkit.Contract`, bounded
`httpkit.Contribution`/`Plan` contracts, migrated API/Admin templates, focused
tests, documentation, and T039 evidence.

Acceptance criteria: selected routes and Admin descriptors are inspectable and
defensively owned; missing or duplicate dependencies fail before startup;
builders run only after a matching application is ready.

Allowed files: `module/**`, `appkit/**`, `httpkit/**`, relevant transports,
Starter Profile application/feature templates and tests, documentation, feature
009 governance, and T039 evidence.

Validation commands: contribution RED/GREEN and side-effect tests; generated
API/Admin builds; root/all-module test and race; docs; strict T039 artifacts;
`git diff --check`.

Definition of Done: selected routes and Admin descriptors are inspectable;
missing/duplicate dependencies fail before startup; generated records no longer
hides authorization/session dependencies.

TDD plan: RED declares an unavailable contribution capability and proves no
Module start occurs. GREEN adds pure planning and migrates generated composition.
REFACTOR bounds/copies metadata and rejects route or permission drift.

Packet path: `.ai-platform/specs/009-component-boundary-closure/packets/T039.yaml`

Evidence required: `.ai-platform/evidence/T039/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

## T040: Operational Admin Surface

Status: Completed
Priority: P0
Story / Requirement: FR-007 through FR-012, NFR-004
Depends on: T039
Blocks: T041
Parallel: No
Conflicts with: all other 009 tasks

Goal: add permission-aware composition, shared Admin primitives, and optional
task/audit APIs and React views without weakening backend authorization.

Test targets: bounded task/audit readers, scope isolation, cursor/filter
behavior, component selection validation, conditional source/config/module
graphs/assets, authenticated metadata and APIs, restricted grants and direct
forbidden requests, shared primitives, task/audit states, accessibility,
responsive layout, and deterministic bundles.

Deliverables: public read contracts and facades; PostgreSQL/River readers;
repeatable `--with tasks|audit`; permission-bearing Admin context; shared React
work-surface primitives; task/audit views and APIs; four canonical bundles;
English/Chinese onboarding; T040 evidence.

Acceptance criteria: default Admin remains River/audit-free; selected readers
are bounded and read-only; audit is actor-scope-bound; frontend visibility
follows grants while backend denial remains authoritative; selected generated
source and assets rebuild byte-identically.

Allowed files: public task/audit contracts, component implementations, Admin
templates/assets/tests, Starter selection inputs, onboarding, feature 009
governance, and T040 evidence.

Validation commands: Go observer/API and generator tests; frontend
lint/type/test/build/assets/audit; copied-out default and operational Admin;
real PostgreSQL generated tests; desktop/mobile browser QA; strict T040
artifacts; `git diff --check`.

Definition of Done: default Admin remains minimal; selected task/audit surfaces
are complete, scoped, bounded, permission aware, accessible, and absent when not
selected; no P0-P2 finding remains.

TDD plan: RED requires optional source/module-graph absence and restricted
permission behavior. GREEN adds readers, generated contributions, context, and
views. REFACTOR extracts only primitives shared by concrete Admin modules and
stabilizes variant asset identity.

Packet path: `.ai-platform/specs/009-component-boundary-closure/packets/T040.yaml`

Evidence required: `.ai-platform/evidence/T040/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, and browser/copyout acceptance details.

## T041: External Acceptance And Release Readiness

Status: Completed
Priority: P0
Story / Requirement: FR-013, FR-014, all 009 acceptance criteria and NFRs
Depends on: T038, T039, T040
Blocks: owner release decision
Parallel: No
Conflicts with: every implementation task

Goal: prove the closed component boundary in fresh external consumers and
restore truthful Modary v0.2 engineering readiness.

Test targets: copied-out API/default Admin/operational Admin/Governed consumers,
source and module-graph absence, all-module tests and race, frontend frozen
pipeline and production audit, generated asset parity before and after rebuild,
PostgreSQL acceptance, browser desktop/mobile behavior, docs, neutrality, and
four-pass final review.

Deliverables: copied-out consumer records; current generated assets and docs;
repository and external test results; final spec/engineering/UX/release review;
T041 evidence.

Acceptance criteria: fresh consumers build and test with `GOWORK=off`; default
and operational Admin assets match their selected source; all module and
frontend gates pass; browser layout and console are clean; no P0-P2 finding
remains. Publication and clean committed-candidate release preflight stay
owner-controlled after this uncommitted implementation handoff.

Allowed files: tests, scripts, CI, release/docs/governance/evidence, generated
canonical assets, and narrowly required fixes found by review, including the
official PostgreSQL components, shared internal schema coordination, public
task/audit wire contracts, and Admin module source.

Validation commands: copied-out consumer gates; all-module tidy/test/vet/race;
frontend lint/type/test/build/assets/audit; docs/links/neutrality/generated state;
desktop/mobile browser QA; strict T041 artifacts; `git diff --check`. Repeat,
fuzz, cross-build, vulnerability, and clean release preflight remain normal
release-candidate gates and are not replaced by this F0 handoff.

Definition of Done: every 009 F0 acceptance criterion passes in the implementation
worktree, acceptance evidence matches the implementation, no P0-P2 finding
remains, and clean committed-candidate release preflight is explicitly deferred
to the owner-controlled commit/release step.

TDD plan: RED runs external generation and exposes dependency/asset drift.
GREEN fixes generated source, graphs, tests, assets, and docs. REFACTOR performs
spec, engineering, UX/accessibility, and release-readiness review without
expanding product scope.

Packet path: `.ai-platform/specs/009-component-boundary-closure/packets/T041.yaml`

Evidence required: `.ai-platform/evidence/T041/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, and copied-out/browser acceptance details.
