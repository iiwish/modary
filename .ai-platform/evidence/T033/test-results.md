# T033 Test Results

- Result: Passed
- Date: 2026-08-02

- Starter: full package tests passed, including create-only safety, deterministic
  output, generated file inventory, selected-component inspection, dependency
  graph inspection, real-PostgreSQL integration, and both binary builds.
- Copied out: `GOWORK=off` tidy, tests, vet, and builds passed outside the
  framework checkout for the generated application and worker.
- Governed behavior: required Preview, exact plan-bound Execute, optimistic
  state, unbound-actor default deny, stable idempotent replay, detailed SQL
  Audit, persisted restart Preview, and one decoded post-restart River task
  passed.
- Transaction boundary: the generated Action resolves governed
  `database.Access` and `task.Service`; mutation and enqueue run only inside the
  Runtime-owned transaction. The generated worker uses `task.Runner` and never
  imports River types.
- Regression: focused PostgreSQL Adapter, Action Runtime, HTTP/MCP, AppCmd, and
  Starter tests passed. The existing Counter runtime/CLI/HTTP/MCP/restart and
  transactional-task conformance tests passed.
- Race: Starter, governed PostgreSQL, Action Runtime, and HTTP/MCP transport
  race tests passed.
- Vet: affected framework packages, command, generated project, and generated
  worker passed.
- Absence: API and Admin tests continue to prove no River or governed adapter
  package in their dependency graph; Governed contains no Admin UI,
  `postgresdb`, `sessionhttp`, or records slice.
- Documentation checker, strict T033 artifact validation, diff, formatting, and
  whitespace gates passed.
