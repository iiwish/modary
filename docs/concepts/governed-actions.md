# Governed Action Lifecycle

An Action is a static business-operation contract plus a consumer-owned Handler.
Every supported transport submits the same request to `action.Runtime`; no
surface calls `Execute` directly.

## Descriptor

The descriptor declares:

- stable Action ID and version;
- input, preview, and output JSON Schemas;
- required permission and allowed channels;
- Preview policy and audit level;
- whether idempotency is required;
- consumer-owned public errors and their semantic kinds.

Descriptors are compiled and validated during assembly. Invalid schemas,
duplicate IDs, undeclared error contracts, or unsupported combinations prevent
startup.

## Preview

Runtime validates the request and actor, checks intent authorization, and calls
`Handler.Plan`. The Handler returns an opaque payload, a caller-visible summary,
bounded impact, and an optional snapshot hash.

Runtime authorizes the concrete impact and creates a time-bounded plan hash that
binds Action contract, actor, channel, execution scope, input, impact, policy
fingerprint, and snapshot. The caller cannot edit a plan and preserve its hash.

## Execute

Runtime revalidates the request and plan, reserves idempotency, begins the
transaction, reauthorizes inside the transaction, and invokes
`Handler.Execute` with the bound plan. The Handler rechecks mutable business
state and returns a schema-valid result. Required audit and durable result state
complete under the defined transaction outcome.

If the descriptor requires Preview, execution without a valid plan fails. A
stale snapshot, changed input, actor, scope, channel, Action contract, or policy
decision fails closed.

## Error Contract

Framework errors use built-in codes. Consumer business errors must be declared
by the descriptor and returned as `action.Error` with the declared semantic
kind. Ordinary dependency errors, invalid envelopes, undeclared codes, panic,
or malformed outputs are normalized as internal failures without leaking unsafe
diagnostics.

## Concurrency And Cancellation

The same Handler may receive concurrent planning and execution calls. It treats
requests and plans as immutable, uses concurrency-safe dependencies, and
returns promptly after context cancellation. Modary bounds its own waits but
cannot interrupt a callback or writer that ignores cancellation.

See [ADR-002](../adr/ADR-002-governed-action-transaction.md) and the
[exposure guide](../how-to/expose-action.md).
