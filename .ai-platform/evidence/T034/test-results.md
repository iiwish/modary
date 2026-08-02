# T034 Test Results

- Result: Passed
- Date: 2026-08-02

## Repository Gates

- `make acceptance`: passed formatting, tidy, source diff, canonical docs and
  links, full framework tests, copied-out Counter tests, panic-nil mode, vet,
  generated state, neutrality, builds, and six-target cross-build.
- `make race`: passed full framework and copied-out Counter race suites.
- `make repeat`: passed 20 shuffled repetitions of the risk-selected framework
  packages and the complete Counter consumer.
- `make fuzz-smoke`: passed manifest, Action JSON, Action schema, protocol JSON,
  and Darwin filesystem-policy fuzz smoke gates.
- `make cross-build`: passed Linux amd64/arm64, Darwin amd64/arm64, and Windows
  amd64/arm64 builds plus the configured cross-compiled test binaries.
- `make format-check`, `make tidy-check`, `./scripts/check-neutrality.sh`,
  `./scripts/check-source-diff.sh`, and `git diff --check`: passed after final
  evidence updates.

## Profile Acceptance

Fresh CLI output was created under `/tmp/modary-t034.KLdRYy` with the current
source explicitly bound and work-file discovery disabled.

- API: `go mod tidy`, `go test ./... -count=1`, `go vet ./...`, and
  `go build ./...` passed. Existing Starter process acceptance also proved
  `/healthz`, `/api/ping`, signal drain, and shutdown. PostgreSQL, River,
  localidentity, RBAC, sqlaudit, and governed adapter packages were absent.
- Admin: the same Go gates passed against PostgreSQL 17 at isolated schema
  `t034_admin`. Copied-out `pnpm install --frozen-lockfile`, lint, typecheck,
  six-file/eight-test unit suite, production build, and byte-identical
  `assets:check` passed. River, governed PostgreSQL, and sqlaudit were absent;
  the fresh database had no queue schema.
- Governed: the same Go gates passed against isolated schemas `t034_governed`
  and `t034_governed_queue`. PostgreSQL/River, sqlaudit, Action, and task
  packages were present; `postgresdb`, sessionhttp, Admin UI, and records source
  were absent. Integration covered Preview, Execute, default deny, replay,
  audit, restart, and task consumption.

Core intentionally shares small provider-neutral Action and task contract
packages. Absence claims apply to unselected concrete adapters, infrastructure
libraries, initialization, routes, migrations, configuration, processes, and
UI rather than to every leaf contract package.

## Frontend And Visual QA

The canonical Admin source passed the same frozen install, lint, typecheck,
unit, production build, and asset-parity pipeline. T032 browser evidence remains
valid for the accepted UI source: authenticated desktop and mobile CRUD,
desktop empty state, dialog keyboard/focus behavior, no horizontal overflow,
and no browser console error.

Evidence images:

- `.ai-platform/evidence/T032/login-desktop.png`
- `.ai-platform/evidence/T032/records-desktop.png`
- `.ai-platform/evidence/T032/records-desktop-empty.png`
- `.ai-platform/evidence/T032/records-mobile.png`

## Documentation And Delivery

- `./scripts/check-docs.sh`: passed canonical files, state/evidence parity,
  release-boundary statements, and active-surface checks.
- `./scripts/check-doc-links.sh`: passed document inventory, relative links,
  and English/Chinese navigation.
- `go test ./scripts ./cmd/modary -count=1`: passed docs, neutrality, Makefile,
  and create-only command regression tests without leaving a source-tree binary.
- strict T034 delivery-artifact validation: passed.
- Alpha 3 tag object `55e0a5b0d7b8a8422f4e9bd2e504b7d61d50d9c0`
  still points to accepted commit `f39457a52c10ceecd8defb77e0def1b331c45dd2`
  and tree `84722726fa592f04e31596b873445ce566a2f857`.

Toolchain: Go 1.26.3 darwin/arm64, Node.js 24.15.0, pnpm 11.1.3, PostgreSQL
17 Alpine acceptance container.
