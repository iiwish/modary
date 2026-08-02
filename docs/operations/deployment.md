# Deployment Profile

Modary F0 uses one PostgreSQL control database. API processes and River worker
processes may scale independently. Modary is not a distributed transaction
coordinator, PostgreSQL operator, or public-internet IAM system.

## Recommended Topology

Run consumer-owned API and worker executables under process supervision. Both
connect to the same control database. Modary and product control tables live in
the application schema; River lives in a distinct queue schema. Business data
may remain in consumer-selected databases or APIs behind Connector interfaces.

Terminate public TLS and apply host, origin, proxy, and rate policy at the
consumer boundary. PostgreSQL replication, failover, pooling, statement
timeouts, capacity, vacuum, and network policy remain operator responsibilities.

## Configuration

Load configuration before constructing `appkit.Definition`. Keep the PostgreSQL
URL in a secret store. Validate schema names, bind addresses, external origins,
cookie policy, trusted proxies, worker queues, concurrency, job timeout, soft
stop timeout, and application shutdown timeout.

The configured role must own both schemas. Treat the application and queue
schemas as one exclusive pair; the adapter persists that binding and rejects a
schema already paired with another profile. Use one database for both schemas
when an Action must atomically write state and enqueue work. A separate queue
database is not equivalent.

## Startup

Startup validates the module graph before side effects, connects PostgreSQL,
creates or verifies owned schemas, verifies their durable pairing, applies River
and Module migrations, starts providers in dependency order, constructs
handlers, and assembles the application. PostgreSQL advisory locks serialize
schema bootstrap and migrations across simultaneous process starts. Keep
readiness false until assembly completes.

A startup failure cleans process resources in reverse order. It does not undo a
Module migration that committed before another Module failed.

## Worker Processes

Construct runners from `application.Tasks()` with explicit queues and worker
limits. A runner is immutable after construction. River delivery is at least
once, so use the job ID or a product run ID as an idempotency key for external
effects. Do not assume that handler return means exactly-once execution.

## Health

Mount the framework health handler explicitly for API processes. Add
consumer-owned checks for Connectors and product dependencies. Worker health
should cover PostgreSQL reachability, runner startup, queue lag, retry/discard
rates, and the age of the oldest available job.

## Shutdown

1. stop accepting new public traffic;
2. cancel the application command context;
3. stop River from fetching new jobs and allow the soft-stop budget to drain;
4. cancel remaining job contexts before closing the database pool;
5. let Module cleanup run in reverse dependency order;
6. keep the supervisor's final timeout longer than the application budget.

Handlers and callbacks must honor cancellation. External effects still need
idempotency because a process may stop after the effect succeeds but before the
job result is committed.

## Production Checklist

- Exact Modary, River, PostgreSQL, and consumer versions are recorded.
- Both schemas exist, are distinct, exclusively paired, and owned by the application role.
- TLS and PostgreSQL network policy match the deployment trust boundary.
- API and worker concurrency have explicit capacity limits.
- Production identity and RBAC contain no example credentials.
- Queue retry, discard, latency, and backlog are monitored.
- A restore test covers both schemas and restored pending jobs.
- Shutdown and restart are exercised with active requests and jobs.
- Connectors document idempotency, timeouts, and partial-failure behavior.
- Known limitations are accepted or mitigated by product and operations owners.

See [security](security.md) and [PostgreSQL backup and restore](postgresql-backup-restore.md).
