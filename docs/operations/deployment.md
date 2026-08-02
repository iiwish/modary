# Deployment

Deployment topology follows the selected component graph. Modary does not
operate PostgreSQL, terminate public TLS, schedule containers, or provide a
public-internet identity service.

## API Profile

Run the generated Go command under process supervision. No database or worker
is required by the framework. Add health checks for consumer dependencies and
drain the HTTP server before calling application shutdown.

## Admin Profile

Run one or more API/UI processes against the same PostgreSQL application
schema. The Go binary embeds the prebuilt frontend; production does not install
Node.js.

Replace development Identity as required, terminate TLS, keep secure cookies,
and enforce host, origin, proxy, account, request, and rate policy at reviewed
boundaries. PostgreSQL pooling, failover, vacuum, capacity, network policy, and
backups remain operator responsibilities.

Admin has no River worker unless the project explicitly adds a task component.

## Governed Profile

Run API and worker binaries as separate supervised processes. Both connect to
one PostgreSQL database and the same exclusive application/queue schema pair.
API processes may enqueue without running workers; worker processes consume
through River. Each runner's concurrency is a per-process limit.

A separate queue database is not equivalent: it cannot share the Action's local
transaction.

## Startup

Startup validates the complete Module graph before side effects, then starts
providers in dependency order and applies Module migrations. Governed startup
also verifies the schema pair, applies River migrations, and assembles Action
handlers. PostgreSQL advisory locks serialize schema bootstrap across processes.

Keep readiness false until `appkit.Start` returns and route construction
succeeds. A later startup failure cleans process resources but does not reverse
an already committed forward migration.

## Shutdown

1. stop accepting new traffic;
2. cancel the process context;
3. stop task runners with a bounded soft-drain context when present;
4. call application shutdown and allow active facade leases to drain;
5. keep the supervisor deadline longer than application and runner budgets.

Callbacks and handlers must honor cancellation. At-least-once work still needs
idempotency when a process stops after an external effect but before River
records completion.

## Production Checklist

- Exact application, Modary, Go, PostgreSQL, and frontend dependency versions
  are recorded.
- Generated examples and development credentials are replaced or explicitly
  accepted for the trust boundary.
- TLS, cookies, host/origin/proxy handling, rate limits, and access logs are
  reviewed.
- Database roles own only required schemas and credentials are rotated.
- Admin asset drift checks pass and no Node runtime is in the deployment image.
- Governed application/queue schemas are distinct and consistently paired.
- Queue lag, retries, discards, and oldest-job age are monitored when River is
  selected.
- Backup and restore tests cover every selected schema.
- Shutdown/restart is tested with active requests and, when selected, jobs.
- Known limitations are accepted or mitigated.
