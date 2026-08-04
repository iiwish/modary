# T042 Test Results

- Result: Passed
- Date: 2026-08-04

Passed gates:

- root identity, authz, module, appkit, session HTTP, governed HTTP/MCP, CLI,
  and Action Runtime tests;
- real PostgreSQL Local Identity and RBAC tests, including password-rotation
  freshness and one actor bound to two exact scopes with an unbound denial;
- all root, PostgreSQL, Governed PostgreSQL, integration, and Counter module
  tests;
- copied-out default Admin, operations Admin, schema-boundary Admin, and
  Governed Profile acceptance with real PostgreSQL;
- generated React install, lint, typecheck, tests, production builds, and asset
  parity for selected Admin variants;
- generated Profile drift and `git diff --check`.

The full `ci-core` wrapper reached its documentation gate and correctly
rejected the historical T041 source digest while v0.3 work is active. The final
T047 acceptance task owns the new release-wide digest and reruns the complete
wrapper after every v0.3 component is frozen.
