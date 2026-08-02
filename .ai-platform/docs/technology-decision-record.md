# Current Technology Decisions

- Version: 7.0
- Status: Confirmed
- Last updated: 2026-08-02

The active component, Profile, Starter, Admin, optional durable-runtime, and
compatibility decisions are recorded in
`.ai-platform/specs/007-component-framework-refoundation/plan.md` and the
React Admin delivery decision is recorded in
`.ai-platform/specs/008-react-admin-starter/plan.md`.

`module` and `appkit` form a database-free Core. Optional capabilities remain
ordinary packages composed through typed Module registrations. Profiles are
create-time source presets, not hidden runtime modes. `modary new` creates only
new projects and never patches handwritten business source.

The API Profile uses `net/http` and requires no external service. The Admin
Profile uses optional PostgreSQL, Identity, RBAC, sessions, and React 19 source.
The Governed Profile retains the accepted PostgreSQL transaction, River task,
SQL Audit, Action, CLI, HTTP, and MCP contracts without making them Core
requirements.

The accepted Alpha 3 storage and transaction decisions remain recorded in
`.ai-platform/specs/005-postgres-task-runtime/` and
`docs/adr/ADR-003-postgresql-and-module-migrations.md`. Release and immutable
tag decisions remain recorded in `.ai-platform/specs/006-postgres-alpha-release/`.
Those artifacts govern the published release they describe and do not override
the active v0.2 product boundary.
