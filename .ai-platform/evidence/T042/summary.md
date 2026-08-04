# T042 Delivery Summary

- Status: Completed
- Task: Principal, Scope, And Session Boundary
- Date: 2026-08-04

`identity.Actor` contains principal identity only. Execution scope is supplied
by consumer HTTP, MCP, CLI, Admin, and business composition and is evaluated by
exact default-deny RBAC bindings. Password verification, session issuance and
resolution, bearer authentication, and browser password login are explicit
independent contracts and capabilities.

Local Identity uses a forward PostgreSQL migration, preserves hashed
credentials and revocable sessions, and carries a bounded credential freshness
value from password verification to session issuance so rotation cannot race a
stale verification into a valid session. Copied-out Admin and Governed Profiles
exercise the new boundary.
