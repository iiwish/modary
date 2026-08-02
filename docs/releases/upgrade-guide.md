# Upgrade Guide: Alpha 3 To The v0.2 Component Model

The transition from `v0.1.0-alpha.3` to the v0.2 source target is intentionally
breaking. Alpha 3 treats PostgreSQL, River, governed Actions, and project-tool
structure as the primary onboarding path. v0.2 separates database-free Core,
selectable components, and three create-only Profiles.

Do not run an automated rewrite over an existing product. There is no Starter
patch command. Use a reviewable migration branch and move composition one
boundary at a time.

## 1. Classify The Application

Choose the closest v0.2 destination:

- API: no selected persistence or auth baseline;
- Admin: ordinary PostgreSQL CRUD, sessions, RBAC, optional React work surface;
- Governed: high-impact Preview/Execute, audit, idempotency, River, and worker;
- Mixed: start from Admin and compose governed components only for selected
  high-impact operations.

Existing Alpha 3 consumers usually map to Governed or Mixed. Do not force
ordinary CRUD through governed Actions merely to preserve the old structure.

## 2. Generate A Reference Project

From the candidate source, create the chosen Profile into a separate temporary
directory. Treat it as a readable reference, not as the destination for blind
file replacement.

```bash
go run ./cmd/modary new /tmp/reference --profile governed \
  --module example.com/reference --name "Reference Governed"
```

For local candidate work, bind the generated reference to the source checkout
with a temporary `replace`; release validation removes local replacements.

## 3. Move The Composition Root

Retain stable consumer Module IDs, Action IDs, permission IDs, schema names,
and persisted identities. Rebuild the application Definition using the current
explicit component composition shown by the generated Profile.

Remove assumptions that every application has:

- an Action Runtime;
- a River worker;
- SQL Audit;
- MCP routes;
- `modary.yaml` or generated Action catalogs.

Install only the components the product path needs.

## 4. Separate Ordinary And Governed Data Access

Ordinary repositories use `database.Store` and explicit
`WithinTransaction`. The official implementation is `adapters/postgresdb`.

Governed Action handlers continue to use `database.Access`; they cannot begin a
transaction. The governed Runtime owns the transaction that joins state,
idempotency, required audit, and task enqueue.

Do not adapt one interface to the other casually. Review every write path and
decide whether it is ordinary application work or a governed operation.

## 5. Recompose Identity And HTTP

Admin applications use `transport/sessionhttp` for login, session restoration,
logout, CSRF, and protected ordinary routes. Governed applications use the
Action transports selected by their composition.

Consumer routes remain explicit. Delete automatic mounting assumptions and
test route absence as well as route success.

## 6. Decide The Frontend Boundary

The Admin Profile includes consumer-owned React source and deterministic embedded
assets. Existing frontend applications may keep their own UI and consume only
the backend components. Do not copy the generated Admin UI unless it provides a
useful baseline.

When adopting it, replace the sample records module through the explicit
frontend module registry and preserve the frozen install, lint, type, test,
build, and asset-parity gates.

## 7. Migrate Tooling

Starter projects do not require `modary.yaml`. Keep `projecttool` only if the
product benefits from generated Module graphs, Action catalogs, TypeScript
contracts, or its constrained build workflow. Otherwise use ordinary Go build
and test commands.

## 8. Rehearse Data And Deployment

Create and verify a production-representative backup. Restore it into an
isolated environment, run the migrated application, inspect migrations, test
representative reads and writes, restart, and verify worker behavior when the
Governed component is selected.

Never rewrite an applied Alpha 3 migration. Add a forward consumer migration or
restore the matching old binary and database state on rollback.

## 9. Validate Externally

Run with work-file discovery disabled:

```bash
GOWORK=off go mod tidy
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
```

Admin consumers also run the pinned frontend pipeline and asset parity check.
Governed consumers test Preview, Execute, replay, restart, audit, and actual
task consumption. Inspect the final dependency graph to confirm omitted
components are absent.

## 10. Pin A Released Version

The v0.2 source target is not a remote version until its tag is published.
Production consumers must not invent that tag. After release, remove local
`replace` directives, pin the exact immutable version, repeat external
acceptance, and record the source commit and database backup in the consumer's
release evidence.
