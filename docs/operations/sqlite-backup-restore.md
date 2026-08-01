# SQLite Backup And Restore

The official F0 durable profile keeps framework state, consumer mutation,
idempotency, successful results, and required audit behavior in one SQLite
transaction domain. Backup and restore are consumer operational responsibilities.

## Define Recovery Objectives

Before deployment, choose acceptable recovery point and recovery time objectives,
retention, encryption, off-host storage, and who can restore. A file that has
never been restored in a representative environment is not a verified backup.

## Consistent Backup

Use a SQLite-aware backup method appropriate to the selected driver and journal
mode. Do not assume that copying only the main database file while the process
is writing captures a consistent database; active `-wal` and `-shm` state may be
part of the live database.

The safest general release procedure is:

1. stop public writes and drain the application;
2. close the application cleanly;
3. copy the complete database using an operator-approved method;
4. record consumer version, Modary version, schema state, time, and checksum;
5. protect the backup with the required access and encryption policy;
6. restart and verify readiness.

A consumer may implement online backup only after validating the exact SQLite
driver API and concurrency behavior. Modary F0 does not expose raw database
authority to feature modules for ad hoc backup.

## Pre-Upgrade Backup

Take and verify a backup before starting a binary that contains new migrations.
Migrations are forward-only and may commit before a later module fails startup.
Keep the pre-upgrade binary, configuration, and database backup together as one
rollback set.

## Restore Test

Restore into a private path that satisfies the SQLite directory ownership
policy. Start the exact compatible consumer version, verify migrations and
readiness, inspect representative domain state and audit history, and exercise a
governed read/write workflow. Never point a restore test at production paths or
credentials.

## Incident Restore

1. stop all processes that can access the database;
2. preserve the damaged database and logs for diagnosis;
3. verify the selected backup checksum and compatibility metadata;
4. restore into a new owner-controlled directory rather than overwriting the
   only damaged copy;
5. start one compatible application process;
6. verify readiness, integrity, domain invariants, Action execution, and audit;
7. reopen traffic only after product and operations approval.

## Downgrade

F0 has no reverse migration mechanism. Downgrade means restoring the database
captured before the upgrade and running its matching older consumer binary. Do
not run older source against a newer migrated database unless the consumer has
an explicit and tested forward-schema compatibility contract.
