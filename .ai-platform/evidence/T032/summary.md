# T032 Admin UI F0

- Status: Completed
- Date: 2026-08-02

The optional Admin Profile includes a consumer-owned Vue 3, TypeScript, Vite,
Pinia, and Vue Router work surface. Its explicit frontend registry selects only
the records module. Authentication restoration, login, logout, filtering,
refresh, create, edit, optimistic update, delete, CSRF propagation, bounded
error presentation, and responsive navigation are implemented without adding
task, audit, governed Action, MCP, marketplace, or runtime-generated UI.

The canonical source produces byte-identical prebuilt assets. Project creation
copies both source and the production bundle; the Go binary embeds and serves
the bundle without a Node.js runtime. A copied-out project passed frontend and
Go build/test gates against real PostgreSQL.

Desktop and 390 x 844 mobile browser acceptance exercised session restoration,
the complete records workflow, editor and deletion Escape handling, mobile
navigation focus restoration, inert off-canvas navigation, zero horizontal
overflow, and both mobile row commands. Browser diagnostics contained no error
or warning. Evidence is recorded in `login-desktop.png`,
`records-desktop.png`, `records-desktop-empty.png`, and `records-mobile.png`.
