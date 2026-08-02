# Security Policy

- Private reporting channel: https://github.com/iiwish/modary/security/advisories/new

## Supported Versions

`v0.1.0-alpha.3` is the immutable published Alpha baseline. The current branch
targets `v0.2.0-alpha.1` and has source-level F0 acceptance but is not a
published support line. Pre-v1 fixes may be delivered as a new exact prerelease;
indefinite backports are not promised.

## Report A Vulnerability

Use the private GitHub Security Advisory link. Do not open a public issue with
an undisclosed vulnerability, credentials, private data, or a working exploit.

Include affected version or commit, package and platform, violated boundary,
realistic impact, minimal secret-free reproduction, and required attacker
position. State whether exploitation needs local process access, the same OS
identity, an authenticated actor, or a malicious trusted callback.

Maintainers should acknowledge a complete report, establish an embargo channel,
classify affected versions, add a focused regression, and coordinate advisory
and fixed release. No response-time SLA is promised before a maintained stable
line is published.

## F0 Boundaries

- Core is database-free and runs trusted consumer Modules in one Go process.
- Admin adds session/CSRF, backend RBAC, ordinary PostgreSQL Store transactions,
  and generated consumer-owned React assets.
- Governed adds Preview/Execute, idempotency, SQL Audit, PostgreSQL transaction
  ownership, River tasks, CLI/HTTP, and MCP.

Modary is not an operating-system sandbox, public-internet IAM system,
distributed transaction coordinator, database operator, or deployment security
boundary. Consumer code, callbacks, migrations, toolchain, environment, secrets,
TLS/proxy policy, database operations, task error safety, backup, and
cancellation cooperation remain trusted inputs or application responsibilities.

Read [Security Boundaries](docs/operations/security.md) and
[Known Limitations](docs/f0-known-limitations.md) before deployment.
