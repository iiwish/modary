# T034 External Acceptance

- Result: Passed
- Date: 2026-08-02

## Isolation

The final Profile acceptance used three fresh directories outside the Modary
checkout: `/tmp/modary-t034.KLdRYy/t034-api`, `t034-admin`, and
`t034-governed`. `modary new` wrote `v0.2.0-alpha.1`; a deliberate local
`replace` bound the unreleased candidate. Every Go command used `GOWORK=off`.
No generated project copied a Modary implementation package.

## Results

| Profile | External result | Selected proof | Absence proof |
|---|---|---|---|
| API | Passed | create, tidy, test, vet, build, process health/ping/drain | no database config, PostgreSQL adapter, River, Identity adapter, RBAC adapter, SQL Audit, governed route, worker, or UI |
| Admin | Passed | real PostgreSQL Store, migrations, scoped RBAC CRUD/restart, session/CSRF, embedded Vue, frozen frontend pipeline | no governed PostgreSQL adapter, River schema/library/worker, SQL Audit, Action route, or MCP route |
| Governed | Passed | real PostgreSQL/River, Preview/Execute, default deny, idempotency, SQL Audit, CLI/HTTP/MCP, restart and worker consumption | no `postgresdb`, sessionhttp, records slice, or Admin frontend |

Common typed Core contract packages may appear transitively without installing
a service. Concrete adapter and infrastructure selection, runtime behavior, and
operational surfaces match the Profile tables.

The independently copied-out Counter consumer also passed full test, vet,
project verification, deterministic generated checks, build through both
source and copied project tools, runtime/CLI/HTTP/MCP parity, restart, and
transactional task consumption.

## Admin Source Pipeline

Inside the copied-out Admin project:

- frozen install: passed, 298 packages reused from the pinned lockfile;
- lint and typecheck: passed with zero warnings/errors;
- unit tests: 6 files and 8 tests passed;
- production build: 1,812 modules transformed;
- asset parity: generated `index.html`, `app.css`, and `app.js` were
  byte-identical to the committed embedded bundle.

## Adoption Boundary

Rulary source was not opened or modified. `docs/guides/rulary-bootstrap.md`
describes Rulary as an external consumer: create an Admin Profile in the Rulary
repository, replace the records example with product Modules, measure the first
rules vertical slice, then add governed publication/deployment operations only
where product risk justifies them.

## Release Boundary

This proves source consumption, not remote distribution. No v0.2 tag was
created or pushed. `v0.1.0-alpha.3` retained its original tag object, commit,
and tree. Publication remains a separate clean-candidate and owner-approval
gate.
