# ADR-002: Governed Action And Transaction Boundary

- Status: Accepted
- Date: 2026-07-31
- Scope: Action authorization, plans, idempotency, transactions, and audit

## Context

Human and automated channels need identical business semantics. Authorization
can change between an initial check and a durable write, retries can arrive after
a committed response is lost, and an allowed audit event must not disagree with
the mutation it describes.

## Decision

Every channel constructs an `action.Request` and calls one governed Runtime. The
Runtime validates the immutable Action contract and canonical input, authorizes
intent, binds Preview output and impact into an expiring plan when required, and
authorizes impact constraints.

The descriptor also declares consumer-owned public error codes and their closed
semantic kinds. Runtime accepts only source-appropriate built-in codes or a
declared custom code with its exact kind and a bounded presentation-safe message.
Handler denial and internal custom codes are invalid; authorization denial is
owned by the Authorizer. HTTP, MCP, CLI, and audit project the same normalized
code, kind, and message instead of independently interpreting string codes.

Transport and command adapters validate the complete Actor returned by an
installed authenticator before catalog discovery or Runtime entry. Error
classification at extension boundaries follows only a bounded graph of
standard-library and framework-owned wrappers; caller-defined `Is`, `As`, and
`Unwrap` methods are never invoked.

Write execution repeats both intent and impact authorization inside the durable
transaction immediately before idempotency reservation and Handler execution.
The reservation, business mutation, completed result, and allowed audit event
commit together. Preview plan persistence and its required audit event also
commit together. A failure aborts the reservation and rolls back the complete
write. Transaction extensions must invoke their callback synchronously and at
most once; missing, repeated, concurrent, swallowed-panic, or escaped invocation
is a contract violation and fails closed. The official transaction owner returns
an exact private outcome correlated with that callback. Plan storage,
idempotency storage, and transaction ownership are installed atomically as one
framework-private persistence bundle; application Modules cannot replace or
mix those services. A business error is
preserved only when rollback is proven complete; pending or failed rollback,
commit failure, forged or wrapped outcomes, and ambiguous callback results are
internal atomicity failures. Nested SQLite calls join the outer transaction and
mark it rollback-only after any inner failure or panic. The transaction manager
owns commit and rollback; consumers receive only an opaque context and sealed
transaction-aware `database.Access`.

A completed retry is looked up only after current intent authorization. The
record is reread inside a transaction, current intent and stored impact are
reauthorized, and the complete stored record, authorization fingerprint, and
result are checked against the current Action contract before disclosure.
Replay therefore survives plan expiry without
bypassing policy revocation or changed Action contracts.

Denied and failed audit records execute after rollback with request values and a
finite detached timeout. Persistence failure is reported through the configured
callback without replacing the primary Action error. Audit normalization bounds
all descriptive data and removes detail from metadata-only events.

## Consequences

- HTTP, command, MCP, and custom channels share authorization, retry, and audit
  behavior rather than reimplementing it.
- A lost response can be retried without repeating a committed business effect.
- Revocation before a transaction or replay prevents the write or disclosure.
- The official atomicity claim requires stores and handlers to use one SQLite
  database and `database.Access` with the provided context.
- F0 custom applications can extend Actions and safe data access, but a new
  privileged persistence adapter is a framework contribution rather than an
  application plug-in.
- Long-running handlers hold a serialized SQLite write transaction and must stay
  bounded.

## Rejected Alternatives

- Authorizing only before opening the transaction leaves a time-of-check to
  time-of-use window.
- Returning a completed idempotency result before authorization leaks data after
  policy revocation.
- Best-effort success audit can commit an effect without its required record.
- Exposing `*sql.DB` or `*sql.Tx` gives Handler code an ungoverned write or
  transaction-control path and permits premature commit or rollback.
