# Test A Modary Application

Test the consumer at four levels: pure Definition, module behavior, governed
Runtime, and independent release consumption. A direct Handler unit test is
useful but does not prove authorization, plans, idempotency, transaction, or
audit behavior.

## Pure Definition

Call the Definition provider and verify metadata, module IDs, capabilities,
Action descriptors, and migrations without starting the application. Assert
that inspection does not create files, open the database, hash passwords,
construct handlers, or invoke Start callbacks.

Run the project verifier and generated drift check:

```bash
GOWORK=off go run ./tools/modary verify
GOWORK=off go run ./tools/modary generate --check
GOWORK=off go run ./tools/modary check
```

## Module And Handler

Cover valid and invalid inputs, schema boundaries, stale versions, dependency
errors, cancellation, custom public errors, and concurrent calls. Migrations
need fresh-database, restart, applied-history, and invalid-source tests.

Handler tests should prove `Plan` describes concrete impact and `Execute`
rechecks mutable state. They must not treat the plan payload as caller-editable.

## Governed Runtime

Assemble the real consumer profile and exercise:

- actor and execution-scope validation;
- intent and impact authorization denial;
- required Preview and mismatched or expired plan rejection;
- idempotent replay and conflicting key rejection;
- transaction rollback on Handler, audit, or persistence failure;
- successful result, audit, and restart durability;
- shutdown rejection and cancellation cooperation.

Use public packages only. This is the most important proof that the application's
business behavior actually uses Modary governance.

## Transport Contract

Exercise CLI, HTTP, and MCP with the same Action. Verify missing input, explicit
`null`, malformed JSON, unauthenticated calls, CSRF/cookie behavior, public
error mapping, request budgets, and successful Preview/Execute projection.

## Race And Repetition

Handlers and retained services are concurrency-safe contracts. Run focused race
tests for lifecycle, identity changes, planning, execution, shutdown, and
callbacks:

```bash
GOWORK=off go test -race ./...
GOWORK=off go test -shuffle=on -count=20 ./...
```

Use synchronization barriers for ordering. Timeouts are deadlock guards, not
performance assertions.

## Independent Release Consumption

Consumer CI must disable Go work files and must not rely on a committed local
replacement. After a Modary tag is published, test a copied checkout or fresh
module download with the exact version. The framework's remote-consumer gate
implements this final proof.
