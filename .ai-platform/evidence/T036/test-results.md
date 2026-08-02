# T036 Test Results

- Result: Passed
- Date: 2026-08-02

## TDD

- RED: focused React tests failed because a 401 from an authenticated API call
  left the protected page visible and a 403 produced a generic retry banner
  instead of a dedicated forbidden state.
- GREEN: the API client notifies the Auth provider of expired sessions, clears
  actor and CSRF state, and protected routing returns to login; records retain
  typed error status and render an `Access denied` state for 403.
- RED: generated Admin acceptance failed because `/assets/app.js` returned
  `Cache-Control: public, max-age=31536000, immutable` under a fixed filename.
- GREEN: fixed assets return `Cache-Control: no-cache` and retain their ETag.
- REFACTOR: complete visible CRUD, filter, toast, focus-return, Strict Mode, and
  protected-route tests were added while all gates stayed green.

## Frontend

- `pnpm install --frozen-lockfile`: passed with pnpm 11.1.3.
- `pnpm lint`: passed with zero warnings.
- `pnpm typecheck`: passed under strict TypeScript.
- `pnpm test`: 7 files and 14 tests passed.
- Covered behavior: session restoration, unauthenticated redirect, login return,
  session expiry, CSRF, registry absence, record store CRUD, visible CRUD,
  filters, forbidden state, empty state, dialogs, focus restoration, mobile
  inert/focus behavior, Strict Mode, and axe structural checks.
- `pnpm build`: passed and produced 253.00 kB JS (80.17 kB gzip) and
  14.15 kB CSS (3.93 kB gzip).
- `pnpm assets:check`: passed byte-for-byte asset comparison.

## Go And Browser

- `go test ./starter ./transport/sessionhttp -count=1`: passed, including real-
  PostgreSQL generated Admin acceptance and the asset-cache regression test.
- Desktop 1440 x 900: sign-in, create, edit dialog focus/Escape, record table,
  both row commands, sign-out surface, zero horizontal overflow, and no browser
  warning/error passed.
- Mobile 390 x 844: card layout, both row commands, open/close navigation,
  inert hidden navigation, focus transfer and restoration, bottom-sheet delete
  confirmation, empty state, toast, zero horizontal overflow, and no browser
  warning/error passed.
- Live asset response: JavaScript returned `200`, JavaScript content type,
  `Cache-Control: no-cache`, and a SHA-256 ETag.
- `git diff --check`: passed.
