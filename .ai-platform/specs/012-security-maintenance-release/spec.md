# Security Maintenance Release Specification

- Version: 1.0
- Status: Confirmed
- Date: 2026-08-27
- Source: explicit owner request to complete `v0.3.0-alpha.2`
- Target release: `v0.3.0-alpha.2`
- Execution authorization: Granted

## Purpose

Publish a narrow v0.3 security maintenance release that replaces the Go 1.26.5
minimum with the current supported Go 1.26.7 patch and republishes the existing
five-module framework train without expanding the product or runtime contract.

## User Stories

- US-001: A Modary consumer can pin one coordinated `v0.3.0-alpha.2` release
  train that is built and scanned against Go 1.26.7.
- US-002: A maintainer can prove that the five reachable standard-library
  vulnerabilities reported against the Go 1.26.5 baseline are absent from the
  release candidate.
- US-003: An existing Alpha 1 consumer can upgrade without an application API,
  schema, generated-product, or behavior migration.

## Functional Requirements

- FR-001: Root, workspace, all four published component modules, integration
  fixtures, examples, generated Profile modules, and generated build images use
  Go 1.26.7 as their exact minimum patch baseline.
- FR-002: The root module and the PostgreSQL, Governed PostgreSQL, OIDC, and
  OpenTelemetry modules use `v0.3.0-alpha.2` as one coordinated version at one
  candidate commit.
- FR-003: Starter defaults and current installation examples generate or pin
  `v0.3.0-alpha.2` without local replacements in released mode.
- FR-004: Release automation rejects any Go baseline other than 1.26.7 and
  rejects missing, lightweight, mismatched, or differently targeted release
  tags.
- FR-005: Changelog, support, security, installation, framework, release, and
  bilingual onboarding documentation describe Alpha 2 as a security maintenance
  release with no framework API or migration change.
- FR-006: The pinned source vulnerability gate reports zero reachable
  vulnerabilities for the root and four published component modules.
- FR-007: Candidate, tag-mode, hosted CI, normal Go module resolution, copied-out
  remote consumer, and released container gates pass for the exact tag train.
- FR-008: A GitHub prerelease is published from the root tag with scope,
  compatibility, security baseline, limitations, and remote-verification result.

## Non-Functional Requirements

- NFR-001 Security: no known reachable vulnerability may remain in the scanned
  release source; evidence must identify the scanner and toolchain.
- NFR-002 Compatibility: no exported application API, migration, Action,
  capability, Profile selection, route, or generated product behavior changes.
- NFR-003 Reproducibility: candidate validation runs with `GOTOOLCHAIN=local`
  through the exact Go 1.26.7 binary and the pinned frontend toolchain.
- NFR-004 Integrity: all five annotated tags peel to the same clean candidate
  commit and are never moved or reused.
- NFR-005 Independence: remote verification uses `GOWORK=off` and contains no
  local `replace` directive or framework-checkout dependency.
- NFR-006 Governance: the existing Design Partner Validation contract remains
  review-only and receives no implementation authority from this release.

## Scope

- Go 1.26.7 baseline and generated Docker build baseline.
- Coordinated root and four component module version references.
- Tests, release automation, current docs, changelog, and release evidence.
- Candidate commit, five annotated tags, GitHub prerelease, hosted CI, remote
  module consumption, and released container acceptance.

## Non-Goals

- New framework components, adapters, Profiles, APIs, migrations, or UI.
- Consumer-product implementation or changes to a consumer repository.
- Go 1.27 adoption or a stable-v1 compatibility promise.
- Design Partner Validation execution.
- Dependency upgrades not required by the Go 1.26.7 baseline or a failing
  release gate.

## Success Criteria

- SC-001: Go 1.26.5 produces the recorded five reachable standard-library
  findings and the Alpha 2 candidate produces zero reachable findings under Go
  1.26.7.
- SC-002: `make acceptance`, `make race`, `make ci`, and candidate
  `make release-readiness VERSION=v0.3.0-alpha.2` pass from the candidate.
- SC-003: generated API, Admin, and Governed projects require Go 1.26.7 and pin
  Alpha 2 while preserving selected-component absence.
- SC-004: all five annotated tags peel to one candidate commit and hosted tag CI
  succeeds.
- SC-005: `make remote-consumer VERSION=v0.3.0-alpha.2` and
  `make released-container-acceptance VERSION=v0.3.0-alpha.2` pass without local
  source replacement.
- SC-006: the final release record, GitHub prerelease, remote module metadata,
  and clean `origin/main` agree on the release state.

## Approval

- Requirements: Confirmed by the owner request on 2026-08-27.
- Planning and work graph: Approved as necessary to complete the named release.
- Execution and publication: Explicitly authorized for `v0.3.0-alpha.2`.
- Tag mutation: Never authorized; any conflicting published tag stops release.
