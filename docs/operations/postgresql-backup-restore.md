# PostgreSQL Backup And Restore

Backup scope follows selected components. Define RPO, RTO, retention,
encryption, off-site storage, checksum, and restore authority before deployment.
A backup is verified only after a representative restore.

## Admin

Back up the configured application schema containing product tables, local
Identity when selected, RBAC, Module migration history, and other consumer
state. No River queue schema exists by default.

## Governed

Take a consistent backup of both schemas in the same database:

- application: product control state, plans, idempotency, Identity, RBAC, audit,
  and Module history;
- queue: River jobs, leaders, queues, schedules, migrations, and profile binding.

Do not capture the two schemas at unrelated points. Record application, Modary,
PostgreSQL, and River versions, schema names, backup time, and integrity hash.

## Before Upgrade

Verify a backup before starting a binary with new migrations. Migrations are
forward-only and not reversed when a later Module fails startup. Keep the prior
binary, configuration, and backup as one recovery set.

## Restore Test

1. restore selected schemas into an isolated database;
2. assign ownership to the intended role;
3. start the exact compatible application version;
4. verify Module history, representative product state, Identity/RBAC, and
   application behavior;
5. for Governed, verify audit, plans/idempotency, queued/retryable work, and one
   Preview/Execute/worker flow;
6. confirm restored tasks do not repeat external effects incorrectly.

## Downgrade

F0 has no reverse migration mechanism. Downgrade means restoring the matching
pre-upgrade backup and binary. Do not run older code against a newer schema
without an explicit tested forward-schema compatibility contract.
