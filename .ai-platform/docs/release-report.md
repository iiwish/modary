# Modary Framework F0 Release Report

- Report version: 1.0
- Status: Accepted
- Technical F0 acceptance: Accepted
- Distribution status: Not_released
- Version tag: None
- Owner-selected redistribution license: None
- Last updated: 2026-07-31

## Scope

The release object is the independent Modary Go framework: public Kernel,
AppKit, application-command and HTTP/MCP integrations, pure project tooling,
neutral official Adapters, and an external consumer conformance module.

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

## Release Claim

The conformance module proves local independent consumption with `GOWORK=off`
and an explicit local `replace`. Remote installation is not a release claim
until `github.com/iiwish/modary` has a published version tag. This repository has
no owner-selected redistribution license, so technical acceptance does not grant
public redistribution rights. No tag, package publication, or distribution is
claimed by this report.
