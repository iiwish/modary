# Troubleshoot Modary Applications

Run the failing command with `GOWORK=off` and keep the first structured error.
Do not bypass it by importing `internal` packages, weakening a descriptor, or
rewriting an applied migration.

## Starter Refuses The Destination

`modary new` accepts one nonexistent or empty real directory whose base is a
lowercase project ID. It rejects symlinks, non-directories, unsafe parents, and
any existing content. The command is create-only and intentionally has no force
or merge flag.

## The Generated Project Cannot Resolve Modary

Inspect module selection:

```bash
GOWORK=off go list -m -json github.com/iiwish/modary
```

The current source workflow needs the local replacement written through
`MODARY_STARTER_REPLACE`. A released consumer removes that replacement and pins
an exact published tag.

## Graph Validation Fails

Common causes are duplicate Module IDs, a missing capability provider, multiple
providers for one capability, a dependency cycle, an undeclared resolve/provide
operation, or an invalid Action descriptor. Definition construction must remain
free of database, network, random, hashing, and goroutine work.

## A Typed Service Cannot Be Resolved

Provider and consumer must import the same package-level `module.Key[T]`.
Recreating its string and type does not recreate identity. Also verify that the
consumer declares the capability requirement and the provider declares it.

## A PostgreSQL Migration Is Rejected

Migration history is forward-only. Add a new file; never change or remove an
applied migration. The policy rejects transaction control, temporary objects,
administrative statements, invalid ordering, and bounded-source violations.

For Governed startup, application and queue schemas must be distinct, owned by
the configured role, and consistently paired. For Admin, configure only the
application schema used by `postgresdb`.

## An Ordinary Mutation Requires A Transaction

Call `database.Store.ExecContext` only with the context supplied to
`WithinTransaction`. Do not use a request context directly for a write.

## A Task Enqueue Requires A Transaction

`task.Service.Enqueue` is available only inside Governed
`Handler.Execute`. Admin has no task service by default. Do not retain the
transaction-bound context or enqueue from a route after Execute returns.

## An Action Request Is Rejected

Check Action ID, channel, actor, scope, permission, exact input, Preview policy,
plan hash, idempotency key, optimistic version, and declared error code. Repeat
Preview after relevant state changes.

Public errors intentionally omit dependency diagnostics. Reproduce the path in
a protected integration test and observability sink when more detail is needed.

## Admin Login Works Locally But Not Over HTTP

Secure cookies are the default. Set `MODARY_ALLOW_INSECURE_COOKIE=true` only for
local HTTP. Production uses TLS. Also check CSRF header propagation, duplicate
cookies, account rate limiting, and the configured Identity replacement.

## Admin Assets Drift

From `web/` run:

```bash
pnpm install --frozen-lockfile
pnpm build
pnpm assets:check
```

Commit the source and matching `internal/web/dist` outputs together.

Continue with [testing](test-application.md), [security](../operations/security.md),
and [known limitations](../f0-known-limitations.md).
