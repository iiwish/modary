# Current Delivery State

- Version: 11.0
- Status: Confirmed
- Last updated: 2026-08-10
- Active implementation work graph: None
- Latest completed work graph: `.ai-platform/specs/010-production-foundation/tasks.md`
- Proposed next contract: `.ai-platform/specs/011-design-partner-validation/spec.md`

## Current Gate

The v0.3 Production Foundation work graph is closed. T042 through T048 are
completed, `v0.3.0-alpha.1` is released, and remote verification is recorded in
`.ai-platform/docs/release-report.md`.

The Design Partner Validation specification is `Ready_For_User_Review`. It is
not an active work graph and grants no planning or execution authority. No new
task ID is reserved until the owner confirms that requirements contract. After
confirmation, checklist, technical plan, work graph, analysis, and execution
packets remain separate approval-gated artifacts.

## Delivery Ledger

| Task | State | Acceptance object |
|---|---|---|
| T042 | Completed | Scope-independent principal and replaceable session contracts |
| T043 | Completed | Production OIDC component and selected Admin flow |
| T044 | Completed | Process health, readiness drain, build identity, and migration command |
| T045 | Completed | Consumer-owned OCI and local Compose deployment baseline |
| T046 | Completed | Structured logging and optional OpenTelemetry integration |
| T047 | Completed | Production copied-out, failure, security, and documentation acceptance |
| T048 | Completed | Coordinated v0.3 release and remote verification |
| T038 | Completed | PostgreSQL and River module-graph isolation |
| T039 | Completed | Explicit pre-start HTTP and Admin contribution contracts |
| T040 | Completed | Permission-aware shared Admin primitives and task/audit UI |
| T041 | Completed | External boundary acceptance and v0.2 engineering readiness |
| T035 | Completed | React-only Admin platform, typed application architecture, tests, and deterministic assets |
| T036 | Completed | React Admin behavior, accessibility, responsive quality, and browser acceptance |
| T037 | Completed | Copied-out React consumer, canonical docs, and final v0.2 release readiness |
| T028 | Completed | Lightweight product contract, competitor research, Profiles, and acceptance boundary |
| T029 | Completed | Database-free Core and optional component surfaces |
| T030 | Completed | Create-only CLI and API Profile |
| T031 | Completed | Optional Admin backend Profile |
| T032 | Completed | Superseded Admin UI F0 implementation and frontend module registry |
| T033 | Completed | Optional Governed Profile with retained Alpha 3 guarantees |
| T034 | Completed | Copied-out Profile acceptance, docs, compatibility, and final reviews |
| T024 | Completed | PostgreSQL control adapter and public task contract |
| T025 | Completed | PostgreSQL-native standard persistence |
| T026 | Completed | Alpha 3 framework and copied-out consumer acceptance |
| T027 | Completed | Immutable Alpha 3 release and remote verification |

Feature-scoped specifications, plans, work graphs, packets, and evidence under
`.ai-platform/specs/002-framework-decoupling/` through
`.ai-platform/specs/010-production-foundation/` remain the canonical history for
their accepted delivery slices.

## T048: v0.3 Coordinated Release And Remote Verification

Status: Completed
Priority: P0
Dependencies: T047
Blocks: None
Story / Requirement: US-006, NFR-007, NFR-008, SC-006
Parallel: No
Conflicts with: tags, release identity, canonical reports, and main branch publication

Goal: publish the accepted Production Foundation source through one coordinated
five-module immutable tag train and verify hosted and local remote consumption.

Allowed files: release/version/docs/automation and T048 evidence; Git refs,
GitHub Actions, and GitHub prerelease only after clean candidate approval gates.

Test targets: clean worktree, canonical origin, five module versions and tags,
hosted main and tag CI, normal Go proxy resolution, copied-out remote consumers,
release metadata, and immutable tag objects.

Deliverables: candidate commit, annotated tags, hosted CI, remote verification,
GitHub prerelease, final release report, and evidence.

Acceptance criteria: all tags peel to one accepted commit; all five modules
resolve at `v0.3.0-alpha.1` without replacement; GitHub prerelease and final
record are published; no tag moves.

Definition of Done: release and remote gates pass, the final record commit is
pushed, hosted CI passes, and the worktree is clean.

Validation commands:
- `make release-readiness VERSION=v0.3.0-alpha.1`
- `make remote-consumer VERSION=v0.3.0-alpha.1`
- `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 010-production-foundation --task-id T048 --strict`
- `git status --short`

TDD plan: release-fixture tests provide RED/GREEN behavior before live refs;
publication follows immutable stop conditions and has no destructive retry.

Packet path: `.ai-platform/specs/010-production-foundation/packets/T048.yaml`

Evidence required: `.ai-platform/evidence/T048/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, release notes, tag objects, CI, and release URLs.
