# Modary v0.2.0-alpha.1

Modary v0.2 Alpha 1 is a lightweight, component-oriented Go framework for
business systems and administrative backends. It separates database-free Core,
ordinary PostgreSQL applications, optional React Admin, and governed
PostgreSQL/River operations into explicit consumer-owned composition.

## Start

Go 1.26.5 or newer is required.

```bash
go run github.com/iiwish/modary/cmd/modary@v0.2.0-alpha.1 \
  new sample-api --profile api --module example.com/acme/sample-api
```

The `api`, `admin`, and `governed` Profiles are create-only presets. Generated
source belongs to the consumer and is never patched automatically.

## Included

- Database-free Core with explicit Modules, typed capabilities, lifecycle, and
  deterministic HTTP/Admin contribution preflight.
- Separately versioned ordinary PostgreSQL and governed PostgreSQL/River
  components.
- Optional React 19 and TypeScript Admin with Chinese primary UI, sessions,
  CSRF, backend RBAC, responsive navigation, records CRUD, and selectable task
  and audit inspection.
- Optional governed Actions with Preview/Execute binding, authorization,
  idempotency, PostgreSQL transactions, SQL Audit, and durable River tasks.
- Copied-out Profile acceptance, deterministic frontend assets, PostgreSQL 17
  integration, race, repeat, fuzz, cross-build, and vulnerability gates.

## Compatibility

This release is intentionally breaking from `v0.1.0-alpha.3`. The old combined
adapter layout is replaced by `components/postgres` and
`components/governedpostgres`; the Admin Starter is React-only; generated
projects have no automatic upgrade command. Review `CHANGELOG.md` and
`docs/releases/upgrade-guide.md` and migrate existing consumers through a
separate reviewable branch.

Pin all selected Modary modules to exactly `v0.2.0-alpha.1`. Public APIs,
generated source, and component boundaries remain pre-v1 Alpha contracts.

## Boundaries

PostgreSQL is the only official database implementation. Local Identity is for
development and controlled internal deployments, not a complete
public-internet IAM system. Tasks are delivered at least once, so consumers
keep external effects idempotent. Modary does not provide a low-code builder,
runtime plugin marketplace, hosted service, container image, database failover,
distributed transactions, or a stable-v1 compatibility promise.

Read `docs/f0-known-limitations.md`, `docs/reference/support-matrix.md`, and
`docs/operations/security.md` before deployment. Report vulnerabilities through
https://github.com/iiwish/modary/security/advisories/new.
