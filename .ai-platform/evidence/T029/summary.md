# T029 Lightweight Core And Component Surfaces

- Status: Completed
- Date: 2026-08-02

Core application assembly is database-free. `module.Host` constructs the
governed Action Runtime only when at least one selected Module declares an
Action. Applications without Actions start with an empty catalog and nil
Runtime, while optional identity and task facades retain their explicit
availability contracts.

The public consumer test starts a lifecycle-managed feature Module, mounts the
standard health handler and a consumer-owned HTTP route, serves both, and shuts
down without PostgreSQL, Action, Identity, RBAC, Audit, River, MCP, or Node.js.
Existing governed applications remain fail closed: an Action declaration still
requires authorization, audit, and complete Action persistence.

Lifecycle descriptions now describe a modular application with an optional
governed Runtime. The documentation-check fixture includes the feature 007
canonical artifacts so repository-wide tests validate the active contract.
