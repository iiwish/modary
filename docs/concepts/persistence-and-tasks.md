# Persistence And Tasks

Modary does not require a database. Persistence enters through one of two
explicit PostgreSQL components with different authority and dependency graphs.

## Ordinary PostgreSQL

`adapters/postgresdb` installs one provider-neutral `database.Store` and one
application schema. It has no River, Action plan, idempotency, or audit
dependency.

Reads use bounded `SELECT` statements. Mutations use one `INSERT`, `UPDATE`, or
`DELETE` inside `Store.WithinTransaction`:

```go
err := store.WithinTransaction(ctx, func(txCtx context.Context) error {
    _, err := store.ExecContext(txCtx,
        "UPDATE records SET status = $1 WHERE record_id = $2", status, id)
    return err
})
```

The callback is synchronous. The Store owns begin/commit/rollback and does not
expose a raw connection. This is the Admin Profile path.

## Governed PostgreSQL And River

`adapters/postgres` installs governed Action persistence, `database.Access`, and
the provider-neutral `task.Service`. It uses one PostgreSQL database with two
owned schemas:

| Schema | Contents |
|---|---|
| application | Module history, product control tables, plans, idempotency, Identity, RBAC, audit |
| queue | profile binding and River migrations/jobs/coordination |

The schemas must be distinct and owned by the configured role. River needs a
schema and tables, not a dedicated database. Each schema stores a role-aware
pair binding; sharing, re-pairing, or exchanging application/queue roles fails
closed.

The same database is required for the F0 atomicity claim. A governed Action can
write application state and call `task.Service.Enqueue` through the exact same
internal PostgreSQL transaction. Enqueue outside that context returns
`task.ErrTransactionRequired`.

## Business Database Choice

F0 officially implements PostgreSQL. Core contracts remain provider-neutral,
and a consumer may use another database or service behind its own Module, but
Modary does not ship or claim MySQL conformance yet. An external database cannot
join the governed PostgreSQL transaction; use idempotency, reconciliation, or a
product saga across that boundary.

## Delivery

River delivery is at least once. A handler may run again after timeout, lease
loss, process crash, or completion-persistence failure. Use stable product
identity and make external effects idempotent.

`task.Job` contains the job ID, kind, queue, attempt, maximum attempts, and a
defensive payload copy. A runner has immutable queue/concurrency/timeouts and an
explicit Start/Stop lifecycle. API and worker processes may scale separately.

Returned task errors may be retained in River history. Keep them bounded and
presentation-safe; send credentials, connection strings, response bodies, and
other dependency details only to a protected observability sink.

## Choosing The Path

- No persistence: API Profile.
- Ordinary business CRUD: `postgresdb` plus `database.Store`.
- Previewed/audited/idempotent mutation with atomic task insertion:
  `postgres` plus `action.Runtime` and `task.Service`.

Do not select River simply to run any asynchronous function. Select it when
durability, retry, and transactionally recorded intent are product requirements.
