# Modary Alpha Release Readiness Work Graph

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-07-31
- Approval source: user-approved release analysis and explicit execution request

## T017: Release Governance And Claims

Status: Completed
Priority: P0
Depends on: T016
Blocks: T018, T019, T020
Story / Requirement: FR-003, FR-004, FR-009, FR-010; NFR-005, NFR-006
Parallel: No
Conflicts with: T018, T019, T020

Goal: Establish canonical Alpha versioning, compatibility, security, ownership,
release-state, and non-claim policies.

Allowed files:
- `.ai-platform/specs/003-release-readiness/**`
- `.ai-platform/docs/**`
- `.ai-platform/evidence/T017/**`
- `CHANGELOG.md`
- `CONTRIBUTING.md`
- `SECURITY.md`
- `docs/releases/**`
- `scripts/check-docs.sh`
- `scripts/check-doc-links.sh`
- `scripts/check_doc_links_test.go`
- `scripts/check_docs_test.go`
- `Makefile`

Test targets:
- `scripts/check_docs_test.go`
- governance artifact validator

Deliverables: Confirmed release specification, plan, work graph, checklist,
packets, canonical policy documents, and T017 evidence.

Acceptance criteria: Policies distinguish Alpha compatibility, owner decisions,
engineering readiness, distribution, and actual release without overclaiming.

Definition of Done: Documentation checks, artifact validation, and
`git diff --check` pass.

Validation commands:
- `make docs-check`
- `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 003-release-readiness --task-id T017 --strict`
- `git diff --check`

TDD plan: Documentation-only exception; validate required structure and claims
directly, then preserve green checks.

Packet path: `.ai-platform/specs/003-release-readiness/packets/T017.yaml`

Evidence required: Changed files, validation results, claim review, residual
risk.

## T018: Adoption And Operations Documentation

Status: Completed
Priority: P0
Depends on: T017
Blocks: T020
Story / Requirement: FR-001, FR-002, FR-005; NFR-001, NFR-002
Parallel: No
Conflicts with: T017, T019, T020

Goal: Deliver a detailed, navigable documentation system for consumers,
operators, and security reviewers.

Allowed files:
- `README.md`
- `docs/index.md`
- `docs/getting-started/**`
- `docs/concepts/**`
- `docs/how-to/**`
- `docs/reference/**`
- `docs/operations/**`
- `docs/releases/**`
- `scripts/check-docs.sh`
- `scripts/check_docs_test.go`
- `.ai-platform/evidence/T018/**`
- `.ai-platform/specs/003-release-readiness/tasks.md`

Test targets:
- `scripts/check_docs_test.go`
- `testdata/external-consumer/**`

Deliverables: Documentation index, tutorials, concepts, how-to guides,
reference, operations guides, link checks, and T018 evidence.

Acceptance criteria: A new consumer can follow the supported path and every F0
deployment boundary is discoverable without reading governance history.

Definition of Done: Focused docs tests, `make docs-check`, consumer verify and
generate checks, and `git diff --check` pass.

Validation commands:
- `go test ./scripts -run 'TestDocs' -count=1`
- `make docs-check verify check-generated`
- `git diff --check`

TDD plan: RED missing-file/link fixture coverage; GREEN documentation and
checker; REFACTOR navigation after green.

Packet path: `.ai-platform/specs/003-release-readiness/packets/T018.yaml`

Evidence required: RED/GREEN results, changed files, navigation audit, command
validation, residual risk.

## T019: Release And Remote Consumer Gates

Status: Completed
Priority: P0
Depends on: T017
Blocks: T020
Story / Requirement: FR-006, FR-007, FR-008; NFR-003, NFR-004
Parallel: No
Conflicts with: T017, T018, T020

Goal: Implement deterministic release preflight, remote-consumer conformance,
tag CI wiring, and focused failure-path tests.

Allowed files:
- `Makefile`
- `.github/workflows/ci.yml`
- `scripts/release-preflight.sh`
- `scripts/remote-consumer.sh`
- `scripts/release_scripts_test.go`
- `scripts/makefile_test.go`
- `testdata/external-consumer/README.md`
- `docs/releases/**`
- `.ai-platform/evidence/T019/**`
- `.ai-platform/specs/003-release-readiness/tasks.md`

Test targets:
- `scripts/release_scripts_test.go`
- `scripts/makefile_test.go`

Deliverables: Release scripts, Make targets, tag CI gate, tests, operator docs,
and T019 evidence.

Acceptance criteria: Invalid or incomplete releases fail closed; a published
version can be tested outside the repository with no local replacement.

Definition of Done: Focused RED/GREEN tests, script syntax checks, Make tests,
and existing acceptance gates pass.

Validation commands:
- `go test ./scripts -run 'TestRelease|TestRemote|TestMake' -count=1`
- `sh -n scripts/release-preflight.sh scripts/remote-consumer.sh`
- `make acceptance`
- `git diff --check`

TDD plan: RED fixture tests for each missing or mismatched input; GREEN minimal
fail-closed scripts; REFACTOR shared test helpers only after green.

Packet path: `.ai-platform/specs/003-release-readiness/packets/T019.yaml`

Evidence required: RED/GREEN results, command matrix, changed files, security
review, residual risk.

## T020: Full Release Readiness Acceptance

Status: Completed
Priority: P0
Depends on: T018, T019
Blocks: None
Story / Requirement: FR-001 through FR-010; NFR-001 through NFR-006
Parallel: No
Conflicts with: All active release-readiness tasks

Goal: Complete full validation and review, report the exact readiness state,
and reduce owner/external blockers to explicit publish decisions.

Allowed files:
- repository-wide reviewed fixes
- `.ai-platform/specs/003-release-readiness/**`
- `.ai-platform/evidence/T020/**`
- `.ai-platform/docs/**`
- `docs/**`
- root project and release documents

Test targets:
- all framework, external-consumer, release, documentation, and governance gates

Deliverables: Final evidence, review findings, release-readiness matrix,
canonical release report, and owner decision handoff.

Acceptance criteria: No unresolved P0-P2 engineering finding; actual
distribution remains unclaimed until license, remote, and tag evidence exist.

Definition of Done: `make acceptance`, `make ci`, strict T017-T020 artifact
validation, and `git diff --check` pass after the last implementation change.

Validation commands:
- `make acceptance`
- `make ci`
- `for task in T017 T018 T019 T020; do python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 003-release-readiness --task-id "$task" --strict; done`
- `git diff --check`

TDD plan: RED any review or gate finding; GREEN focused repairs; REFACTOR only
after green; rerun full closure gates.

Packet path: `.ai-platform/specs/003-release-readiness/packets/T020.yaml`

Evidence required: Changed files, validation results, spec and engineering
review, readiness matrix, owner blockers, residual risk.
