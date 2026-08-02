# PostgreSQL Control Store And Task Runtime Plan

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-08-02

## Technical Decisions

1. Use `pgx/v5/stdlib` behind `database/sql`. The existing narrow SQL facade and
   River's `riverdatabasesql` driver therefore share the exact same transaction.
2. Use one PostgreSQL database with separate validated application and queue
   schemas. A separate queue database is rejected because it would destroy
   transactional enqueue without adding an isolation benefit needed by F0.
   Persist the schema pair as an exclusive profile and use PostgreSQL advisory
   locks to serialize simultaneous process bootstrap and migrations.
3. Keep unqualified consumer migration SQL. Every pooled connection receives a
   fixed application `search_path`; River receives its queue schema explicitly.
4. Add a small public `task` contract and expose its lifecycle-gated service
   through the Module service registry and `appkit.Application`.
5. Represent framework tasks as one versioned River envelope. Logical task kind
   and JSON payload stay framework concepts; River remains an adapter detail.
6. Create one River client per runner. This freezes handlers and queues before
   start, supports independent process concurrency, and avoids mutable worker
   registration after startup.
7. Preserve at-least-once semantics explicitly. A stable unique key prevents
   duplicate active insertion; handlers must make external effects idempotent.
8. Delete SQLite rather than abstracting common SQL prematurely. PostgreSQL is
   the only implemented dialect; future databases must earn a separate adapter
   through conformance tests.

## Implementation Sequence

1. Establish the public task contract, canonical capability, lifecycle facade,
   PostgreSQL options, backend, transaction binding, and River integration.
2. Port framework persistence and standard adapter migrations/queries to
   PostgreSQL, retaining the existing governance and hostile-dependency tests.
3. Port Counter, integration infrastructure, CI, operational documentation,
   and delete every active SQLite surface.
4. Validate the independently copied-out Counter consumer against the public
   PostgreSQL and task contracts, including durable work consumed after an
   application restart.
5. Run focused RED/GREEN loops, complete real PostgreSQL and race acceptance,
   audit boundaries and deletion, perform two review passes, and record evidence.

## Risk Controls

- Administrative SQL only interpolates identifiers accepted by a strict ASCII
  identifier validator and quoted by PostgreSQL-aware code.
- The adapter never falls back to `public` or an environment-derived schema.
- River migrations run before any task can be inserted or worked.
- Application and queue schemas cannot be re-paired or shared across profiles;
  concurrent processes using the same pair bootstrap idempotently.
- Transaction bindings are backend-specific, unforgeable outside the adapter,
  and invalid after completion through `database/sql` transaction state.
- Runner stop is explicit, bounded by caller context, and completed before the
  database pool is closed.
- Integration tests use isolated schemas and a real PostgreSQL service so test
  concurrency cannot share mutable tables.
