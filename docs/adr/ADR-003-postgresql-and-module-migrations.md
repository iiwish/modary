# ADR-003: PostgreSQL Control Store And Module-Atomic Migrations

- Status: Accepted
- Date: 2026-08-01
- Scope: F0 persistence, task delivery, and migration ownership

## Context

Modary needs one durable control plane with precise Action transaction semantics,
multiple-process concurrency, recoverable background work, and migration history
that fails closed. Product business data must remain consumer-owned and may live
in another database or service.

## Decision

The official durable profile uses one PostgreSQL control database. Modary and
consumer control tables live in an application schema. River owns a distinct
queue schema in the same database. Sharing the database is intentional: an
Action can update domain control state and insert a River job through the exact
same `database/sql` transaction. A dedicated River database would lose that
atomicity and is not the supported profile.

The public `task` package contains framework-neutral enqueue and worker
contracts. River, pgx, schema migration, and raw transaction types stay inside
`adapters/postgres`. Delivery is at least once. Task handlers use stable job
identity and make external effects idempotent.

The framework migration controller validates the complete ordered migration set
before database effects. It rejects removed, inserted, or checksum-changed
applied history. All pending files and registry rows for one Module commit in
one PostgreSQL transaction. Cross-Module startup is ordered but not one global
transaction.

Migration sources are bounded to 256 entries, 1 MiB per file, 16 MiB per Module,
and 255-byte names. The SQL policy rejects explicit transaction control,
savepoints, temporary objects, and administrative statements. Consumer
`database.Access` accepts reads outside a transaction and only single
`INSERT`/`UPDATE`/`DELETE` mutations inside the governed transaction.

The configured database role must own both schemas. System and `public` schemas
are rejected. PostgreSQL search paths are pinned to the application schema;
River receives its schema explicitly. Each physical schema stores the same
role-aware profile marker, and bootstrap locks are keyed by physical schema
rather than role. Re-pairing, sharing, and application/queue role exchange fail
closed while identical profile pairs may start concurrently.

## Consequences

- Action state, idempotency, required audit, and task insertion can commit or
  roll back as one unit.
- River needs tables in a separate schema, not a separate database.
- Business data outside the control database is not part of Modary's local
  transaction and requires product-level idempotency or integration patterns.
- Multiple application and worker processes are supported by PostgreSQL and
  River, while distributed transactions remain outside the framework contract.
- Schema rollback uses a forward migration or an operator restore; applied
  migration files are immutable.

## Rejected Alternatives

- An embedded file database as the official profile limits deployment topology
  and makes durable task recovery a second storage concern.
- A separate River database prevents atomic domain-write-plus-enqueue.
- Exposing River or raw PostgreSQL transaction types couples consumers to an
  implementation detail and bypasses framework authority.
- A global migration transaction couples independent Modules and complicates
  operational recovery.
