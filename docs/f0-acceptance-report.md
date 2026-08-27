# Modary v0.3 F0 Acceptance Report

- Status: Accepted
- Distribution status: Released
- Date: 2026-08-27
- Target version: v0.3.0-alpha.2
- Version tags: v0.3.0-alpha.2, components/postgres/v0.3.0-alpha.2, components/governedpostgres/v0.3.0-alpha.2, components/oidc/v0.3.0-alpha.2, components/otel/v0.3.0-alpha.2
- Frozen baseline tag: v0.3.0-alpha.1
- Current specification: `.ai-platform/specs/012-security-maintenance-release/spec.md`
- Current acceptance evidence: `.ai-platform/evidence/T049/`
- Release evidence: `.ai-platform/evidence/T050/`

## Accepted Boundary

Modary is a lightweight componentized Go backend framework. The database-free
Core owns explicit Module graphs, typed capabilities, lifecycle, process and
HTTP composition contracts. Consumer projects own domain Modules, product
Scope, routes, schema, UI source, deployment, data, and release policy.

The accepted Profiles are database-free API, ordinary PostgreSQL React Admin,
and governed PostgreSQL/River operations. Local password, OIDC browser login,
task inspection, audit inspection, and OTLP telemetry are explicit selections.
Unselected heavy components are absent from fresh source and module graphs.

## Production Foundation

- Principals are scope-independent. Consumer routing derives product Scope and
  exact RBAC bindings remain default deny across zero, one, or many scopes.
- Password verification, sessions, bearer tokens, and browser redirect login
  are separate capabilities. OIDC trusts no role or Scope claim.
- `/livez` is local-only; `/readyz` covers lifecycle and bounded selected
  dependencies and becomes false before graceful drain.
- Database Profiles provide migration-only operation and may serve without
  applying migrations.
- Generated OCI source is static, non-root, multi-architecture, embeds Admin
  assets, includes build identity, and keeps Node and source out of runtime.
- JSON `slog` diagnostics are required; the optional OTel module owns local
  providers and bounded HTTP/database/task telemetry without globals or secret
  and high-cardinality dimensions.

## Security Maintenance

- The exact Go baseline is 1.26.7 for source modules, generated Profiles, and
  generated build images.
- The pinned vulnerability gate reports zero reachable findings for the root and
  four published component modules.
- Framework APIs, migrations, Profiles, routes, and generated product behavior
  remain within the accepted Production Foundation boundary.

## Verification

T042 through T047 cover focused TDD, root and four nested modules, real
PostgreSQL, disposable OIDC provider and OTLP collector, copied-out API/local
Admin/OIDC Admin/telemetry Admin/Governed consumers, React variants, non-root
OCI execution, migration ordering, probes, active-request drain, dependency
failure, race, repeat, fuzz, vet, vulnerability, cross-build, deterministic
generation, docs, source stability, and four-pass review.

The Production Foundation review records no unresolved P0, P1, or P2 issue.
T049 records the Go 1.26.7 maintenance candidate, complete validation, and a
digest over the candidate outside its own evidence directory. T050 records the
immutable five-tag release, hosted CI, normal module resolution, and remote
consumer result.

## Limits

This acceptance does not provide hosted IAM, MFA/recovery, MySQL, SQLite,
Kubernetes operators, TLS automation, secret distribution, PostgreSQL HA,
backup storage, a bundled collector/dashboard, a component marketplace, Rulary
product behavior, or stable-v1 compatibility. See
[known limitations](f0-known-limitations.md) and the
[support matrix](reference/support-matrix.md).
