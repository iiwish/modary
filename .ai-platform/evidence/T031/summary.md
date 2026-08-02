# T031 Optional Admin Backend Profile

- Status: Completed
- Date: 2026-08-02

`database.Store` is the provider-neutral ordinary business-data capability. It
supports bounded reads and callback-scoped transactions without exposing raw
connections, commit, rollback, migration, or administration. The separate
`module.ActionDatabase()` capability remains unable to begin a transaction and
can mutate only with a governed Runtime context. Both capabilities are
atomically published from framework-private database control and revoked with
the Host lifecycle.

`adapters/postgresdb` is the lightweight general PostgreSQL component. It owns
one application schema, migration serialization, and Store publication. Its Go
package and copied-out Admin dependency graph contain no River or governed
PostgreSQL profile. `adapters/postgres` remains the explicit governed profile.

`transport/sessionhttp` provides only login, current session, logout, session
middleware, and session-plus-CSRF mutation middleware. Authentication state is
server-side, cookies are host-only, HttpOnly, SameSite Strict, and secure by
default, request bodies and time are bounded, dependency failures are contained,
and the authenticated Actor is installed in request context for product routes.

The create-only Admin Profile explicitly composes `postgresdb`, local
development Identity, RBAC, `sessionhttp`, and a consumer-owned `records`
vertical slice. The copied-out application performs authorized, scope-isolated
create/list/update/delete, rejects invalid CSRF, persists through restart, and
has no River queue schema, tasks, audit, governed Action, or MCP route/service.
