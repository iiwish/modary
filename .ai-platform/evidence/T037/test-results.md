# T037 Test Results

- Result: Passed
- Date: 2026-08-02

All declared T037 repository, frontend, copied-out consumer, browser, security,
and release-readiness gates pass.

## Repository Gates

- `make acceptance` with Go 1.26.5 passed format, tidy, source-diff, docs, link,
  React residue, frozen frontend, production audit, complete Go tests, copied-out
  Counter, panic-nil, vet, reachable vulnerability, generated-state, neutrality,
  build, and cross-build gates.
- `make race` passed the framework and copied-out Counter race suites.
- `make repeat` passed 20 shuffled repetitions of risk-selected framework
  packages and the complete Counter consumer.
- `make fuzz-smoke` passed manifest, Action JSON, Action schema, protocol JSON,
  and filesystem-policy fuzz targets.
- `make cross-build` passed Linux amd64/arm64, Darwin amd64/arm64, and Windows
  amd64/arm64 builds plus configured cross-compiled test binaries.
- `govulncheck v1.6.0 ./...` passed for Modary and the copied-out Counter with
  zero reachable vulnerabilities under Go 1.26.5.

## Frontend Gates

- `pnpm install --frozen-lockfile`: passed with pnpm 11.1.3.
- `pnpm lint` and `pnpm typecheck`: passed with zero warnings or type errors.
- `pnpm test`: 8 files and 19 tests passed.
- `pnpm build`: passed with 1,855 modules;
  `assets/app-dQlnqaee.js` is 251.90 kB (79.76 kB gzip) and
  `assets/app-BNbZ7QRR.css` is 14.20 kB (3.95 kB gzip).
- `pnpm assets:check`: passed byte-for-byte embedded asset comparison.
- `pnpm audit:prod`: passed with no known production dependency vulnerability.
- `./scripts/check-react-admin.sh`: passed current template, lockfile, generated
  source, embedded runtime, and canonical-document Vue absence checks.

## Copied-Out Consumer

The final independent consumer is
`/tmp/modary-react-t037-final.1qfC4q/react-admin`.

- Generation used `v0.2.0-alpha.1`, an explicit local source replacement, and
  the Admin Profile; generated `go.mod` requires Go 1.26.5.
- Frozen frontend install, lint, strict typecheck, 19 tests, production build,
  asset parity, and production audit passed.
- `GOWORK=off` tidy, two consecutive real PostgreSQL 17.10 integration runs
  against database `modary_react_final_20260802_2008` and schema
  `react_admin_live`, vet, and build passed. Integration covered content-hashed
  embedded React assets, session/CSRF, complete create/update response timestamps,
  scoped CRUD, default deny, shutdown, restart, persistence, fixture cleanup,
  and deletion.
- Source scan found no `.vue` file, Vue/Pinia package, Vue runtime, or
  `react-router-dom` compatibility dependency.

## Browser And Asset Checks

- The final copied-out Go binary served the React application on port 8084.
- Login, create, update, delete, successful logout, and re-login passed against
  the copied-out application.
- Mobile navigation passed at 390 by 844 CSS pixels with no horizontal overflow;
  the desktop work surface passed at the default 1280 by 720 viewport.
- The browser warning/error log was empty after the complete interaction flow.
- HTML uses revalidation while content-hashed JavaScript and CSS return
  `public, max-age=31536000, immutable`; the legacy `/assets/app.js` URL is absent.
- Automated axe checks retain contrast validation, and the raw palette test
  requires at least the WCAG AA 4.5:1 normal-text ratio.

## Release Metadata

The actual Modary worktree was committed before validation. `make
release-readiness VERSION=v0.2.0-alpha.1` passed the clean-worktree release
preflight and the complete CI gate on that committed candidate. Preflight covered
license, security channel, changelog, accepted F0, Go 1.26.5 baseline, module
path, canonical origin, docs, links, format, tidy, and candidate-tag boundaries.
No release tag was created.

Toolchain: Go 1.26.5 darwin/arm64, Node.js 24.15.0, pnpm 11.1.3, and
PostgreSQL 17.10 Alpine.
