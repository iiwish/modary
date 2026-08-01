# Modary Alpha Release Readiness

- Version: 1.0
- Status: Confirmed
- Source: user-approved release analysis and explicit execution request
- Last updated: 2026-07-31
- Governing constitution: `../../memory/constitution.md`

## Objective

Prepare the accepted Modary F0 framework for a truthful, reproducible
`v0.1.0-alpha.1` source release. The release must be independently consumable as
a Go module, documented for application developers and operators, and guarded
by deterministic preflight checks. Modary remains domain-neutral; downstream
product behavior and release artifacts stay consumer-owned.

## Users

- Application developers evaluating or adopting Modary before v1.
- Framework maintainers preparing, reviewing, and publishing a release.
- Operators and security reviewers deciding whether the F0 profile fits a
  deployment.

## Functional Requirements

- FR-001: Documentation provides one navigable entry point and separates
  tutorials, concepts, how-to guides, reference, operations, release policy,
  and project governance.
- FR-002: A new consumer can install a tagged module, create an explicit
  composition root, verify and generate project artifacts, build an
  application, and understand the local-development alternative without a
  committed `replace` directive.
- FR-003: Versioning documentation defines pre-v1 compatibility, deprecation,
  generated-format, database-migration, Go-version, and security-fix policy.
- FR-004: Release documentation defines ownership, prerequisites, exact
  preflight commands, tag rules, release notes, remote-consumer verification,
  rollback, and post-release checks.
- FR-005: Security and operations documentation states the supported profile,
  trust boundaries, secret and token handling, SQLite backup/restore
  requirements, health behavior, shutdown behavior, and production checklist.
- FR-006: A deterministic release preflight validates the requested semantic
  prerelease version, module path, clean tracked release state, required
  documents, owner-selected license, origin URL, tag/commit relationship when
  applicable, dependency metadata, and complete release gates.
- FR-007: Remote-consumer conformance copies the external consumer outside the
  repository, removes its local `replace`, resolves the requested module
  version through normal Go module resolution, and runs verify, generate/check,
  test, build, and version workflows with `GOWORK=off`.
- FR-008: CI runs the release preflight for version tags and preserves the
  existing Ubuntu and native Darwin acceptance gates.
- FR-009: The changelog and release report distinguish technical F0 acceptance,
  release readiness, and actual distribution. No document may claim that an
  unpublished tag, unavailable remote module, or undecided license exists.
- FR-010: Downstream product behavior, schemas, vocabulary, and integration code
  do not enter Modary production code, canonical framework contracts, or
  release automation.

## Non-Functional Requirements

- NFR-001: Documentation examples use public APIs and are backed by the
  executable external-consumer project or focused validation.
- NFR-002: Markdown links and required canonical documents are checked without
  requiring Node.js or a network connection.
- NFR-003: Release scripts are POSIX shell, fail closed, reject ambiguous input,
  avoid mutating the source tree, and have focused fixture tests for success and
  failure paths.
- NFR-004: Remote resolution is a separate networked gate. Local CI must be able
  to validate its command contract without pretending that a remote tag exists.
- NFR-005: Owner-only decisions are explicit blockers. Automation must not
  invent a license, create a public repository, publish a tag, or push a release
  without explicit owner approval.
- NFR-006: The supported release object is the Go source module and its
  documentation. Modary does not publish a generic consumer executable,
  container, UI, or domain starter.

## Success Criteria

- SC-001: A reader can move from installation to a running external consumer
  using one documented path and can identify every supported F0 boundary.
- SC-002: `make release-readiness VERSION=v0.1.0-alpha.1` passes once an
  owner-selected license, canonical origin, and matching release ref exist.
- SC-003: `make remote-consumer VERSION=v0.1.0-alpha.1` proves consumption
  without a local replacement after the version is available from the selected
  origin or configured Go module source.
- SC-004: `make acceptance`, focused release-script tests, documentation checks,
  governance validation, and `git diff --check` pass on the final candidate.
- SC-005: Release reports list any unresolved owner or external-state blocker
  rather than converting it into an engineering success claim.

## Non-Goals

- Selecting a redistribution license on the owner's behalf.
- Creating or changing repository visibility without explicit approval.
- Publishing or pushing a tag before the owner approves the release candidate.
- Promising stable v1 compatibility, high availability, distributed
  transactions, arbitrary storage adapters, or public-internet IAM.
- Designing or implementing a downstream product MVP.

## Acceptance Boundary

Engineering readiness is complete when documentation, policies, automation,
tests, and evidence are complete. Distribution readiness additionally requires
an owner-selected license and canonical remote. Actual release additionally
requires a matching pushed tag and successful remote-consumer conformance.
