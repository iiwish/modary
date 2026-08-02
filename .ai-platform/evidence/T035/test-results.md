# T035 Test Results

- Result: Passed
- Date: 2026-08-02

## TDD

- RED: `go test ./starter -run TestCreateAdminProfileBuildsAndRunsScopedCRUDWithoutRiver -count=1` failed for the intended reasons: missing `App.tsx` and `main.tsx`, missing React dependencies, retained Vue packages, and eight generated `.vue` files.
- GREEN: the same focused generator test passed after the React replacement and generated root assertion update.
- REFACTOR: provider, hook, registry, protected-route, component, and API tests stayed green after type and accessibility cleanup.

## Fresh Validation

- `pnpm install --frozen-lockfile`: passed with pnpm 11.1.3.
- `pnpm lint`: passed with zero warnings.
- `pnpm typecheck`: passed under TypeScript strict mode.
- `pnpm test`: 7 files and 10 tests passed.
- `pnpm build`: passed; Vite transformed 1,805 modules and emitted `index.html`, `assets/app.css`, and `assets/app.js`.
- `pnpm assets:check`: passed byte-for-byte deterministic asset comparison.
- `go test ./starter ./transport/sessionhttp -count=1`: passed, including generated Admin real-PostgreSQL acceptance.
- Active Admin source residue scan: no `.vue`, Vue package, Pinia, or Vue Router implementation remains.
- `git diff --check`: passed.

The emitted bundle is 252.37 kB JavaScript (79.99 kB gzip) and 14.07 kB CSS
(3.92 kB gzip), with no component library or general-purpose state library.
