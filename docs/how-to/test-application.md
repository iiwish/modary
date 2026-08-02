# Test A Modary Application

Scale testing to the selected Profile. Do not force database or Action fixtures
into an API-only project.

## Every Profile

Verify:

- pure Definition construction and expected Module IDs;
- selected capabilities and explicit absence of omitted services;
- health and consumer route composition;
- startup cancellation, exactly-once shutdown, and retained-facade rejection;
- copied-out `GOWORK=off` tidy, test, vet, and build.

Inspect dependency graphs for absence when lightweight composition matters:

```bash
GOWORK=off go list -deps ./...
```

## Admin

Use real PostgreSQL and cover:

- migrations and restart persistence;
- login, current session, logout, cookie policy, and CSRF denial;
- backend RBAC allow/default-deny and scope isolation;
- list/create/update/delete plus optimistic version conflict;
- absence of River, Action, Audit, task, and MCP packages/routes;
- frontend lint, typecheck, unit/a11y tests, production build, and asset drift;
- desktop/mobile keyboard, focus, overflow, and command availability.

## Governed

Use real PostgreSQL and cover:

- actor/scope validation and intent/impact authorization;
- required Preview and mismatched, stale, or expired plan rejection;
- idempotent replay and conflicting-key rejection;
- mutation, required audit, and task insertion transaction outcomes;
- shutdown/restart persistence and worker consumption;
- CLI, HTTP, and MCP projection of the same Action contract.

A direct Handler test does not prove Runtime ordering. Exercise the assembled
Runtime for the critical path.

## Race And Repeat

```bash
GOWORK=off go test -race ./...
GOWORK=off go test -shuffle=on -count=20 ./...
```

Handlers, repositories, callbacks, and retained services are concurrent-use
contracts. Use synchronization barriers for ordering; timeouts are deadlock
guards, not performance assertions.

## Published Release Consumption

Remove local replacements and resolve the exact published tag in a fresh module
or copied checkout. A source checkout proves engineering acceptance; a remote
module resolution gate proves distribution.
