# T035 React Platform And Architecture Migration

- Status: Completed
- Date: 2026-08-02
- Packet: `.ai-platform/specs/008-react-admin-starter/packets/T035.yaml`
- Execution: direct Codex execution; subagent delegation was not authorized

The optional Admin frontend is a React 19, TypeScript, Vite, React Router, and
pnpm application. Vue, Pinia, Vue Router, their compiler/test packages, every
`.vue` file, and Vue-specific configuration were removed rather than wrapped.

React context and focused hooks own application metadata, authentication,
toasts, and example-record state. The explicit typed module registry remains
the route and navigation composition boundary. Backend HTTP, same-site session,
CSRF, RBAC, PostgreSQL, Profile, embedding, and create-only ownership contracts
are unchanged.

The production bundle uses stable asset names and matches a fresh build byte for
byte. Generated Admin acceptance asserts React source and dependencies, rejects
Vue artifacts, embeds the React root, and passes the real-PostgreSQL backend
workflow.
