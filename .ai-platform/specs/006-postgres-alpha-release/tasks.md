# PostgreSQL Alpha 3 Publication Work Graph

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-08-02

## T027: Publish PostgreSQL Alpha 3

Status: Completed
Priority: P0
Depends on: T026
Blocks: Remote consumption of the PostgreSQL and River release
Story / Requirement: FR-001 through FR-007 and SC-001 through SC-006
Parallel: No
Conflicts with: Every release, tag, and candidate-state task

Goal: Publish the accepted source as immutable `v0.1.0-alpha.3` and prove normal
remote consumption.

Allowed files: release metadata, canonical documentation, example dependency
metadata, 006 governance, T027 evidence, Git commits, the Alpha 3 annotated tag,
the canonical remote, and the matching GitHub prerelease.

Test targets: candidate and tag preflight, complete CI, hosted main and tag
workflows, Go module resolution, copied-out remote consumer, release identity,
and final documentation consistency.

Deliverables: candidate commit, annotated tag, hosted workflow results, module
resolution, remote consumer result, GitHub prerelease, and final evidence.

Acceptance criteria: every release object identifies one commit, all local and
hosted gates pass, and no earlier tag is moved.

Definition of Done: Alpha 3 is released and remotely verified, final evidence
records exact identities and URLs, and strict T027 validation passes.

Validation commands:
- `make release-readiness VERSION=v0.1.0-alpha.3`
- `make release-preflight VERSION=v0.1.0-alpha.3 RELEASE_MODE=tag`
- hosted main and tag workflow checks
- `go list -m github.com/iiwish/modary@v0.1.0-alpha.3`
- `make remote-consumer VERSION=v0.1.0-alpha.3`
- strict T027 artifact validation

TDD plan: release exception; validate every reversible state before the next
irreversible transition, stop at the first failed gate, and never move a tag.

Packet path: `.ai-platform/specs/006-postgres-alpha-release/packets/T027.yaml`

Evidence required: `.ai-platform/evidence/T027/summary.md`, `diff.patch`,
`test-results.md`, `review.md`, and `release-notes.md`.
