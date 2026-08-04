# Production Foundation F0 Work Graph

- Version: 1.0
- Status: Confirmed
- Date: 2026-08-04
- Target: v0.3.0-alpha.1
- Execution: direct, sequential, owner-approved autonomous goal

## Epic E010: Production Foundation

### T042: Principal, Scope, And Session Boundary

Status: Completed
Priority: P0
Depends on: T041
Blocks: T043, T044, T047, T048
Story / Requirement: US-001, US-002, FR-001 through FR-004, FR-007, NFR-001, NFR-005
Parallel: No
Conflicts with: every task that consumes identity, authorization, appkit, or generated Profiles

Goal: establish a scope-independent principal and split identity/session
contracts while retaining the local development adapter and multi-scope RBAC.

Allowed files: `identity/**`, `authz/**`, `scope/**`, `module/**`, `appkit/**`,
`transport/sessionhttp/**`, `components/postgres/identitystore/**`,
`components/postgres/rbac/**`, generated Profile templates/tests, affected root
tests/docs, T042 packet/evidence, and module metadata required by the contract.

Test targets: identity contract tests, RBAC multi-scope/zero-scope/default-deny,
local password/session/token rotation and revocation, HTTP session/CSRF, lifecycle,
race, and copied-out local Admin/Governed behavior.

Deliverables: new public contracts, forward PostgreSQL migration when required,
updated adapters/transports/templates, upgrade note, and evidence.

Acceptance criteria: Actor has no Scope; one actor can receive two exact scope
bindings; unbound scopes deny; local login/session/bearer behavior remains secure;
Core dependency neutrality remains.

Definition of Done: focused and broad Go tests, PostgreSQL integration, copied
Profiles, race, docs, source diff, evidence, and review pass.

Validation commands: `go test ./identity ./authz ./module ./appkit ./transport/sessionhttp`;
`go test ./components/postgres/identitystore ./components/postgres/rbac` from the
PostgreSQL module with real database environment; `make copied-profile-acceptance`;
`make ci-core`; strict T042 artifact validation; `git diff --check`.

TDD plan: RED contract/multi-scope tests; GREEN public contract and adapters;
REFACTOR remove obsolete scope coupling and duplicate session responsibilities.

Packet path: `.ai-platform/specs/010-production-foundation/packets/T042.yaml`

