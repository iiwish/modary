# T031 Test Results

- Result: Passed
- Date: 2026-08-02

- RED: Store authority tests failed because `database.Store` and
  `Control.Store` did not exist; the Admin Profile was unavailable.
- Focused behavior: database control, Module/AppKit assembly, general
  PostgreSQL, local Identity, RBAC, session HTTP, and Starter tests passed.
- Copied-out Admin: `GOWORK=off` tidy, dependency inspection, tests, and build
  passed against real PostgreSQL.
- CRUD: login, current session, CSRF denial, RBAC allow/default-deny,
  scope-isolated create/list/update/delete, restart persistence, and final
  deletion passed.
- Absence: generated source contains no governed PostgreSQL, River, SQL Audit,
  Action, Audit, task, MCP, or Action route selection; the dependency graph has
  no River package; the real database contains no selected queue schema.
- Authority: Store writes outside a callback transaction fail; governed Access
  has no transaction-opening method; commit, rollback, and raw database objects
  remain private.
- Race: focused race tests for database, Module, AppKit, postgresdb,
  sessionhttp, and Starter passed.
- Vet: all focused affected packages passed.
- Full repository: `go test ./... -count=1` passed.
- Documentation, link, strict T031 artifact, formatting, diff, and tidy gates
  passed.
