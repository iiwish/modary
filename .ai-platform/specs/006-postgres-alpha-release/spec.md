# PostgreSQL Alpha 3 Publication Specification

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-08-02
- Approval source: owner approval to publish Modary on 2026-08-02

## Goal

Publish the T026-accepted PostgreSQL and River source as the immutable
`v0.1.0-alpha.3` Go module release without expanding the framework contract.

## Requirements

- FR-001: Use `v0.1.0-alpha.3`; never move or reuse an earlier tag.
- FR-002: The candidate is one clean commit containing the accepted source,
  exact changelog entry, Apache-2.0 licensing, current security channel,
  canonical documentation, and independent Counter consumer.
- FR-003: Candidate preflight and the complete local CI gate pass before push.
- FR-004: The candidate commit is pushed to canonical `origin/main`, and its
  hosted main workflow succeeds before the release tag is pushed.
- FR-005: One annotated Alpha 3 tag points at the candidate commit, passes tag
  preflight, and is pushed without force.
- FR-006: Hosted tag CI, normal Go module resolution, and the copied-out remote
  consumer pass before a supported GitHub prerelease is created.
- FR-007: Final reports and T027 evidence record exact immutable identities and
  do not claim product, binary, container, stable-v1, or production-IAM scope.

## Success Criteria

- SC-001: candidate and tag preflight pass.
- SC-002: local complete CI and hosted main CI pass.
- SC-003: the annotated tag and candidate commit have an exact relationship.
- SC-004: hosted tag CI passes against PostgreSQL 17.
- SC-005: the Go module and copied-out consumer resolve Alpha 3 without a local
  replacement or Go work file.
- SC-006: the GitHub prerelease, final report, and evidence identify the same
  version and commit with no unresolved P0, P1, or P2 release finding.

## Exclusions

This task does not publish a downstream consumer product, Modary binary,
container, hosted UI, starter product, stable compatibility promise, or a new
framework capability.
