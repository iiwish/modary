# Modary v0.3 Alpha 1 Readiness Report

- Report version: 4.0
- Status: Candidate_accepted
- Technical F0 acceptance: Accepted
- Engineering readiness: Accepted
- Onboarding readiness: Accepted for local, OIDC, telemetry, API, Admin, and Governed consumers
- Current source version: v0.3.0-alpha.1
- Target version: v0.3.0-alpha.1
- Distribution status: Prepared
- Version tags: v0.3.0-alpha.1, components/postgres/v0.3.0-alpha.1, components/governedpostgres/v0.3.0-alpha.1, components/oidc/v0.3.0-alpha.1, components/otel/v0.3.0-alpha.1
- Remote consumer verification: Pending
- Latest supported release: v0.3.0-alpha.1
- Release: https://github.com/iiwish/modary/releases/tag/v0.3.0-alpha.1
- Published modules: root, components/postgres, components/governedpostgres, components/oidc, components/otel
- Canonical remote: https://github.com/iiwish/modary
- Owner-selected redistribution license: Apache-2.0
- Private security reporting channel: https://github.com/iiwish/modary/security/advisories/new
- Last updated: 2026-08-04

## Scope

The release is the Production Foundation for the lightweight componentized Go
framework. It adds scope-independent identity, an optional OIDC relying party,
deterministic process probes and drain, migration-only execution, consumer-owned
OCI and Compose source, structured diagnostics, and optional OTLP traces and
metrics. Core remains database-free and heavy dependencies remain explicit.

## Acceptance

T042 through T047 prove protocol hostility, PostgreSQL persistence, multi-scope
authorization, selected and absent dependency graphs, frontend variants,
process failure and shutdown, collector failure, non-root containers, copied-out
consumers, platform builds, vulnerabilities, documentation, and candidate
source integrity with no unresolved P0 through P2 finding.

## Release Boundary

Alpha 1 is one immutable commit and five annotated tags. Normal Go module
resolution must return the exact version without a replacement for every
module. Hosted main and tag CI, local and hosted remote consumers, and the
GitHub prerelease identify the same commit. Modary publishes source and
documentation, not a hosted product, container registry, database service,
identity provider, collector, or stable-v1 promise.
