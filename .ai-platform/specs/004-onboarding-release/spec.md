# Modary Onboarding And Alpha Publication

- Version: 1.1
- Status: Confirmed
- Source: owner request for narrow onboarding and Alpha publication; immutable-release recovery follows the approved T022 failure policy
- Last updated: 2026-08-01
- Governing constitution: `../../memory/constitution.md`

## Objective

Make the accepted Modary Alpha approachable through one public, executable
consumer example and one short golden path, then publish the exact validated
commit as the supported `v0.1.0-alpha.2` prerelease. The work does not expand the F0 framework contract
or introduce downstream product behavior.

## Functional Requirements

- FR-001: The repository is distributed under the owner-approved Apache License
  2.0 and preserves all applicable third-party license and attribution notices.
- FR-002: The independently tested Counter consumer is a stable public example
  under `examples/counter`, while remaining the copied-out local and remote
  conformance application.
- FR-003: Documentation provides one golden path that lets a reader run the
  example, inspect the composition root, modify the first Action, regenerate
  contracts, test, build, and identify the next relevant guide.
- FR-004: The README prioritizes product fit, prerequisites, the shortest
  successful command sequence, stability, and the documentation entry point.
  Deep F0 invariants remain discoverable without dominating first use.
- FR-005: Documentation and repository checks reject stale public references to
  the retired test-fixture path and verify every canonical onboarding file.
- FR-006: The canonical public remote, concrete private security-reporting
  channel, changelog date, release report, and repository metadata match the
  release candidate.
- FR-007: The final clean commit passes candidate and tag preflight, complete CI,
  annotated-tag validation, normal remote Go module resolution, copied-out
  remote consumer conformance, and GitHub prerelease publication.

## Non-Functional Requirements

- NFR-001: The public example remains an independent Go module and uses only
  supported public Modary packages.
- NFR-002: Source-checkout development may use one documented relative
  `replace`; published consumer validation removes it and resolves the exact
  tag with `GOWORK=off`.
- NFR-003: Onboarding documentation is command-oriented, copyable, explicit
  about working directories and expected results, and does not require reading
  governance artifacts or framework source.
- NFR-004: Release automation fails closed and must not claim publication,
  successful tag CI, or remote consumption without direct evidence.
- NFR-005: Existing F0 acceptance evidence remains historical and immutable.
  Current canonical docs and scripts describe the public example location.
- NFR-006: No downstream product vocabulary, domain model, schema, UI, or
  release artifact enters Modary.

## Success Criteria

- SC-001: From a tagged checkout, the documented quickstart reaches verified,
  generated, tested, built, versioned, and runnable Counter paths without Node.
- SC-002: Public documentation contains no `testdata/external-consumer` path and
  all local links resolve.
- SC-003: `make acceptance`, `make ci`, candidate/tag preflight, strict T021 and
  T023 artifact validation, and `git diff --check` pass at their documented
  release states.
- SC-004: `go list -m github.com/iiwish/modary@v0.1.0-alpha.2` resolves the tag,
  and `make remote-consumer VERSION=v0.1.0-alpha.2` passes without a replacement.
- SC-005: The public GitHub prerelease, source tag, changelog, license, security
  channel, and release report identify the same commit and version.

## Non-Goals

- Adding a project scaffolder or global Modary CLI.
- Designing a generic starter for every application type.
- Expanding or stabilizing the F0 API contract.
- Hosting a separate documentation website.
- Beginning any downstream product implementation inside this repository.

## Acceptance Boundary

T021 completes when onboarding, licensing, attribution, and local release gates
pass on a reviewable candidate. T022 preserves the rejected immutable
`v0.1.0-alpha.1` attempt. T023 completes only after the repaired annotated tag,
tag CI, remote Go module consumption, GitHub prerelease, and final truthful
evidence all exist for `v0.1.0-alpha.2`.
