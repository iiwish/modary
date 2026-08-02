# T033 Governed Profile Optionalization

- Status: Completed
- Date: 2026-08-02

The create-only Starter exposes `governed` as a complete Profile. Its generated
composition visibly selects governed PostgreSQL, River, local development
Identity, RBAC, SQL Audit, a consumer-owned required-preview Action, CLI and
HTTP Action transports, MCP, and a dedicated durable worker. Choosing API or
Admin continues to omit every governed dependency.

The generated `limits.set` feature owns its schema, optimistic version, Action
contract, business mutation, durable `limits.changed` event, and task decoder.
The Runtime retains transaction authority: state mutation and River enqueue
occur inside the same framework-owned transaction. The worker consumes only
through `task.Runner` and presents an explicit idempotent product callback.

A copied-out project with `GOWORK=off` passed real-PostgreSQL Preview, RBAC
default-deny, Execute, idempotent replay, SQL Audit, shutdown/restart state and
plan recovery, and post-restart River task consumption. Both generated
binaries passed build and vet. Existing Runtime, PostgreSQL, HTTP/MCP, and
Counter conformance remained green.
