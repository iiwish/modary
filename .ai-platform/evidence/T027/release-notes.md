# Modary v0.1.0-alpha.3

Modary is a Go-first framework for governed modular applications. Alpha 3
publishes the PostgreSQL control profile and River-backed durable task runtime,
while keeping business schemas, UI, deployment, and product behavior under
consumer ownership.

## Install

```bash
go get github.com/iiwish/modary@v0.1.0-alpha.3
```

Pin the exact prerelease. Go 1.26 or newer and PostgreSQL 17 are required for
the supported durable profile. Node.js is not required.

## Included

- Explicit Module composition, typed capabilities, and bounded lifecycle.
- Governed Actions with authorization, Preview/Execute binding, idempotency,
  PostgreSQL transactions, and required audit ordering.
- Public transactional task enqueue and immutable runners backed internally by
  River, including retry, recovery, duplicate suppression, and multi-worker use.
- PostgreSQL-native Identity, RBAC, SQL Audit, migrations, plans, idempotency,
  and the independently consumable Counter application.
- CLI, HTTP, MCP, project verification, deterministic generation, build tools,
  English and Chinese onboarding, operations guidance, and full quality gates.

## Compatibility

Alpha 3 replaces the Alpha 2 durable profile. The former embedded control store
and its adapter are absent, and there is no automatic in-place data migration.
Consumers configure distinct application and River schemas in PostgreSQL,
regenerate consumer-owned outputs, and rehearse their data transition before
deployment.

Public APIs, generated formats, and migrations remain pre-v1 Alpha contracts.
Review `CHANGELOG.md`, `docs/releases/upgrade-guide.md`, generated diffs, and
database backups on every upgrade.

## Boundaries

Modary does not provide a generic admin UI, product schema, scaffold generator,
container, public-internet IAM suite, distributed transaction system, database
failover controller, or hostile plugin sandbox. Task delivery is at least once;
consumer handlers keep external effects idempotent.

Read `docs/f0-known-limitations.md`, `docs/reference/support-matrix.md`, and
`docs/operations/security.md` before deployment. Report vulnerabilities through
https://github.com/iiwish/modary/security/advisories/new.
