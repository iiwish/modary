# Current Delivery Work Graph

- Version: 6.2
- Status: Confirmed
- Last updated: 2026-08-02

The active PostgreSQL-first work graph is
`.ai-platform/specs/005-postgres-task-runtime/tasks.md`.

| Task | State | Acceptance object |
|---|---|---|
| T024 | Completed | PostgreSQL control adapter, governed transaction binding, and public task contract |
| T025 | Completed | PostgreSQL-native Action, Identity, RBAC, Audit, and migration persistence |
| T026 | Completed | Independent consumer, deletion audit, documentation, full gates, and two-pass review |
| T027 | Completed | Immutable Alpha 3 publication, hosted CI, remote consumption, and release evidence |

## T026: Consumer, Deletion, And Acceptance

Status: Completed
Priority: P0
Dependencies: T025
Blocks: Modary F0 acceptance
Story / Requirement: FR-009, FR-010, SC-001, SC-002, SC-003, SC-004, and SC-005
Parallel: No
Conflicts with: None

Goal: Prove Modary is a PostgreSQL-first framework with a public transactional
task runtime, an independently consumable Counter application, no active
embedded-database surface, and complete acceptance evidence.

Allowed files: active Modary source, tests, documentation, 005 governance
artifacts, and T026 evidence; immutable historical evidence is excluded.

Test targets: framework packages, PostgreSQL adapters, copied-out Counter with
`GOWORK=off`, task restart consumption, documentation, neutrality, builds,
race detection, repetition, fuzzing, and source stability.

Deliverables: PostgreSQL-only framework and Counter consumer, public task
runtime acceptance coverage, current documentation, release report, T026
packet, test results, diff manifest, and two independent final review records.

Acceptance criteria: `make acceptance`, `make ci`, the strict T026 delivery
artifact validator, and `git diff --check` pass; current final reviews contain
no unresolved P0, P1, or P2 finding.

Definition of Done: no active embedded-database surface remains, Counter proves
that a governed Action enqueue survives producer shutdown and is consumed after
an application restart through the public Runner, and all acceptance evidence
describes the reviewed source.

Validation commands:
- `make acceptance`
- `make ci`
- `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 005-postgres-task-runtime --task-id T026 --strict`
- `git diff --check`

TDD plan: RED records the legacy-storage and consumer-boundary failures; GREEN
requires PostgreSQL-only Counter and framework acceptance to pass; REFACTOR
requires the complete CI, race, stability, and strict artifact gates to pass.

Packet path: `.ai-platform/specs/005-postgres-task-runtime/packets/T026.yaml`

Evidence required: `.ai-platform/evidence/T026/summary.md`, `diff.patch`,
`test-results.md`, and the current final review records.

## T027: Publish PostgreSQL Alpha 3

Status: Completed
Priority: P0
Dependencies: T026
Blocks: Remote consumption of the PostgreSQL and River release
Story / Requirement: 006 FR-001 through FR-007 and SC-001 through SC-006
Parallel: No
Conflicts with: Every release, tag, and candidate-state task

Goal: Publish the accepted Modary source as immutable `v0.1.0-alpha.3`, prove
hosted and normal Go module consumption, and record truthful release evidence.

Allowed files: release metadata, canonical documentation, example dependency
metadata, 006 governance, T027 evidence, Git commits, the Alpha 3 annotated tag,
the canonical remote, and the matching GitHub prerelease.

Test targets: candidate and tag preflight, complete CI, hosted main and tag
workflows, Go module resolution, copied-out remote consumer, release identity,
and final documentation consistency.

Deliverables: one candidate commit, immutable annotated tag, successful hosted
CI, verified remote consumer, GitHub prerelease, and complete T027 evidence.

Acceptance criteria: every release object identifies one commit; local and
hosted gates pass; normal module resolution and the copied-out consumer use
`v0.1.0-alpha.3`; no earlier tag is moved.

Definition of Done: Alpha 3 is released and remotely verified, final reports
and evidence contain the exact commit, tag object, workflow, resolution, and
release URL, and strict T027 artifact validation passes.

Validation commands:
- `make release-readiness VERSION=v0.1.0-alpha.3`
- `make release-preflight VERSION=v0.1.0-alpha.3 RELEASE_MODE=tag`
- hosted main and tag workflow checks
- `go list -m github.com/iiwish/modary@v0.1.0-alpha.3`
- `make remote-consumer VERSION=v0.1.0-alpha.3`
- strict T027 artifact validation

TDD plan: release exception; validate every reversible candidate state before
the next irreversible transition, stop on any failed gate, and never move a
published tag.

Packet path: `.ai-platform/specs/006-postgres-alpha-release/packets/T027.yaml`

Evidence required: candidate and tag identity, local and hosted validation,
module resolution, remote consumer output, release URL, review, and residual
risk under `.ai-platform/evidence/T027/`.

Completed framework, release-readiness, and onboarding work remains available
as historical governance under `.ai-platform/specs/002-framework-decoupling/`,
`.ai-platform/specs/003-release-readiness/`, and
`.ai-platform/specs/004-onboarding-release/`. Its evidence describes the source
accepted at that time and does not override the current 005 contract.

Task completion requires its implementation, focused and full validation,
evidence packet, and review contract. Current framework acceptance requires no
unresolved P0, P1, or P2 finding in T026.
