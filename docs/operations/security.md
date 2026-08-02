# Security Boundaries

Modary is a same-process application framework, not a hostile plugin sandbox,
network security product, IAM suite, or substitute for secure product design.

## Core And Modules

Module graph validation, typed capabilities, and sealed lifecycle scopes reduce
accidental authority. Consumer Modules and official components still execute as
trusted code under the same operating-system identity. Malicious or unsafe code
in that process can use Go and operating-system capabilities outside Modary.

## Ordinary Admin Mutations

Admin routes authenticate sessions, validate CSRF for mutations, and authorize
the actor, operation, and scope in backend RBAC before repository work.
`database.Store` owns callback transactions and exposes no raw commit/rollback.

UI visibility is not authorization. Every route remains responsible for its
backend decision, bounded input, scope isolation, optimistic concurrency, and
safe error mapping.

## Governed Mutations

Only operations selected as Actions enter `action.Runtime`. The Runtime
validates actor/request/schema, authorizes intent, binds Preview to impact,
requires idempotency when declared, owns the transaction, reauthorizes, validates
output, and enforces audit ordering. CLI, HTTP, MCP, and custom surfaces must not
retain or call raw Handlers.

Ordinary CRUD is not automatically governed. Product review decides which
operations require the stronger boundary.

## Identity And Credentials

The local Identity Adapter provisions only explicitly configured principals and
credentials. It is suitable for development and controlled internal scenarios,
not a complete public-internet identity lifecycle.

Before wider exposure, define SSO/MFA or equivalent requirements, enrollment,
recovery, revocation, brute-force defense, session policy, account monitoring,
and incident response. Never ship generated passwords or bearer tokens.

CLI bearer tokens belong in standard input or a supported mode-0400/0600 token
file. Do not place them in command arguments, source, logs, or shell history.

## HTTP, Cookies, And MCP

Secure cookies are the default. The insecure-cookie option is for explicit
local HTTP only. The deployment owns TLS, trusted proxies, host/origin policy,
request/body limits, rate limits, and access-log redaction.

MCP is an untrusted caller surface and receives no bypass around identity,
authorization, Preview, idempotency, transaction, or audit.

## PostgreSQL And Tasks

Treat URLs and credentials as secrets. Use dedicated roles, restrict network
reachability, require TLS where appropriate, rotate credentials, and monitor
privilege changes. Modary does not configure database encryption, roles,
replication, backups, or row-level security.

Governed schemas must not be `public`, `information_schema`, or `pg_*`. The
configured role owns the application and queue schemas. Admin uses a separately
selected ordinary application schema.

Task delivery is at least once. Handler error text may be retained in River
history, so return stable secret-safe errors. Keep URLs, credentials, tokens,
complete payloads, and dependency response bodies out of returned errors.

## Frontend

Generated Admin source is product code. Review dependencies, CSP, content
rendering, accessibility, and any new dynamic HTML. The F0 UI renders bounded
JSON fields as text and relies on backend authorization. Building assets is a
trusted supply-chain step; production serves the checked, embedded bundle.

## Callback Boundary

Handlers, repositories, identity, authorization, audit, lifecycle callbacks,
task consumers, clocks, and writers must be concurrency-safe and
cancellation-cooperative. Panic containment prevents many boundary escapes but
cannot stop unsafe memory behavior or interrupt a callback that never returns.

## Review Checklist

- Selected Modules and dependency graphs match the intended Profile.
- Every Admin mutation authorizes at the backend and validates CSRF.
- Every Action has the narrowest permission, channels, scope, and impact.
- No transport calls a raw Action Handler.
- Example credentials and development Identity are replaced or explicitly
  accepted.
- Secrets are redacted from errors, task history, audit, and logs.
- TLS, cookie, proxy, host, origin, and rate policy are explicit.
- Database ownership, backup, restore, and task idempotency are tested.
- Module and callback paths pass race and cancellation tests.
- The private reporting channel in [SECURITY.md](../../SECURITY.md) is valid.
