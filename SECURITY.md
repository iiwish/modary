# Security Policy

- Private reporting channel: https://github.com/iiwish/modary/security/advisories/new

## Supported Versions

`v0.1.0-alpha.3` is the supported PostgreSQL and durable-task Alpha line. The
support table in `docs/reference/support-matrix.md` and its release notes define
the runtime boundary. Pre-v1 releases may receive a security fix as a new
prerelease or patch, but are not promised indefinite backports.

## Reporting A Vulnerability

Use the private GitHub Security Advisory link above. Do not
open a public issue containing an undisclosed vulnerability, credentials,
tokens, private data, or a working exploit. If the selected host has no private
reporting channel, contact the repository owner privately before sharing
details.

A useful report contains:

- affected package, version or commit, and platform;
- the violated security boundary and realistic impact;
- minimal reproduction steps with secrets removed;
- whether the issue requires local access, the same operating-system identity,
  an authenticated actor, or a malicious trusted callback;
- any proposed mitigation.

Maintainers should acknowledge a complete report, establish an embargo channel,
classify affected versions, develop a focused regression, and coordinate an
advisory and fixed release. No response-time service-level agreement is promised
before a maintained release line and contact channel are published.

## F0 Security Boundary

The supported F0 profile uses one PostgreSQL control database, an owned
application schema, and an owned River queue schema. Database credentials are
deployment secrets. Grant the application role only the database and schema
privileges required by Modary and consumer migrations.
Modary governs authorization, Preview/Execute plan binding, idempotency,
transaction boundaries, and required audit behavior. It is not a process,
container, operating-system, or hostile-extension sandbox.

The following remain trusted inputs or deployment responsibilities:

- consumer module code, callbacks, handlers, output writers, and migrations;
- the selected Go executable, toolchain, consumer source, and inherited
  environment not explicitly normalized by project tooling;
- operating-system identity, filesystem ownership, mounts, and host isolation;
- local identity provisioning, credentials, authorization policy, TLS, proxy,
  network exposure, backups, and secret rotation;
- task handler errors, which River persists as job history and therefore must be
  stable and secret-safe rather than wrapped dependency detail;
- cancellation cooperation by callbacks and writers.

Read `docs/operations/security.md` and `docs/f0-known-limitations.md` before any
deployment. Public-internet IAM, hostile plugin isolation, arbitrary storage
adapters, high availability, and distributed transactions are outside F0.
