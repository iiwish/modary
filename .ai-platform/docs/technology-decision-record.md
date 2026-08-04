# Current Technology Decisions

- Version: 8.0
- Status: Confirmed
- Last updated: 2026-08-04

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
the active v0.3 product boundary.

## Production Foundation Decisions

The confirmed v0.3 decisions, alternatives, risks, and validation consequences
are recorded in `.ai-platform/specs/010-production-foundation/plan.md`.

- Principal identity is independent from product scope. RBAC bindings, not the
  identity record, grant access to exact scopes.
- Password login, session storage, and provider-specific browser login are
  distinct contracts. Local Identity remains a development adapter.
- Generic OIDC is an independently versioned component module using maintained
  OIDC/OAuth2 libraries. Modary does not implement the protocol primitives or
  provider-owned MFA and recovery.
- Process lifecycle, probes, migration commands, and structured logging use
  root packages limited to the Go standard library and existing lightweight
  contracts.
- OpenTelemetry SDK/exporters live in an independently versioned optional
  module. Generated composition selects it explicitly; no global provider is a
  hidden framework dependency.
- Deployment artifacts are generated consumer source. OCI is the executable
  baseline; Compose is local infrastructure; Kubernetes remains a documented
  example rather than a framework runtime requirement.
