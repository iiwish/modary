# PostgreSQL Control Store And Task Runtime

- Version: 1.0
- Status: Confirmed
- Approval source: owner-approved PostgreSQL-first rewrite with no compatibility requirement on 2026-08-01
- Last updated: 2026-08-02
- Governing constitution: `../../memory/constitution.md`

## Objective

Make PostgreSQL the single official Modary durable profile and provide a
domain-neutral, transaction-aware background task contract backed by River.
Delete SQLite and all compatibility code rather than preserving two persistence
models.

## Storage Boundaries

- The control database stores Modary runtime state and consumer control-plane
  state. It is required by the official durable profile.
- The application schema owns Modary and consumer module tables.
- A distinct queue schema in the same PostgreSQL database is owned by River.
  This is a schema boundary, not a requirement for a second database.
- Consumer business data remains outside the control-store contract. Products
  reach PostgreSQL, MySQL, warehouses, APIs, or other systems through their own
  Connector abstractions.

## Functional Requirements

- FR-001: `adapters/postgres` is the only official durable profile and accepts
  an explicit PostgreSQL URL plus validated application and queue schema names.
- FR-002: PostgreSQL startup creates and exclusively pairs the two configured
  schemas, serializes concurrent bootstrap and migration attempts, applies
  River migrations to the queue schema, and applies checksum-protected Modary
  and Module migrations to the application schema.
- FR-003: public reads use the application connection pool; writes remain
  restricted to the context of a governed Action transaction.
- FR-004: nested governed operations join the outer PostgreSQL transaction,
  mark it rollback-only after a nested failure, and never expose commit,
  rollback, raw connection, or transaction authority.
- FR-005: the public `task` package defines a bounded enqueue, job, handler, and
  runner lifecycle contract without exposing River or PostgreSQL types.
- FR-006: task enqueue requires the current governed transaction and uses
  River's transactional insert on the exact same `*sql.Tx`.
- FR-007: task runners use River's durable claim, retry, recovery, concurrency,
  and graceful-stop behavior and support multiple processes safely.
- FR-008: standard Identity, RBAC, SQL Audit, migration, plan, and idempotency
  persistence use PostgreSQL-native DDL, placeholders, types, and constraints.
- FR-011: local Identity provisions principals independently from optional
  password and bearer credentials so service principals require no synthetic
  interactive secret.
- FR-009: the Counter consumer proves independent composition and PostgreSQL
  migration behavior.
- FR-010: all active SQLite packages, migrations, dependencies, documentation,
  scripts, and tests are removed.

## Non-Functional Requirements

- NFR-001: no compatibility alias, deprecated wrapper, schema translator, or
  dual-database branch is retained.
- NFR-002: SQL identifiers supplied as options are strictly validated and only
  safe quoted identifiers are used in administrative SQL.
- NFR-003: credentials are never logged or embedded in generated artifacts;
  connection errors cross the existing opaque dependency boundary.
- NFR-004: queue payloads are valid JSON, size bounded, and defensively copied.
- NFR-005: task delivery is at least once. Consumers use stable task identity
  and idempotent effects; the framework does not claim exactly-once execution.
- NFR-006: the PostgreSQL integration suite uses a real supported PostgreSQL
  server and proves rollback, commit, migration drift, enqueue atomicity,
  concurrent process bootstrap, exclusive schema pairing, duplicate
  suppression, multi-runner claiming, retry, restart recovery, and shutdown.
- NFR-007: Modary remains domain-neutral and contains no downstream product schema, rule,
  run, connector, or product vocabulary.

## Success Criteria

- SC-001: no active source, dependency, example, or canonical documentation
  references SQLite.
- SC-002: a failed governed Action leaves neither its domain row nor River job;
  a successful Action commits both.
- SC-003: two independent runners cannot work one job concurrently, and a job
  survives process restart and is retried or discarded according to River.
- SC-004: standard adapters, Counter, full Modary tests, race tests, quality
  checks, docs checks, and governance validation pass.
- SC-005: The copied-out Counter consumer can use only public Modary packages
  to persist consumer-owned control state, enqueue a task transactionally, and
  operate a River-backed worker after an application restart.

## Non-Goals

- MySQL support in this phase.
- Using River as a business-data store.
- A generic distributed transaction across the control database and external
  Connector systems.
- A queue dashboard, cron UI, workflow engine, or arbitrary River API mirror.
- Migration of existing SQLite databases or source compatibility with the
  unpublished SQLite profile.

## Acceptance Boundary

Acceptance requires the implementation, deletion audit, real PostgreSQL
integration evidence, copied-out Counter conformance, documentation, and two
current review passes to contain no unresolved P0 through P2 finding. No
downstream product repository is an acceptance dependency.
