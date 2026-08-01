# Modary Alpha Release Readiness Report

- Report version: 1.0
- Status: Accepted
- Technical F0 acceptance: Accepted
- Engineering readiness: Accepted
- Onboarding readiness: Accepted
- Target version: v0.1.0-alpha.1
- Distribution status: Not_released
- Version tag: None
- Canonical remote: https://github.com/iiwish/modary
- Owner-selected redistribution license: Apache-2.0
- Private security reporting channel: https://github.com/iiwish/modary/security/advisories/new
- Remote consumer verification: Not_run
- Last updated: 2026-08-01

## Scope

The release object is the independent Modary Go framework: public Kernel,
AppKit, application-command and HTTP/MCP integrations, pure project tooling,
neutral official Adapters, and a public independent Counter example that also
serves as the remote consumer conformance module.

The framework repository contains no consumer application executable, domain
schema, frontend toolchain, default identity or policy, container image, or
consumer release path.

## Current Result

The public Kernel, lifecycle, AppKit, transports, project tooling, official
Adapters, and copied-out consumer implementation are present. The integrated
application tree is preserved outside the framework with independently verified
source and governance manifests and is absent from active framework paths.

T010 through T016 are complete. The frozen source state passes the Node-free Go
framework acceptance and CI gates, copied-out consumer conformance, race,
count-20, fuzz, cross-build, generated-drift, neutrality, documentation, source,
and strict governance checks. Two fresh independent T016 reviews inspect the
same frozen digest and report zero P0, P1, and P2 findings.

The canonical acceptance matrix and residual risks are recorded in
`docs/f0-acceptance-report.md` and `docs/f0-known-limitations.md`. Reproducible
command and review records are in `.ai-platform/evidence/T016/`.

Release Readiness adds a navigable consumer documentation system, installation
and quickstart, concepts, how-to guides, public package and project reference,
deployment and security guidance, SQLite recovery, versioning and upgrades,
contribution and vulnerability policy, candidate/tag preflight, a copied-out
normal Go Module remote consumer, and read-only tag CI. T017 through T020 record
the implementation, tests, review, and full-gate evidence.

Onboarding Readiness uses the independently tested Counter consumer as the
public example under `examples/counter`, adds a short run-and-change journey,
and validates Apache-2.0 licensing and aggregated third-party attribution. T021
records accepted local onboarding evidence before publication begins.

## Release Claim

The conformance module proves local independent consumption with `GOWORK=off`
and an explicit local `replace`. Remote installation is not a release claim
until `github.com/iiwish/modary` has a published version tag. Modary-owned source
and documentation use Apache-2.0, with applicable third-party licenses and
notices preserved. No tag, package publication, or distribution is claimed by
this report.

The candidate is `Distribution_Ready`: Apache-2.0, the canonical public remote,
the enabled private security reporting channel, and accepted T021 onboarding
evidence are present. It is not `Released`. The resulting clean candidate
requires annotated-tag preflight, tag CI, and remote-consumer verification
before the report can claim release.
