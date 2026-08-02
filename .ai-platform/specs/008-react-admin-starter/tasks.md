# React Admin Starter Work Graph

- Version: 1.0
- Status: Confirmed
- Approval source: owner approval on 2026-08-02
- Last updated: 2026-08-02

## T035: React Platform And Architecture Migration

Status: Completed
Priority: P0
Story / Requirement: US-001, US-003, FR-001 through FR-004, FR-007, NFR-001, NFR-002, NFR-005
Depends on: T034
Blocks: T036, T037
Parallel: No
Conflicts with: T036, T037

Goal: replace the Vue toolchain and application architecture with a strict,
small React application while preserving backend contracts, explicit module
composition, and deterministic assets.

Allowed files: `starter/templates/admin/web/**`,
`starter/templates/admin/internal/web/dist/**`,
`starter/templates/admin/application_test.go.tmpl`, `starter/create_test.go`,
feature 008 governance, `.ai-platform/evidence/T035/**`, and
`.ai-platform/docs/tasks.md`.

Test targets: React dependency and source contract, Vue absence, API/CSRF,
authentication initialization/login/logout, explicit module registry and routes,
strict typing, lint, component tests, production build, asset parity, and starter
generation.

Deliverables: React-only frontend dependencies and lockfile; typed application,
router, providers, module registry, example module, and tests; rebuilt production
assets; focused generator contract; T035 implementation and review evidence.

Acceptance criteria: focused Go and frontend gates pass from the checked-in
lockfile; active Admin source, dependencies, and generated output contain no Vue;
React routing, auth, CSRF, module composition, and assets preserve the accepted
backend and generator contracts.

Validation commands:
- `go test ./starter -run 'TestCreateAdmin' -count=1`
- `pnpm install --frozen-lockfile && pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm assets:check` in `starter/templates/admin/web`
- `go test ./starter ./transport/sessionhttp -count=1`
- strict T035 artifact validation
- `git diff --check`

Definition of Done: React owns the complete active Admin frontend, no Vue source
or dependency remains, focused behavior and build checks pass, generated asset
parity is current, and review has no unresolved P0 through P2 finding.

TDD plan: RED adds a generator test that requires React files/dependencies and
rejects Vue, and verifies it fails against the current template. GREEN replaces
the toolchain and architecture. REFACTOR tightens typed providers, hooks, module
contracts, and tests while keeping all gates green.

Packet path: `.ai-platform/specs/008-react-admin-starter/packets/T035.yaml`

