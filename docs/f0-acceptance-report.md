# Modary v0.2 F0 Acceptance Report

- Acceptance object: componentized Modary Go framework and three Starter Profiles
- Governing contracts: `.ai-platform/specs/007-component-framework-refoundation/spec.md`,
  `.ai-platform/specs/008-react-admin-starter/spec.md`, and
  `.ai-platform/specs/009-component-boundary-closure/spec.md`
- Date: 2026-08-03
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
| Admin backend | Accepted | Ordinary PostgreSQL Store, local Identity, explicit session capability, scoped RBAC, contribution preflight, cross-component schema-role exclusion, CRUD and restart, no default River/Action/Audit/MCP |
| Admin UI | Accepted | React source, permission-aware exact icon/module registry, precision-safe task/audit cursors, records CRUD plus selectable inspection, session expiry, responsive/accessibility QA, deterministic selected assets |
| Governed Profile | Accepted | Preview/Execute/replay/restart/audit/task consumption through CLI, HTTP, MCP, and worker |
| Component boundaries | Accepted | Root Core, ordinary PostgreSQL, and governed PostgreSQL/River are separate Go modules; omitted concrete components are absent from generated module graphs |
| Architecture | Accepted | Public import direction, explicit composition, no Rulary product code in framework, optional advanced project tooling |
| Repository quality | Accepted | Full tests, race, vet, formatting, tidy, curated repeat, cross-build, generated, documentation, neutrality, source digest, and diff gates |
| Product review | Accepted | Profiles solve the lightweight-starting-point problem with no unresolved P0 through P2 finding |
| Engineering review | Accepted | Current implementation and external acceptance have no unresolved P0 through P2 finding |

## External Acceptance

All Profiles are generated into temporary directories outside the repository.
Validation disables Go work-file discovery and binds the candidate source only
for local pre-release testing. The API project runs without infrastructure; the
Admin and Governed projects pass real PostgreSQL integration. Admin frontend
validation uses a frozen pnpm install and checks lint, types, tests, production
build, and byte-for-byte embedded asset parity.

T041 records a digest over the complete candidate source outside its own evidence
directory. Documentation and release preflight recompute it, so an implementation,
automation, generated-asset, or canonical-document change invalidates acceptance
until the evidence and review are refreshed.

The Profile-wide refoundation evidence is recorded in
`.ai-platform/evidence/T034/` and `.ai-platform/evidence/T037/`. Current
component isolation, contribution contracts, task/audit Admin surfaces,
copied-out consumers, and browser acceptance are recorded in
`.ai-platform/evidence/T038/` through `.ai-platform/evidence/T041/`.

## Release Boundary

`v0.1.0-alpha.3` remains immutable and continues to describe its historical
Governed-first contract. The v0.2 work does not move or rewrite that tag. A
future `v0.2.0-alpha.1` release must publish one immutable coordinated tag train:
`v0.2.0-alpha.1`, `components/postgres/v0.2.0-alpha.1`, and
`components/governedpostgres/v0.2.0-alpha.1`, all at the same commit. The
documented release process and normal remote consumer verification remain
mandatory; until then consumers evaluate v0.2 from a pinned source checkout.
