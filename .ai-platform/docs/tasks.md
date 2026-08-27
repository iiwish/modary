# Current Delivery State

- Version: 12.0
- Status: Confirmed
- Last updated: 2026-08-27
- Active implementation work graph: `.ai-platform/specs/012-security-maintenance-release/tasks.md`
- Latest completed work graph: `.ai-platform/specs/010-production-foundation/tasks.md`
- Proposed next contract: `.ai-platform/specs/011-design-partner-validation/spec.md`

## Current Gate

The v0.3 Production Foundation work graph is closed. T042 through T048 are
completed and `v0.3.0-alpha.1` remains immutable. The owner has authorized the
narrow `v0.3.0-alpha.2` security maintenance release. T049 prepares the exact
Go 1.26.7 candidate; T050 owns publication and remote verification.

The Design Partner Validation specification is `Ready_For_User_Review`. It is
not an active work graph and grants no planning or execution authority. No new
task ID is reserved until the owner confirms that requirements contract. After
confirmation, checklist, technical plan, work graph, analysis, and execution
packets remain separate approval-gated artifacts.

## Delivery Ledger

| Task | State | Acceptance object |
|---|---|---|
| T049 | Completed | Go 1.26.7 security baseline and Alpha 2 release candidate |
| T050 | Pending | Coordinated Alpha 2 publication and remote verification |
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
