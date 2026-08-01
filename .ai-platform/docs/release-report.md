# Modary Alpha Release Readiness Report

- Report version: 1.0
- Status: Accepted
- Technical F0 acceptance: Accepted
- Engineering readiness: Accepted
- Onboarding readiness: Accepted
- Target version: v0.1.0-alpha.2
- Distribution status: Released
- Version tag: v0.1.0-alpha.2
- Rejected version: v0.1.0-alpha.1 at f57e3adda9a5f0e7335e821ef0b69eaf75c3548b
- Canonical remote: https://github.com/iiwish/modary
- Owner-selected redistribution license: Apache-2.0
- Private security reporting channel: https://github.com/iiwish/modary/security/advisories/new
- Remote consumer verification: Passed
- Release URL: https://github.com/iiwish/modary/releases/tag/v0.1.0-alpha.2
- Rejected release URL: https://github.com/iiwish/modary/releases/tag/v0.1.0-alpha.1
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

The immutable `v0.1.0-alpha.1` tag resolved normally, but the copied-out remote
consumer rejected two checkout-only example assertions. No GitHub release was
created for that tag. The release policy requires the repaired candidate to use
the subsequent `v0.1.0-alpha.2` version rather than moving a cached tag.

## Release Claim

The supported release is `v0.1.0-alpha.2` at
`a4700f1c7ef53fe058a50fd43d65b906c3be89c4`. Its annotated tag, Linux quality,
Darwin arm64 native checks, tag preflight, source stability, public Go proxy
resolution, and copied-out remote consumer all passed. The GitHub prerelease,
source tag, changelog, Apache-2.0 license, security channel, and current public
documentation identify that version.

The report therefore claims `Released` and `Remote_Verified` for Alpha 2. Alpha
1 remains an immutable rejected prerelease record and is not supported.
