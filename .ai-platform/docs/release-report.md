# Modary v0.3 Alpha 2 Readiness Report

- Report version: 5.0
- Status: Candidate_accepted
- Technical F0 acceptance: Accepted
- Engineering readiness: Accepted
- Onboarding readiness: Accepted for local, OIDC, telemetry, API, Admin, and Governed consumers
- Current source version: v0.3.0-alpha.2
- Target version: v0.3.0-alpha.2
- Distribution status: Prepared
- Version tags: v0.3.0-alpha.2, components/postgres/v0.3.0-alpha.2, components/governedpostgres/v0.3.0-alpha.2, components/oidc/v0.3.0-alpha.2, components/otel/v0.3.0-alpha.2
- Remote consumer verification: Pending
- Latest supported release: v0.3.0-alpha.1
- Release: https://github.com/iiwish/modary/releases/tag/v0.3.0-alpha.2
- Published modules: root, components/postgres, components/governedpostgres, components/oidc, components/otel
- Canonical remote: https://github.com/iiwish/modary
- Owner-selected redistribution license: Apache-2.0
- Private security reporting channel: https://github.com/iiwish/modary/security/advisories/new
- Last updated: 2026-08-27

## Scope

The release is a security maintenance publication of the accepted Production
Foundation. It uses Go 1.26.7 across source modules, generated Profiles, and
generated build images. Framework APIs, migrations, Profiles, routes, and
generated product behavior remain unchanged.

## Acceptance

T042 through T047 retain the complete Production Foundation acceptance. T049
proves exact Go and module baselines, zero reachable vulnerabilities, copied-out
Profiles, real PostgreSQL, frontend reproducibility, non-root containers,
platform builds, documentation, and candidate source integrity with no
unresolved P0 through P2 finding.

## Release Boundary

Alpha 2 is one immutable commit and five annotated tags. Normal Go module
resolution must return the exact version without a replacement for every
module. Hosted main and tag CI, local and hosted remote consumers, and the
GitHub prerelease identify the same commit. Modary publishes source and
documentation, not a hosted product, container registry, database service,
identity provider, collector, or stable-v1 promise.
