# Deployment Profile

Modary F0 is suitable for a single-node, single-process application using one
official SQLite durability domain and explicit local identity/RBAC
configuration. It is not a high-availability platform or a public-internet IAM
system.

## Recommended Topology

Run one consumer-owned executable under a process supervisor. Store the SQLite
database on a local filesystem with stable durability semantics and the
ownership policy required by the adapter. Terminate TLS and apply network policy
at a reviewed consumer-owned server or reverse proxy. Keep generated UI assets
inside the consumer release if they are served by the process.

Avoid network filesystems, multiple independent application writers, shared
database files across containers, or automatic failover unless the consumer has
separately proved SQLite locking, filesystem, backup, and process behavior.

## Configuration

The consumer loads configuration before creating the pure Definition. Separate
non-secret configuration from secrets. Validate database paths, bind addresses,
external origins, cookie policy, trusted proxy settings, and shutdown timeouts.

Do not place credentials, bearer tokens, private keys, or encryption material in
`modary.yaml`, generated files, command arguments, source code, or logs. The
framework provides no universal secret store.

## Startup

Startup validates the entire module graph before side effects, applies
forward-only migrations, starts providers in dependency order, constructs
Handlers, and assembles the application. Keep readiness false until assembly is
complete. A startup failure attempts reverse cleanup of process resources but
does not reverse a committed migration.

Run backup and restore verification before deploying any migration-bearing
upgrade. Never start an older binary against a database migrated by a newer
release unless the consumer explicitly supports that combination.

## Health

Mount the framework health handler explicitly. Use process liveness to detect a
running process and readiness to control traffic only after the application is
assembled. A healthy process does not prove that downstream consumer systems,
backup policy, free disk, or public proxy policy are correct; add consumer-owned
checks where appropriate.

## Shutdown

On SIGTERM or another supervisor signal:

1. stop accepting new public traffic;
2. cancel the application command context;
3. allow Runtime and identity leases to drain;
4. let module cleanup run in reverse dependency order;
5. bound the process supervisor's final timeout beyond the application's
   cooperative shutdown budget.

Non-cooperative callbacks can outlive their timeout and overlap later cleanup.
Treat cancellation compliance as a deployment qualification for every consumer
module and adapter.

## Production Checklist

- The exact Modary and consumer versions are pinned and recorded.
- The deployment matches the [support matrix](../reference/support-matrix.md).
- `make acceptance` or the consumer-equivalent gates pass from a clean checkout.
- TLS, host validation, proxy trust, rate limits, and external origin are explicit.
- Production identity and RBAC provisioning contain no development defaults.
- Token files and database paths satisfy the documented ownership policy.
- Database backup and restore have been tested on the release candidate.
- Disk capacity, database integrity, audit retention, and process logs are monitored.
- Shutdown and restart durability have been exercised under active requests.
- Known limitations are accepted by the product and operations owners.

See [security](security.md) and [SQLite backup and restore](sqlite-backup-restore.md).
