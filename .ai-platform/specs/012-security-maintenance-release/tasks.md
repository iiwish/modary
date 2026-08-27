# Security Maintenance Release Work Graph

- Version: 1.0
- Status: Confirmed
- Date: 2026-08-27
- Target: `v0.3.0-alpha.2`
- Approval source: explicit owner request to complete the release

## Graph

`T049 -> T050`

## T049: Go Security Baseline And Release Candidate

Status: Completed
Priority: P0
Depends on: T048
Blocks: T050
Story / Requirement: US-001, US-002, US-003, FR-001 through FR-006, NFR-001 through NFR-003, NFR-006, SC-001 through SC-003
Parallel: No
Conflicts with: all release source, version, documentation, and evidence changes

Goal: prepare one clean Alpha 2 candidate using Go 1.26.7 with no runtime or
product-scope expansion and zero reachable vulnerability findings.

Allowed files: Go/workspace/module files; Starter templates and focused tests;
release scripts and tests; current English/Chinese documentation; changelog;
canonical release and task state; feature 011 version baseline; feature 012
artifacts; T049 evidence.

Test targets: release and Starter RED/GREEN assertions; all Go modules;
copied-out Profiles; real PostgreSQL; vulnerability scans; docs; Provider and
container acceptance; candidate release readiness under Go 1.26.7.

Deliverables: exact version and baseline changes, current canonical docs, clean
candidate commit, T049 evidence, and no unresolved P0 through P2 finding.

Acceptance criteria: Go 1.26.7 and Alpha 2 are consistent across the complete
candidate, all local release gates pass, and vulnerability scanning reports zero
reachable findings.

Validation commands: focused RED/GREEN tests; `make acceptance`, `make race`,
`make ci`, `make release-readiness VERSION=v0.3.0-alpha.2`, strict T049 artifact
validation, source review, and `git diff --check`, all with Go 1.26.7.

Definition of Done: one committed clean candidate passes every local candidate
gate, has zero reachable vulnerabilities, and has no unresolved P0 through P2
review finding.

TDD plan: focused assertions reject the old baseline and Starter version before
the complete source is updated; complete gates provide GREEN evidence.

Packet path: `.ai-platform/specs/012-security-maintenance-release/packets/T049.yaml`

Evidence required: `.ai-platform/evidence/T049/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

## T050: Coordinated Publication And Remote Verification

Status: Pending
Priority: P0
Depends on: T049
Blocks: None
Story / Requirement: FR-007, FR-008, NFR-004, NFR-005, SC-004 through SC-006
Parallel: No
Conflicts with: all source changes, tags, releases, and main publication

Goal: publish the accepted Alpha 2 candidate through one immutable five-tag
train and prove hosted and local remote consumption.

Allowed files: release evidence and canonical release reports after the clean
candidate; Git refs, GitHub Actions, and GitHub prerelease after all stop
conditions pass.

Test targets: tag-mode preflight, hosted main and tag CI, replacement-free
remote consumers, released-source containers, five module proxy queries, and
GitHub prerelease metadata.

Deliverables: five aligned annotated tags, GitHub prerelease, hosted CI proof,
remote-consumption proof, final canonical release records, and clean worktree.

Acceptance criteria: every public ref, release surface, and remote consumer
resolves to the accepted candidate with no unresolved P0 through P2 finding.

Validation commands: tag-mode preflight, hosted tag CI, remote consumer,
released container acceptance, five remote module queries, GitHub release view,
strict T050 artifact validation, clean worktree, and hosted final `main` CI.

Definition of Done: all tags peel to one candidate commit; GitHub prerelease,
remote module resolution, released consumers, final record, `origin/main`, and
the clean local worktree agree; the delivery goal is complete.

TDD plan: tag-mode preflight first rejects missing release refs; GREEN creates
one coordinated immutable tag train and proves each remote surface; REFACTOR
records publication without changing tagged source.

Packet path: `.ai-platform/specs/012-security-maintenance-release/packets/T050.yaml`

Evidence required: `.ai-platform/evidence/T050/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, `release-notes.md`, `tags.md`, and `ci.md`.
