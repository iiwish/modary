# Security Boundaries

Modary provides application-level governance around typed business Actions. It
does not replace operating-system isolation, network security, secure product
design, or review of consumer code.

## Governed Boundary

Every supported business mutation reaches `action.Runtime`, which validates the
request and actor, authorizes intent, binds Preview to concrete impact, checks
idempotency, owns the transaction, reauthorizes, validates Handler output, and
requires audit behavior. Transports must not retain or call raw Handlers.

Consumer modules and official adapters run in the same Go process. They are
trusted code, not sandboxed plugins. Reflection restrictions and sealed
capabilities prevent accidental authority leaks through public contracts; they
do not defend against arbitrary malicious code already executing as the same
operating-system identity.

## Identity And Authorization

The local identity adapter is appropriate for explicit local/private profiles.
Provision principals, optional password material, bearer tokens, roles,
bindings, permissions, scope, and row limits deliberately. A credentialless
service principal does not need a synthetic password. The adapters create no
default principal, policy, grant, or secret.

Do not expose the local profile to the public internet without a consumer-owned
security design covering TLS, account lifecycle, recovery, brute-force defense,
MFA or SSO requirements, proxy trust, session policy, rate limiting, monitoring,
and incident response.

## Bearer Tokens

Prefer standard input or the supported `--token-file <path>` boundary. Linux and
Darwin accept only a retained regular file owned by the effective UID with exact
mode `0400` or `0600`; Darwin also rejects extended ACLs. Other systems reject a
token path before filesystem access and retain only `--token-file -` standard
input support.

Never pass a bearer token as a command argument, where it may appear in process
listings or history. Do not log request authorization headers, session secrets,
passwords, or complete Action input by default.

## PostgreSQL Connections

Treat the PostgreSQL URL as a secret. Use a dedicated role and database. The
configured role must own the application and River schemas; do not use
`public`, a `pg_*` schema, or an unrelated shared schema. Restrict network
reachability, require TLS where traffic leaves a trusted host boundary, rotate
credentials, and monitor authentication and privilege changes.

Modary pins the application connection `search_path` and supplies River's schema
explicitly. It does not configure PostgreSQL TLS, disk encryption, row-level
security, roles, backups, or key management for the operator.

Business Connectors may use other databases or APIs. Their credentials and data
are outside Modary's control-store transaction and require an independent
security and idempotency review.

Handler error text is persisted in River task history. Task handlers return a
stable secret-safe error and keep connection strings, credentials, tokens,
payloads, and dependency response bodies out of the returned error chain.

## HTTP And MCP

Mount routes explicitly. Use secure cookies outside local development. Define
TLS, external origin, host policy, CSRF behavior, trusted proxy headers, request
limits, rate limits, and access logs at the consumer boundary. MCP is another
untrusted caller surface and receives no bypass around identity, authorization,
Preview, idempotency, or audit.

## Callbacks And Errors

Handlers, lifecycle callbacks, cleanup, authorization, identity, audit, clocks,
database dependencies, and output writers are trusted to be concurrency-safe
and cancellation-cooperative. Modary contains panic and dependency failures at
documented boundaries, but cannot stop unsafe memory behavior or interrupt a
blocked callback.

Return declared public business errors. Ordinary dependency details and panic
values must not become public protocol messages or audit content.

## Review Checklist

- No route or command calls a Handler outside Runtime.
- Every Action has the narrowest permission, channel set, and impact.
- Production identity and policy differ from example values.
- Secrets enter through a reviewed consumer-owned boundary and are redacted.
- Database roles, schema ownership, token files, generated output, and build paths satisfy ownership policy.
- Backups are protected, integrity-checked, and restorable.
- Modules honor cancellation and are race-tested.
- Public exposure has explicit TLS, proxy, cookie, host, origin, and rate policy.
- The [security policy](../../SECURITY.md) has a valid private contact before distribution.
- Every item in [known limitations](../f0-known-limitations.md) is accepted or mitigated.
