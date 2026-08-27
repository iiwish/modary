# Current Delivery State

- Version: 13.0
- Status: Confirmed
- Last updated: 2026-08-27
- Active implementation work graph: None
- Latest completed work graph: `.ai-platform/specs/012-security-maintenance-release/tasks.md`
- Proposed next contract: `.ai-platform/specs/011-design-partner-validation/spec.md`

## Current Gate

The v0.3 Production Foundation and Alpha 2 security maintenance work graphs are
closed. `v0.3.0-alpha.1` remains immutable, and `v0.3.0-alpha.2` is the current
released framework train. T049 records the Go 1.26.7 candidate; T050 records
coordinated publication and remote verification.

The Design Partner Validation specification is `Ready_For_User_Review`. It is
not an active work graph and grants no planning or execution authority. No new
task ID is reserved until the owner confirms that requirements contract. After
confirmation, checklist, technical plan, work graph, analysis, and execution
packets remain separate approval-gated artifacts.

## Delivery Ledger

| Task | State | Acceptance object |
|---|---|---|
| T049 | Completed | Go 1.26.7 security baseline and Alpha 2 release candidate |
| T050 | Completed | Coordinated Alpha 2 publication and remote verification |
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
`.ai-platform/specs/012-security-maintenance-release/` remain the canonical
history for their accepted delivery slices.

## T049: Go Security Baseline And Release Candidate

Status: Completed
Priority: P0
Dependencies: T048
Blocks: T050
Story / Requirement: feature 012 US-001 through US-003, FR-001 through FR-006, SC-001 through SC-003
Parallel: No
Conflicts with: all release source, version, documentation, and evidence changes

Goal: prepare one clean `v0.3.0-alpha.2` candidate using Go 1.26.7 with zero
reachable vulnerabilities and no runtime or product-scope expansion.

Allowed files: the bounded paths listed in the T049 execution packet.

Test targets: release and Starter RED/GREEN assertions, all Go modules,
copied-out Profiles, real PostgreSQL, vulnerability scans, docs, containers, and
candidate release readiness under Go 1.26.7.

Deliverables: exact version and baseline changes, current canonical docs, clean
candidate commit, T049 evidence, and no unresolved P0 through P2 finding.

Acceptance criteria: Go 1.26.7 and Alpha 2 are consistent across the complete
candidate, all local release gates pass, and vulnerability scanning reports zero
reachable findings.

Definition of Done: one committed clean candidate passes every T049 validation
and is ready for T050 publication.

Validation commands:
- `make acceptance GO=<go1.26.7>`
- `make race GO=<go1.26.7>`
- `make ci GO=<go1.26.7>`
- `make release-readiness VERSION=v0.3.0-alpha.2 GO=<go1.26.7>`
- strict T049 artifact validation and `git diff --check`

TDD plan: focused assertions reject the old baseline and Starter version before
the complete source is updated; complete gates provide GREEN evidence.

Packet path: `.ai-platform/specs/012-security-maintenance-release/packets/T049.yaml`

Evidence required: `.ai-platform/evidence/T049/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

## T050: Coordinated Publication And Remote Verification

Status: Completed
Priority: P0
Dependencies: T049
Blocks: None
Story / Requirement: feature 012 FR-007, FR-008, NFR-004, NFR-005, SC-004 through SC-006
Parallel: No
Conflicts with: all source changes, tags, releases, and main publication

Goal: publish the accepted `v0.3.0-alpha.2` candidate through one immutable
five-tag train and prove hosted and local remote consumption.

Allowed files: the bounded paths listed in the T050 execution packet.

Test targets: tag-mode preflight, hosted main and tag CI, replacement-free
remote consumers, released-source containers, five module proxy queries, and
GitHub prerelease metadata.

Deliverables: five aligned annotated tags, GitHub prerelease, hosted CI proof,
remote-consumption proof, final canonical release records, and clean worktree.

Acceptance criteria: every public ref, release surface, and remote consumer
resolves to the accepted candidate with no unresolved P0 through P2 finding.

Definition of Done: all tags peel to one candidate commit; GitHub prerelease,
remote module resolution, released consumers, final record, `origin/main`, and
the clean local worktree agree.

Validation commands:
- tag-mode release preflight and hosted tag CI
- `make remote-consumer VERSION=v0.3.0-alpha.2`
- `make released-container-acceptance VERSION=v0.3.0-alpha.2`
- five `go list -m` queries, GitHub release view, and hosted final `main` CI
- strict T050 artifact validation and `git diff --check`

TDD plan: tag-mode preflight rejects absent refs; the five-tag train and remote
gates provide GREEN evidence; the post-tag record contains no candidate source.

Packet path: `.ai-platform/specs/012-security-maintenance-release/packets/T050.yaml`

Evidence required: `.ai-platform/evidence/T050/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, `release-notes.md`, `tags.md`, and `ci.md`.
