# Modary v0.2 F0 Acceptance Report

- Acceptance object: componentized Modary Go framework and three Starter Profiles
- Governing contracts: `.ai-platform/specs/007-component-framework-refoundation/spec.md`,
  `.ai-platform/specs/008-react-admin-starter/spec.md`, and
  `.ai-platform/specs/009-component-boundary-closure/spec.md`
- Date: 2026-08-04
- Status: Accepted
- Accepted source: 3600a38345380401f36958970f82cc93e2c29cd2
- Consumption status: Tagged Go modules and copied-out source
- Distribution status: Released
- Target version: v0.2.0-alpha.1
- Version tags: v0.2.0-alpha.1, components/postgres/v0.2.0-alpha.1, components/governedpostgres/v0.2.0-alpha.1
- Frozen baseline tag: v0.1.0-alpha.3
- License: Apache-2.0

## Verdict

The v0.2 F0 source is technically accepted as a lightweight componentized Go
backend framework. Core starts without a database or governance stack. API,
Admin, and Governed Profiles select materially different dependency graphs and
produce visible consumer-owned composition. Ordinary Admin CRUD is independent
from River and governed Actions; the Governed Profile retains the strict
Preview, transaction, audit, idempotency, and durable-task path.

The accepted source is published as `v0.2.0-alpha.1`. Hosted main and tag CI,
normal module resolution, and local and hosted copied-out consumers verify the
same immutable commit across the root and both component modules.

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
Validation disables Go work-file discovery. Local pre-release acceptance binds
the candidate source explicitly; published acceptance resolves all three module
tags through a normal Go proxy with no local replacement. The API project runs
without infrastructure; the Admin and Governed projects pass real PostgreSQL
integration. Admin frontend validation uses a frozen pnpm install and checks
lint, types, tests, production build, and byte-for-byte embedded asset parity.

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
Governed-first contract. Alpha 1 passed clean candidate validation, hosted main
and tag CI, normal Go module resolution, and copied-out remote consumer
verification. Its root and component tags all resolve to accepted commit
`3600a38345380401f36958970f82cc93e2c29cd2`. Future releases repeat those gates
and never move a published tag.
