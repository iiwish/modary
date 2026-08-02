# PostgreSQL Backup And Restore

The official durable profile stores framework state, product control data,
idempotency results, required audit events, and River jobs in one PostgreSQL
database. Backup and restore remain application operations responsibilities.

## Recovery Objectives

Define recovery point and recovery time objectives, retention, encryption,
off-site storage, and restore authority before deployment. A backup is verified
only after a representative restore test.

## Backup Boundary

A consistent backup includes both configured schemas:

- the application schema containing Modary and consumer control tables;
- the River queue schema containing job, leader, queue, and migration state.

Use PostgreSQL physical backup or `pg_dump`/`pg_restore` according to the
operator's recovery design. Do not back up the schemas at unrelated snapshots.
Record the application version, exact Modary version, PostgreSQL version,
schema names, backup time, and checksum.

## Before Upgrade

Take and verify a backup before starting a binary with new migrations. Migrations
are forward-only and a completed Module migration is not undone when a later
Module fails startup. Keep the prior binary, configuration, and backup as one
rollback set.

## Restore Test

1. Restore both schemas into an isolated PostgreSQL database.
2. Assign schema ownership to the configured application role.
3. Start the exact compatible application version.
4. Verify readiness, migration history, representative product state, audit
   history, queued and retryable jobs, and one governed write workflow.
5. Confirm workers do not duplicate external effects when restored jobs retry.

## Incident Restore

Preserve the damaged database and logs, select a verified backup, restore into a
new database, validate integrity, then switch application configuration. Do not
overwrite the only damaged copy. Reopen traffic only after application and
operations approval.

## Downgrade

F0 has no reverse migration mechanism. Downgrade means restoring the pre-upgrade
database and running its matching older binary. Do not run older code against a
newer schema without an explicit, tested forward-schema compatibility contract.
