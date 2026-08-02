# Modary v0.2 F0 Acceptance Report

- Acceptance object: componentized Modary Go framework and three Starter Profiles
- Governing contracts: `.ai-platform/specs/007-component-framework-refoundation/spec.md`
  and `.ai-platform/specs/008-react-admin-starter/spec.md`
- Date: 2026-08-02
- Status: Accepted
- Distribution status: Not released
- Target version: v0.2.0-alpha.1
- Frozen baseline tag: v0.1.0-alpha.3
- License: Apache-2.0

## Verdict

The v0.2 F0 source is technically accepted as a lightweight componentized Go
backend framework. Core starts without a database or governance stack. API,
Admin, and Governed Profiles select materially different dependency graphs and
produce visible consumer-owned composition. Ordinary Admin CRUD is independent
from River and governed Actions; the Governed Profile retains the strict
Preview, transaction, audit, idempotency, and durable-task path.

Technical acceptance does not claim publication. The v0.2 candidate still
requires a clean committed source candidate, owner release approval, immutable
tag, hosted tag CI, and normal remote module verification.

## Acceptance Matrix

| Area | Result | Proved behavior |
|---|---|---|
| Core | Accepted | Database-free Module graph, capabilities, lifecycle, health, explicit routes, absent optional facades |
| API Profile | Accepted | Create-only external project, `GOWORK=off` test/build/start/shutdown, no optional infrastructure imports |
| Admin backend | Accepted | Ordinary PostgreSQL Store, local Identity, scoped RBAC, session/CSRF, CRUD and restart, no River/Action/Audit/MCP |
| Admin UI | Accepted | React source, explicit module registry, session expiry, login and CRUD states, responsive/accessibility QA, deterministic embedded assets |
| Governed Profile | Accepted | Preview/Execute/replay/restart/audit/task consumption through CLI, HTTP, MCP, and worker |
| Component boundaries | Accepted | Separate ordinary and governed database authority; omitted concrete adapters and infrastructure libraries absent from generated graphs |
| Architecture | Accepted | Public import direction, explicit composition, no Rulary product code in framework, optional advanced project tooling |
| Repository quality | Accepted | Full tests, race, vet, formatting, tidy, repeat, cross-build, generated, documentation, neutrality, and diff gates |
| Product review | Accepted | Profiles solve the lightweight-starting-point problem with no unresolved P0 through P2 finding |
| Engineering review | Accepted | Current implementation and external acceptance have no unresolved P0 through P2 finding |

## External Acceptance

All Profiles are generated into temporary directories outside the repository.
Validation disables Go work-file discovery and binds the candidate source only
for local pre-release testing. The API project runs without infrastructure; the
Admin and Governed projects pass real PostgreSQL integration. Admin frontend
validation uses a frozen pnpm install and checks lint, types, tests, production
build, and byte-for-byte embedded asset parity.

The Profile-wide commands and results are recorded in
`.ai-platform/evidence/T034/test-results.md` and
`.ai-platform/evidence/T034/external-acceptance.md`. The React Admin replacement,
copied-out frontend pipeline, generated Go application, and browser acceptance
are recorded in `.ai-platform/evidence/T037/`.

## Release Boundary

`v0.1.0-alpha.3` remains immutable and continues to describe its historical
Governed-first contract. The v0.2 work does not move or rewrite that tag. A
future `v0.2.0-alpha.1` release must be created from the accepted source through
the documented release process; until then consumers evaluate v0.2 from a
pinned source checkout.
