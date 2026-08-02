# T032 Test Results

- Result: Passed
- Date: 2026-08-02

- Frontend install: `pnpm install --frozen-lockfile` passed with pnpm 11.1.3.
- Frontend quality: ESLint passed with zero warnings; `vue-tsc --noEmit`
  passed; Vitest passed 6 files and 8 tests.
- Frontend behavior: registry absence, session restoration, login/logout, CSRF,
  records CRUD stores, view commands, structural axe checks, modal Escape, and
  mobile navigation focus/inert behavior passed.
- Production assets: Vite built fixed `index.html`, `assets/app.css`, and
  `assets/app.js`; deterministic asset comparison passed. The generated Admin
  source independently passed frozen install, lint, typecheck, tests, build,
  and asset comparison outside the repository checkout.
- Go integration: focused Starter and SPA tests, focused race tests, and vet
  passed. A copied-out Admin project passed real-PostgreSQL tests and Go build.
- Component absence: generated source, Go dependency graph, and frontend bundle
  contain no River, governed PostgreSQL, SQL Audit, Action, task, audit, MCP, or
  marketplace selection.
- Browser: Chromium at 1440 x 900 and 390 x 844 passed login, session restore,
  filter, create, edit, delete, editor/deletion Escape, mobile menu focus and
  Escape, both row commands, and zero-overflow checks; console warnings/errors
  were empty.
- Accessibility: semantic axe tests found no structural violation. Important
  small-text and control contrast pairs are at least 4.5:1; focus indicators,
  accessible icon names, native modal focus trapping, reduced motion, inert
  hidden navigation, and keyboard dismissal are implemented.
- Documentation checker, strict T032 artifact validation, generated diff, and
  whitespace gates passed.
