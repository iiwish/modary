# Persistence And Durable Tasks

Modary separates three kinds of state that are easy to confuse.

## Control Database

The official PostgreSQL Profile owns one control database with two schemas:

| Boundary | Schema | Examples |
| --- | --- | --- |
| Application control plane | `modary` by default | Module migrations, Action plans, idempotency results, identity, RBAC, audit, consumer control tables |
| Durable task runtime | `modary_queue` by default | Modary profile binding plus River jobs, queues, leaders, schedules, migration state |

The schema names are configurable, must be distinct, and must be owned by the
configured database role. Modary pins the pgx `search_path` to the application
schema. River always receives its schema explicitly. An application schema and
queue schema form one durable profile: the adapter persists both directions of
that binding and rejects re-pairing or sharing either schema with another
profile. Each schema carries the same role-aware profile marker, so a schema
cannot be reused by changing it from application to queue or vice versa.
Multiple processes configured with the same pair are supported.

The two schemas use the same database because PostgreSQL cannot atomically
commit one transaction across independent databases. Keeping River beside the
application schema lets a governed Action write control state and enqueue a job
using the exact same `*sql.Tx` internally.

## Business Data

Modary does not require product business data to use PostgreSQL. A consumer may
use PostgreSQL, MySQL, an API, object storage, or another system behind a
consumer-owned Connector. That dependency is outside the local control-store
transaction.

Use the control database for workflow and governance state that must be atomic
with task creation. Use Connectors for product data and external effects. When a
task crosses that boundary, design the external operation to be idempotent.

## Transactional Enqueue

`task.Service.Enqueue` is available only inside a governed transaction. Calling
it outside that context returns `task.ErrTransactionRequired`. This rule avoids
the two classic split-brain outcomes:

- product state commits but no job exists;
- a job exists for state that rolled back.

The public API exposes neither River nor PostgreSQL transaction types. The
PostgreSQL adapter recognizes its own context-bound transaction and calls River
`InsertTx` internally.

## Delivery Semantics

Jobs are delivered at least once. A handler may run again after a process crash,
timeout, lease loss, or completion persistence failure. The framework supplies
job ID, task kind, queue, attempt, maximum attempts, and a defensive payload
copy. It does not claim exactly-once external effects.

Choose a stable product identity such as a Run ID. Before an external effect,
load the current product state. After success, persist a terminal state through
a governed Action. Repeated delivery should observe that terminal state and
return without repeating the effect. Where the external system supports an
idempotency key, send the same stable identity on every attempt.

Handler error text is stored in River's job history. Return a stable,
presentation-safe error and send dependency detail to a separately protected
observability sink. Never wrap database URLs, credentials, tokens, request
bodies, or third-party response bodies into the returned task error.

## Process Topology

An API process may enqueue without starting a runner. A worker process may start
one or more immutable runners for selected queues. Multiple processes can work
the same queue because River owns job leasing and coordination in PostgreSQL.
Queue concurrency is a per-runner process limit, not a global limit.

## Failure Boundary

The transactional guarantee ends at the control database. A handler that writes
another database or calls an API cannot extend the PostgreSQL transaction over
that operation. Use idempotency, reconciliation, or a product-specific saga when
the external system can partially succeed.
