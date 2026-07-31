# ADR-003: SQLite Durable Profile And Module-Atomic Migrations

- Status: Accepted
- Date: 2026-07-31
- Scope: F0 persistence, migration ownership, and filesystem boundary

## Context

F0 needs one operationally small durable profile with precise transaction
semantics. Migration history must fail closed when applied files change or when
an earlier file disappears, and a partially applied Module schema must remain
retryable.

## Decision

The official durable profile uses the pure-Go SQLite driver and one database for
plans, idempotency, authorization, identity, audit, and consumer business state.
Action writes begin an immediate transaction so concurrent retries serialize
before a read-to-write upgrade.

The framework-internal migration controller validates the Module ID and complete
ordered migration set before database side effects. It rejects empty, malformed,
inserted, removed, or checksum-changed history. All pending files and registry
rows for one Module commit in one transaction. Migration files are forward-only
and immutable after release; the transaction boundary does not span independent
Modules.

A source root implements `fs.ReadDirFile` with the positive batch contract. The
controller streams at most 256 directory entries, accepts files no larger than
1 MiB, retains no more than 16 MiB for one Module, bounds names to 255 bytes, and
reads at most one byte past a limit before failing. Applied history is capped at
256 rows. The SQLite validator accepts forward schema and data statements and
rejects explicit transaction control, savepoints, temporary objects,
administrative statements, trigger rollback expressions, and rollback conflict
actions before opening the migration transaction. It also rejects bare and
quoted `temp.` schema qualification because those objects are connection-local
and cannot satisfy durable migration history.

Consumer `database.Access` uses a separate SQL policy: reads are one `SELECT`,
and transaction-bound mutations are one `INSERT`, `UPDATE`, or `DELETE` with no
executable rollback form. SQL validation completes before a write executor is
resolved, so rejected input cannot cross the official backend boundary. SQLite
commit and rollback hooks detect statements
that terminate the framework-owned transaction even when parser-level policy is
bypassed by privileged framework code.

The Module Host owns migration timing. It applies a Module's matching migration
set after the database capability exists and before constructing that Module's
Action handlers. Consumer Modules receive only the narrow `database.Access`;
privileged migration and transaction control uses an internal sealed type and a
private typed service key shared only by Host assembly and official durable
adapters. F0 does not expose an application-level custom durable-adapter plug-in
boundary.

The file-backed adapter anchors relative paths at Definition construction,
creates missing directories with owner-only permissions, and validates directory,
database, and sidecar ownership and modes before and after opening the database.
It rejects symlinks and requires every directory ancestor to be owned by the
effective UID or root. A group- or other-writable ancestor is accepted only when
it is root-owned and sticky. The final database directory remains
effective-UID-owned and non-writable by group or other users. The adapter never changes
permissions or ownership of existing paths and fails closed where ownership
cannot be verified.

## Consequences

- A failed pending migration leaves no partial schema or registry state for its
  Module and can be retried after the cause is repaired.
- Applied SQL history is append-only; schema rollback requires a new forward
  migration or an operator restore procedure.
- Cross-Module startup is not globally transactional. Lifecycle cleanup applies
  to process resources, not already committed schemas.
- File mode and UID checks reduce exposure to distinct unprivileged local users
  but do not defend against root, the same UID, ACL overrides, mount replacement,
  or filesystems that falsify metadata.
- An ACL-aware adapter is required for file-backed operation on Windows.

## Rejected Alternatives

- One transaction per migration file leaves a Module partially upgraded.
- Rewriting applied checksums destroys reproducibility and hides drift.
- A global migration transaction couples independent Modules and is not portable
  to future storage implementations.
- Automatically changing permissions or ownership can silently seize or expose
  operator-managed data.
