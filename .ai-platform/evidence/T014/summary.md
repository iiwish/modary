# T014 Evidence Summary

- Status: Completed
- Date: 2026-07-30
- Packet: `.ai-platform/specs/002-framework-decoupling/packets/T014.yaml`

## Changed Files

- Neutral adapters: `adapters/sqlite/**`, `adapters/localidentity/**`,
  `adapters/rbac/**`, and `adapters/sqlaudit/**`.
- Shared SQL boundary: `database/**`.
- Cross-adapter and authorization atomicity coverage: `adapters/conformance_test.go`
  and `action/{runtime.go,runtime_test.go}`.

## Red Result

Focused regressions reproduced insecure file creation and ownership assumptions,
cross-database transaction substitution, caller transaction escape, direct RBAC
row-limit bypass, authorization revocation between final check and write,
password-rotation session resurrection, concurrent credential invalidation
misclassification, expired-session accumulation, metadata audit detail leakage,
corrupt provenance acceptance, and credential-verification memory amplification.

## Green Result

- SQLite creates missing directories as `0700` and databases as `0600`, requires
  the final directory/database/sidecars to be owned by the effective UID, rejects
  unsafe or symbolic paths, and never repairs existing ownership or modes.
- The database package owns transaction creation, binds a transaction to its
  originating database, and exposes only an opaque `Executor` without commit or
  rollback control.
- Runtime repeats intent and impact authorization inside the write transaction;
  both new writes and completed-result replay fail when policy changes at the
  transaction boundary.
- Local Identity uses Argon2id password hashes, digest-only bearer/session
  storage, a context-aware bounded password-check gate, single-query credential
  resolution, canonical identity validation, session cleanup, and explicit
  revocation/reactivation rules.
- RBAC is durable, default deny, scope exact, transaction aware, fingerprinted,
  and directly enforces impact row bounds.
- SQL Audit validates successful provenance, metadata detail exclusion,
  duplicate references, canonical timestamps, bounded text, and corrupt rows,
  and joins the caller's SQL transaction.

## Validation

The final implementation passed:

```text
go test ./action ./adapters/... ./database
go vet ./action ./adapters/... ./database
go test -race ./action ./adapters/... ./database
go test -count=20 ./action ./adapters/... ./database
rg -i 'rulary|ruleset|workspace|demo|default.*password|default.*token|os\.Getenv|core/config' adapters database action  # no matches
git diff --check -- action adapters database
```

SQLite ownership tests also pass Linux and Windows cross-compilation; the
Windows file-backed profile fails closed before filesystem access.

## Provisioning Review

Empty Options create schema and services only. They create no principal,
password, token, session, role, permission, binding, grant, policy, audit event,
or consumer row. Provisioning is a validated, defensive-copied patch: omitted
durable records remain unchanged, revocation is idempotent, and re-provisioning
an actor or role does not silently resurrect revoked credentials or bindings.
Security-context changes invalidate sessions and require explicit bearer
reactivation.

## Durability Review

Module migrations are append-only filename/checksum prefixes and all pending
files for one module commit atomically. Plans, idempotency provenance, policy,
credentials, and audit data survive restart. Failed Actions roll back the
business mutation, reservation/completion, and allowed audit together. Policy
mutation makes a preview or prior completed result stale before disclosure.

## Security Review

Credentials and caller secrets are never implicitly provisioned or read from
environment/global configuration. Bearer generation uses 256 bits from
`crypto/rand`; custom random readers are documented as CSPRNG-only in
production. Password verification has equal costly work for known and unknown
users and bounded concurrent memory. Audit and identity errors expose stable
sentinels rather than stored secrets. SQLite's secure file profile fails closed
when owner, mode, path type, or ownership metadata cannot be proven.

## Residual Risk

- POSIX mode/UID checks do not isolate root, the same UID, ACL grants, hostile
  mount replacement, or filesystems that misreport ownership; those environments
  require an ACL-aware or externally isolated database adapter.
- Network/IP/account rate limiting is deployment policy beyond the adapter's
  in-process Argon2 concurrency bound.
- SQL Audit F0 publishes a write Hook, not a public query/export/retention API.
- The supported atomicity claim is one SQLite-backed durable store; distributed
  transactions and arbitrary adapter atomicity remain out of scope.

## Independent Review

The final independent review found no unresolved P0, P1, or P2. It separately
verified transactional authorization and replay, database ownership binding,
credential concurrency and revocation, RBAC constraints, Audit corruption
handling, SQLite file security, restart behavior, neutrality, and public API
documentation. The listed residual risks are explicit F1 or deployment bounds.
