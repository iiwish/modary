# Modary v0.2 Alpha 1 Readiness Report

- Report version: 3.1
- Status: Remote_verified
- Technical F0 acceptance: Accepted
- Engineering readiness: Accepted
- Onboarding readiness: Accepted for tagged and copied-out Profiles
- Current source version: v0.2.0-alpha.1
- Target version: v0.2.0-alpha.1
- Distribution status: Released
- Version tags: v0.2.0-alpha.1, components/postgres/v0.2.0-alpha.1, components/governedpostgres/v0.2.0-alpha.1
- Remote consumer verification: Passed
- Latest supported release: v0.2.0-alpha.1
- Candidate and tag commit: 3600a38345380401f36958970f82cc93e2c29cd2
- Root annotated tag object: 51b98be0c7809e3e97b88bb45e0514cbc452dc73
- PostgreSQL annotated tag object: 4c8d459192f3e8a176841442d837b3a3ac747cdd
- Governed PostgreSQL annotated tag object: d3b5a7699b2e8ed56aa5fd5d32b0179cd3257134
- Main CI: https://github.com/iiwish/modary/actions/runs/30870905898
- Tag CI: https://github.com/iiwish/modary/actions/runs/30871605918
- Release: https://github.com/iiwish/modary/releases/tag/v0.2.0-alpha.1
- Published modules: root, components/postgres, components/governedpostgres
- Canonical remote: https://github.com/iiwish/modary
- Owner-selected redistribution license: Apache-2.0
- Private security reporting channel: https://github.com/iiwish/modary/security/advisories/new
- Last updated: 2026-08-04

## Scope

The acceptance object is the lightweight componentized Modary Go framework.
Core provides explicit Module composition, typed capabilities, deterministic
lifecycle, and bounded HTTP/Admin contribution contracts without requiring a
database, task queue, identity implementation, Action Runtime, or frontend.

The release adds create-only API, Admin, and Governed Profiles. Ordinary
PostgreSQL and governed PostgreSQL/River implementations are separately
versioned component modules. The optional React Admin is consumer-owned source
with permission-aware navigation, records CRUD, and selectable task and audit
inspection surfaces.

## Current Result

T028 through T041 establish the component architecture, create-only Starter,
React Admin, heavy-module isolation, explicit contribution contracts, shared
Admin primitives, and task/audit surfaces. The complete F0 review reports no
unresolved P0 through P2 finding.

Fresh copied-out API, default Admin, operations Admin, and Governed consumers
pass outside the checkout with `GOWORK=off`. Real PostgreSQL 17 and River tests
cover ordinary and governed transactions, migrations, schema-role exclusion,
task durability, retry, restart, multi-runner behavior, authorization, audit,
and deterministic frontend assets. Core and API module graphs contain no
PostgreSQL or River implementation dependency.

The accepted source is published and remotely consumable as
`v0.2.0-alpha.1`. The coordinated immutable tag train, hosted main and tag CI,
normal Go module resolution, and hosted and local copied-out remote consumer
gates all identify candidate commit
`3600a38345380401f36958970f82cc93e2c29cd2`.

## Release Boundary

Alpha 1 is `Remote_verified` through one commit and three annotated tags:
`v0.2.0-alpha.1`, `components/postgres/v0.2.0-alpha.1`, and
`components/governedpostgres/v0.2.0-alpha.1`. Future releases repeat the clean
candidate, immutable coordinated tags, hosted CI, normal module resolution,
copied-out consumer, and matching release-note process. Published tags remain
immutable. This release provides Go source and documentation, not a hosted
product, binary distribution, container, database service, stable-v1
compatibility promise, or public-internet IAM suite.
