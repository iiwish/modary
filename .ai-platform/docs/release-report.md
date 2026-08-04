# Modary v0.2 Alpha 1 Readiness Report

- Report version: 3.0
- Status: Distribution_ready
- Technical F0 acceptance: Accepted
- Engineering readiness: Accepted
- Onboarding readiness: Accepted for copied-out Profiles
- Current source version: v0.2.0-alpha.1 release candidate
- Target version: v0.2.0-alpha.1
- Distribution status: Not_released
- Version tags: None
- Remote consumer verification: Not_run
- Latest published historical version: v0.1.0-alpha.3
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

The candidate is suitable for exact-version design-partner adoption after the
coordinated immutable tag train, hosted CI, normal Go module resolution, and
remote copied-out consumer verification pass.

## Release Boundary

Publication uses one commit and three annotated tags:
`v0.2.0-alpha.1`, `components/postgres/v0.2.0-alpha.1`, and
`components/governedpostgres/v0.2.0-alpha.1`. The release publishes Go source
and documentation, not a hosted product, binary distribution, container,
database service, stable-v1 compatibility promise, or public-internet IAM
suite. Failed external verification stops publication without moving a tag.
