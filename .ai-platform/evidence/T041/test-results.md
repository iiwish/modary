# T041 Test Results

- Result: Passed
- Date: 2026-08-03

## Repository Gates

- `make ci` passed with Go 1.26.5 and PostgreSQL 17. It covers format, tidy,
  source diff, docs and links, React residue, frontend production audit, every Go
  module, panic-nil, vet, vulnerability, build and cross-build, race, repeat,
  fuzz smoke, generated state, neutrality, copied-out consumers, and final source
  immutability checks.
- The repeat contract uses `REPEAT_TIMEOUT=8m`: selected Core packages and
  create-only Starter contracts run 20 times, stateful PostgreSQL/component and
  integration contracts run 5 times, and focused copied-out consumer contracts
  run 10 times. Static and governance-only packages are not multiplied.
- Root `GOWORK=off go list -m all` contains no PostgreSQL driver, PostgreSQL
  component, pgx, or River module. Generated graphs distinguish API, default
  Admin, task-enabled Admin, and Governed selections by construction.
- `git diff --check`, strict T041 artifact validation, documentation checks, and
  the source-digest evidence check passed.
- Script regressions prove `MODARY_DOCS_ROOT=.` still executes the T041 digest
  check and rejects source drift. A Make-level behavior regression starts with a
  hostile external `MODARY_DOCS_ROOT` and proves the authoritative `docs-check`
  replaces it with the current checkout. CI runs the aggregate copied-profile
  target, and tagged release CI waits for that job.

## Generated Admin Acceptance

- Fresh default and operations Admin consumers passed `go mod tidy`, dependency
  graph assertions, required PostgreSQL integration, `go build ./...`, frozen
  pnpm installation, asset parity before rebuild, lint, typecheck, tests,
  production build, and asset parity after rebuild outside the checkout.
- Copied-out Admins with a valid 63-byte project ID and reserved-sensitive
  `public` and `pg` IDs passed required PostgreSQL/River integration without
  schema environment overrides, then their built binaries reached `/healthz`
  using runtime schema defaults.
- Unit and rendered-template tests cover the 63-byte PostgreSQL limit, River's
  46-byte limit, deterministic hashing, truncation collision resistance,
  role-namespace separation, reserved-schema avoidance, and independently
  bounded integration-test schemas for Admin and Governed.
- Invalid-input tests reject an exact or nested `vendor` Go Module Path segment
  before writing files, closing the Go vendoring import-path ambiguity.
- Generated integration compares ID, Chinese label, path, icon, order, complete
  permissions, required permissions, and grants for every selected Admin
  descriptor. Frontend selection tests independently require the corresponding
  source module IDs and exact icon keys.
- Each of `records.list`, `records.create`, `records.update`, and
  `records.delete` is revoked independently. The authenticated real route returns
  HTTP 403, `/api/admin/context` omits the revoked grant, permissions are restored,
  and the original record remains the only stored record.
- Task and audit endpoints cover authentication, bounded and invalid queries,
  empty results, restricted grants, and real HTTP 403 responses. Audit reads stay
  bound to the authenticated actor scope.

## Generated Governed Acceptance

- A fresh Governed consumer passes `go mod tidy`, dependency-graph assertions,
  `go test -count=1 ./...`, and both generated command builds outside the
  checkout with `GOWORK=off`.
- The required test event for
  `TestGovernedProfileCommitsAndConsumesDurableWork` reports `pass`; a missing or
  skipped PostgreSQL integration test fails the copied-profile gate.
- Unique application and queue schemas exercise Preview, Execute, default deny,
  idempotent replay, SQL Audit, restart recovery, River enqueue, and worker
  consumption. Queue schema validation rejects identifiers above River's
  46-byte notification-topic boundary before startup side effects; generated
  role-prefixed defaults satisfy that boundary and the reserved-schema policy
  before reaching component construction.

## React Admin

- Canonical frontend lint and typecheck passed; 12 files and 34 tests passed.
- Default, tasks, audit, and operations source selections built successfully and
  matched their checked-in embedded assets byte for byte.
- Tests cover permission-aware module resolution, duplicate fail-closed behavior,
  records commands, task/audit filters and pagination, latest-request ownership,
  session expiry, Chinese error mapping, loading/empty/failure/forbidden states,
  accessibility, and modal mobile navigation.
- Browser acceptance at 1440 by 1000 and 390 by 844 passed against a generated
  operations Admin. Records, Tasks, and Audit render in Chinese with exact icons;
  navigation focus, modal isolation, scroll locking, viewport width, and browser
  logs are clean. Unknown `/api` and `/healthz` paths remain HTTP 404 for every
  tested `Accept` shape while ordinary browser routes retain SPA fallback.

## Release Boundary

- On 2026-08-04 the complete candidate `make ci` gate passed again with Go
  1.26.5, Node.js 24.15.0, pnpm 11.1.3, and PostgreSQL 17. It repeated every
  repository, component, copied-profile, React, race, repeat, fuzz, build,
  vulnerability, neutrality, generated-state, and source-immutability check.
- The first invocation reached `govulncheck` after all preceding gates passed
  but the Go proxy timed out while resolving the pinned scanner. The pinned
  scanner was fetched through a fallback module proxy and the complete CI gate
  was restarted from the beginning; no scan or other gate was skipped.
- Candidate release preflight reaches the expected clean committed-worktree
  boundary before commit. Clean candidate release readiness then passed at
  `3600a38345380401f36958970f82cc93e2c29cd2`, including a complete second CI
  run and candidate-mode preflight.
- Fixture tests cover source-digest drift, component version alignment, the
  coordinated tag train, canonical origin, owner inputs, and remote-consumer
  replacement removal.
- Hosted main CI passed at
  https://github.com/iiwish/modary/actions/runs/30870905898. Hosted tag CI passed
  at https://github.com/iiwish/modary/actions/runs/30871605918, including tag
  preflight and copied-out remote consumption.
- The root, PostgreSQL, and Governed PostgreSQL annotated tag objects are
  `51b98be0c7809e3e97b88bb45e0514cbc452dc73`,
  `4c8d459192f3e8a176841442d837b3a3ac747cdd`, and
  `d3b5a7699b2e8ed56aa5fd5d32b0179cd3257134`. All peel to the accepted commit.
- Local remote consumption initially encountered a `proxy.golang.org` network
  timeout. The complete gate was retried through a fallback Go proxy and passed
  without a local replacement or work file. Normal module queries resolved the
  root and both component modules at exactly `v0.2.0-alpha.1`.
- The verified GitHub prerelease is
  https://github.com/iiwish/modary/releases/tag/v0.2.0-alpha.1.