Evidence required: `.ai-platform/evidence/T035/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

## T036: Admin Experience And Visual Acceptance

Status: Completed
Priority: P0
Story / Requirement: US-002, FR-005, FR-006, NFR-003, NFR-004
Depends on: T035
Blocks: T037
Parallel: No
Conflicts with: T035, T037

Goal: make the React Admin work surface behaviorally complete, accessible,
responsive, visually coherent, and suitable as the official framework starter.

Allowed files: `starter/templates/admin/web/src/**`, focused frontend config and
tests under `starter/templates/admin/web/**`, rebuilt checked-in Admin assets,
`starter/templates/admin/application.go.tmpl`,
`starter/templates/admin/application_test.go.tmpl`,
feature 008 governance, `.ai-platform/evidence/T036/**`, and
`.ai-platform/docs/tasks.md`.

Test targets: authenticated navigation; record CRUD, search, filters, errors,
empty state, forbidden/session expiry; dialog focus and focus restoration;
mobile navigation; accessibility; desktop/mobile browser flows; overflow;
console/network health; reduced motion.

Deliverables: complete React Admin screens and states; accessible dialogs and
navigation; responsive styles; focused behavior tests; desktop/mobile screenshots
and metrics; T036 implementation and review evidence.

Acceptance criteria: primary workflows succeed with mouse and keyboard at the
accepted viewports; automated accessibility has no violation in tested surfaces;
focus, overflow, console, network, loading, empty, error, forbidden, and expiry
checks pass; visual review has no unresolved P0 through P2 finding.

Validation commands:
- `pnpm lint && pnpm typecheck && pnpm test && pnpm build && pnpm assets:check`
- desktop and mobile browser QA at the generated Admin application
- strict T036 artifact validation
- `git diff --check`

Definition of Done: all required states and workflows are polished and verified
at desktop and mobile widths, keyboard and automated accessibility checks pass,
no blank/overlap/overflow/console/network defect remains, and review has no
unresolved P0 through P2 finding.

TDD plan: RED adds Testing Library expectations for any behavior missing from
the translated UI. GREEN implements user-visible behavior. REFACTOR improves
component boundaries, CSS stability, focus behavior, and visual hierarchy after
the behavior suite is green.

Packet path: `.ai-platform/specs/008-react-admin-starter/packets/T036.yaml`

Evidence required: `.ai-platform/evidence/T036/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, and browser screenshots/metrics.

## T037: Generated Consumer And Release Readiness

Status: Completed
Priority: P0
Story / Requirement: US-001, US-003, FR-008 through FR-010, NFR-001 through NFR-005
Depends on: T035, T036
Blocks: None
Parallel: No
Conflicts with: T035, T036

Goal: prove the React Admin Starter as an independent generated consumer, align
all current documentation and release contracts, and complete final Modary v0.2
release-readiness review.

Allowed files: `starter/**`, `scripts/**`, `Makefile`, `go.mod`, `go.sum`,
`examples/counter/go.mod`, `.github/workflows/ci.yml`, `README.md`,
`CONTRIBUTING.md`, `CHANGELOG.md`, current
canonical `.ai-platform/docs/**`, current framework documentation under `docs/**`,
feature 008 governance, `.ai-platform/evidence/T037/**`, and narrowly required
quality tests. Immutable tags and Alpha 3 evidence are forbidden.

Test targets: clean copied-out Admin Go and React builds/tests, repeated Admin
integration tests in one schema, complete record response timestamps, failed
logout and initialization behavior, WCAG AA normal-text contrast, bundle serving,
Vue absence, production dependency and Go vulnerability scans, docs and links,
current terminology, format/tidy/vet/test/race/repeat/cross-build/fuzz gates,
deterministic generation, strict artifacts, and four independent final review
passes.

Deliverables: external copied-out acceptance record; active-source Vue residue
gate; canonical English and Chinese React documentation; rebuilt release assets;
full test results, diff, four-pass review, and T037 release-readiness evidence.

Acceptance criteria: a clean generated consumer builds, tests, and serves the
React bundle outside the repository; current documentation is React-only and
complete; every declared release command passes freshly; final reviews report no
unresolved P0 through P2 finding.

Validation commands:
- `make acceptance`
- `make race && make repeat && make fuzz-smoke && make cross-build`
- clean copied-out Admin Profile Go and frontend gates with `GOWORK=off`
- two consecutive copied-out Admin integration runs against one PostgreSQL schema
- `./scripts/check-docs.sh && ./scripts/check-doc-links.sh`
- actual-worktree `make release-preflight VERSION=v0.2.0-alpha.1`
- strict T037 artifact validation
- `git diff --check`

Definition of Done: a clean external Admin consumer is React-only and fully
operational, canonical English/Chinese docs are current, every required release
gate passes from fresh state, final reviews contain no unresolved P0 through P2
finding, and the v0.2 React release is ready for owner acceptance.

TDD plan: RED adds focused current-source and generated-output checks that expose
Vue residue or outdated React documentation. GREEN updates generator outputs,
assets, checks, and docs. REFACTOR consolidates deterministic release commands
without weakening any existing gate.

Packet path: `.ai-platform/specs/008-react-admin-starter/packets/T037.yaml`

Evidence required: `.ai-platform/evidence/T037/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, and copied-out acceptance records.