Evidence required: `.ai-platform/evidence/T042/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

### T043: Production OIDC Component And Admin Flow

Status: Completed
Priority: P0
Depends on: T042
Blocks: T047, T048
Story / Requirement: US-001, US-002, FR-005 through FR-007, FR-018, NFR-001, NFR-005
Parallel: No
Conflicts with: T042 and generated Admin identity/template files

Goal: provide a separately versioned generic OIDC relying-party component and
an explicitly selected redirect-based Admin flow using revocable local sessions.

Allowed files: new `components/oidc/**`; required PostgreSQL identity/session
extension files; OIDC-facing root contracts only when approved by T042; Starter
CLI/templates/tests; Admin authentication UI/tests; release/module automation;
OIDC docs; T043 packet/evidence.

Test targets: discovery and real provider login; PKCE/state/nonce/replay;
issuer/audience/expiry/redirect/size/duplicate rejection; stable issuer+subject;
claim mapping; session restart/revocation/logout; unselected graph/source absence.

Deliverables: OIDC module, selected Admin composition and UI, pinned dependencies,
real-provider fixture, security documentation, and evidence.

Acceptance criteria: a copied-out OIDC Admin completes the browser flow and
multi-scope authorization; hostile cases fail closed; local Admin is unchanged;
OIDC dependencies are absent unless selected.

Definition of Done: module tests, adversarial tests, real provider acceptance,
browser acceptance, vulnerability scan, copied-out selection/absence, docs,
evidence, and review pass.

Validation commands: OIDC module `go test -race ./...`; selected Starter tests;
real IdP acceptance; Admin frontend lint/type/test/build; dependency graph checks;
strict T043 artifact validation; `git diff --check`.

TDD plan: RED protocol and selection tests; GREEN minimal maintained-library
integration; REFACTOR bounded flow/session and frontend composition.

Packet path: `.ai-platform/specs/010-production-foundation/packets/T043.yaml`

Evidence required: `.ai-platform/evidence/T043/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, and browser/provider artifacts.

### T044: Process Runtime, Probes, Drain, And Migrations

Status: Completed
Priority: P0
Depends on: T042
Blocks: T045, T046, T047, T048
Story / Requirement: US-003, US-004, FR-008 through FR-011, FR-013, NFR-002, NFR-003, NFR-006
Parallel: No
Conflicts with: generated process templates and HTTP health composition

Goal: establish one standard-library process contract for build identity,
liveness, readiness checks, pre-drain transition, graceful shutdown, structured
logs, and explicit migration-only execution.

Allowed files: new process/health packages in the root module; `appkit/**`,
`module/**`, database component migration entry points, `transport/httpapi/**`,
all process templates/tests, operational docs, T044 packet/evidence.

Test targets: probe method/body/query/output; readiness dependency failure and
timeout; concurrent transition; signal/drain/active request; migration-only and
serve-without-migrate; structured lifecycle diagnostics and secret redaction.

Deliverables: runtime APIs, generated commands, probes, migration policy,
shutdown behavior, docs, and evidence.

Acceptance criteria: `/livez` remains local; `/readyz` reflects startup,
dependencies, and draining; no ambiguous `/healthz`; migrations run independently;
all Profiles share one termination contract.

Definition of Done: unit/integration/race/repeat tests, active-request process
acceptance, PostgreSQL migration tests, copied Profiles, docs, evidence, review.

Validation commands: focused root/PostgreSQL tests; process conformance tests;
`make copied-profile-acceptance`; `make race`; strict T044 artifact validation;
`git diff --check`.

TDD plan: RED process and migration behavior tests; GREEN runtime package and
templates; REFACTOR shared bounded lifecycle logic.

Packet path: `.ai-platform/specs/010-production-foundation/packets/T044.yaml`

Evidence required: `.ai-platform/evidence/T044/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

### T045: Consumer-Owned OCI Deployment Baseline

Status: Completed
Priority: P0
Depends on: T044
Blocks: T047, T048
Story / Requirement: US-004, FR-012, NFR-002, NFR-004, NFR-005
Parallel: No
Conflicts with: Starter template registry and copied-profile acceptance

Goal: generate a secure, reproducible, non-root OCI build and local PostgreSQL
Compose source that exercises the approved process and migration contracts.

Allowed files: Starter deployment templates/registry/tests; container acceptance
scripts and CI; deployment/security/backup docs; T045 packet/evidence.

Test targets: deterministic files, image contents/user/platform metadata,
embedded Admin assets, probe behavior, migrate-then-serve, PostgreSQL connection,
SIGTERM drain, secret/source/VCS/cache absence, and no Node runtime.

Deliverables: Dockerfile, `.dockerignore`, database Compose, optional Kubernetes
reference documentation, container test automation, and evidence.

Acceptance criteria: copied-out Profile images build; database Profiles migrate
and serve; processes run non-root and drain; platform-specific machinery remains
outside Core.

Definition of Done: source tests, Docker/OCI acceptance where available, hosted
Linux gate, copied-out validation, docs, evidence, and review pass.

Validation commands: Starter tests; container acceptance script; copied-profile
acceptance; cross-build; docs; strict T045 artifact validation; `git diff --check`.

TDD plan: RED generated-file and container assertions; GREEN minimal artifacts;
REFACTOR common templates without hiding Profile topology.

Packet path: `.ai-platform/specs/010-production-foundation/packets/T045.yaml`

Evidence required: `.ai-platform/evidence/T045/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

### T046: Structured Operations And Optional OpenTelemetry

Status: Completed
Priority: P0
Depends on: T044
Blocks: T047, T048
Story / Requirement: US-005, FR-013 through FR-017, NFR-003, NFR-005, NFR-006
Parallel: No
Conflicts with: process/HTTP templates, module release automation, and dependency gates

Goal: provide correlated secret-safe structured logs and a separately versioned,
explicitly selected OTLP traces/metrics component with bounded attributes.

Allowed files: root standard-library logging/correlation packages; new
`components/otel/**`; selected HTTP/PostgreSQL/River instrumentation hooks;
Starter selection/templates/tests; telemetry CI/fixtures/docs; T046 evidence.

Test targets: JSON diagnostics and redaction; propagation; stable route templates;
status/duration/in-flight metrics; action/database/task signals; cardinality
rejection; exporter shutdown/failure; real Collector export; selection absence.

Deliverables: slog contract, OTel module, explicit generated composition, OTLP
acceptance, operator docs, and evidence.

Acceptance criteria: selected consumer exports correlated spans/metrics; disabled
consumer has no OTel dependencies or lifecycle; no secret or high-cardinality
attribute is emitted; exporter shutdown is bounded.

Definition of Done: module tests, race, Collector integration, copied-out
selection/absence, vulnerability checks, docs, evidence, and review pass.

Validation commands: root logging tests; OTel module `go test -race ./...`; real
Collector acceptance; selected Starter and graph tests; strict T046 validation;
`git diff --check`.

TDD plan: RED logging/export/absence tests; GREEN standard library and OTel
component; REFACTOR shared semantic names and explicit lifecycle.

Packet path: `.ai-platform/specs/010-production-foundation/packets/T046.yaml`

Evidence required: `.ai-platform/evidence/T046/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

### T047: External Production Acceptance And Documentation

Status: Completed
Priority: P0
Depends on: T043, T045, T046
Blocks: T048
Story / Requirement: US-006, FR-017, FR-018, all NFRs, SC-001 through SC-006
Parallel: No
Conflicts with: all source and canonical acceptance artifacts

Goal: prove the complete v0.3 boundary from copied-out applications, disposable
infrastructure, failure injection, browser behavior, docs, and current review.

Allowed files: acceptance scripts/tests/CI; docs and Chinese onboarding;
release automation; canonical v0.3 reports; T047 packet/evidence; narrow fixes
required by review findings with evidence refresh.

Test targets: all selected/unselected consumers, real PostgreSQL/IdP/Collector,
OCI, failure/restart/drain, Admin browser, module graphs, remote-ready release
fixtures, complete repository quality and source digest.

Deliverables: external acceptance, bilingual production guides, upgrade guide,
known limitations/support matrix, final digest, and four-pass review.

Acceptance criteria: every success criterion passes with no skipped required
integration and no unresolved P0 through P2 finding.

Definition of Done: `make ci`, release-readiness candidate, strict artifacts,
source-digest stability, copied-out tests, browser/container evidence, and review.

Validation commands: `make ci`; `make release-readiness VERSION=v0.3.0-alpha.1`;
strict T047 artifact validation; `git diff --check`.

TDD plan: RED missing external/absence/failure assertions; GREEN acceptance and
fixes; REFACTOR deterministic gates and canonical docs.

Packet path: `.ai-platform/specs/010-production-foundation/packets/T047.yaml`

Evidence required: `.ai-platform/evidence/T047/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, browser/container/provider/collector artifacts.

### T048: v0.3 Coordinated Release And Remote Verification

Status: Pending
Priority: P0
Depends on: T047
Blocks: None
Story / Requirement: US-006, NFR-007, NFR-008, SC-006
Parallel: No
Conflicts with: tags, release identity, canonical reports, and main branch publication

Goal: publish the accepted source through one coordinated five-module immutable
tag train and verify hosted and local remote consumption.

Allowed files: release/version/docs/automation and T048 evidence; Git refs,
GitHub Actions, and GitHub prerelease only after clean candidate approval gates.

Test targets: clean worktree, canonical origin, five module versions/tags,
hosted main/tag CI, normal Go proxy resolution, copied-out remote consumers,
release metadata, and immutable tag objects.

Deliverables: candidate commit, annotated tags, hosted CI, remote verification,
GitHub prerelease, final release report, and evidence.

Acceptance criteria: all tags peel to one accepted commit; all five modules
resolve at `v0.3.0-alpha.1` without replacement; GitHub prerelease and final
record are published; no tag moves.

Definition of Done: release and remote gates pass, final record commit is pushed,
hosted CI passes, worktree is clean, and goal is marked complete.

Validation commands: `make release-readiness VERSION=v0.3.0-alpha.1`; tag-mode
preflight; hosted CI; `make remote-consumer VERSION=v0.3.0-alpha.1`; release view;
strict T048 artifact validation; `git status --short`.

TDD plan: release-fixture tests provide RED/GREEN behavior before live refs;
publication follows immutable stop conditions and has no destructive retry.

Packet path: `.ai-platform/specs/010-production-foundation/packets/T048.yaml`

Evidence required: `.ai-platform/evidence/T048/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, release notes, tag objects, CI and release URLs.
