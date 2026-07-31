# Framework Decoupling Requirements Checklist

- Version: 1.0
- Status: Completed
- Source spec: `../spec.md`
- Last updated: 2026-07-31

- [x] Framework and consumer ownership are explicit.
- [x] Canonical import path and public package boundary are decided.
- [x] Composition has one SSOT and avoids dynamic package discovery.
- [x] Pure Definition and side-effecting Start phases are separated.
- [x] Lifecycle rollback and migration rollback promises are distinguished.
- [x] Typed capability, handler ownership, and catalog exposure are defined.
- [x] Open `module.Capability` values, standard framework constants, and
  consumer-owned namespaced typed-service capabilities are defined for
  database, identity, authorization, and audit composition.
- [x] Independent providers and consumers share one package-level typed key;
  undeclared access and recreated same-name keys fail closed.
- [x] The HandlerFactory Resolver is valid only during the factory call;
  retained use fails with `module.ErrInvalidResolver`.
- [x] Constructor-only Host availability and terminal Start reference release
  fail closed for nil, zero, forged, copied, unavailable, successful, failed,
  and canceled lifecycle paths. Host-owned Start callbacks, handler factories,
  and migration filesystem references are released without mutating
  caller-owned Registrations.
- [x] Execution scope and SQLite transaction promises are explicit.
- [x] File-backed SQLite ancestor ownership and root-owned sticky writable-directory exceptions are explicit.
- [x] Headless, generation, transport, Adapter, and neutrality gates are measurable.
- [x] Authenticator output and opaque public extension/dependency error chains fail closed without caller-defined error-method dispatch or internal consumer helpers.
- [x] Declared custom Action errors preserve one bounded code, semantic kind, safe message, context, and audit outcome across Runtime, CLI, HTTP, and MCP.
- [x] Public SQL policy rejects multiple statements, DDL, transaction control, executable rollback conflicts, and quoted or unquoted `temp.` schema access before invoking a backend.
- [x] Migration entry, name, file, aggregate, and applied-history bounds, full pre-effect validation, history continuity, and rollback behavior are explicit.
- [x] Transaction callbacks require exact private completion and rollback outcome proof; untrusted or uncertain outcomes fail closed.
- [x] Nested SQLite transactions join one outer rollback-only unit without savepoints, including swallowed inner failures.
- [x] Callback lifecycle detects completion independently of recovered values, including `panic(nil)`, and enforces synchronous at-most-once execution and cleanup.
- [x] Cleanup defines invocation-start order.
  A timed-out cleanup callback may overlap later callbacks and provider cleanup;
  trusted callbacks honor
  context cancellation and stop using dependencies.
- [x] Runtime, Handler, authorization, audit, identity, database, Clock, and
  `AuditFailure` contracts are concurrency-safe and context-aware;
  `AuditFailure` receives an independent deadline context.
- [x] The production import matrix has an explicit public package inventory,
  privileged-internal allowlist, and fail-closed appcmd and sibling-Adapter
  dependency fixtures.
- [x] Application shutdown atomically revokes and cancels every Runtime and identity lease, gives each caller an independently bounded wait, and automatically starts exactly-once Host cleanup after the final lease drains without requiring a second call.
- [x] An Action schema, request input, or Handler plan payload is one separate
  Action JSON document. A Preview summary or Result data is also one separate
  Action JSON document. A persisted Action JSON value is likewise one separate
  Action JSON document. Every Action JSON document has independent limits of at
  most 1 MiB (1,048,576) source bytes, 256 nested object or array containers
  including the root container, 65,536 JSON value nodes including containers and
  scalar values but excluding object member names, and 4,096 source bytes for
  any one JSON number token. Every document is valid UTF-8 and contains
  exactly one JSON value, with no duplicate object member names.
  HTTP and MCP request envelopes have independent byte budgets; their 2 MiB defaults
  carry one complete maximum-size Action JSON document plus required envelope fields
  before every extracted Action document is revalidated against the
  per-document Action limits.
- [x] Action schemas use Draft 7 object or boolean roots. One immutable
  executable SchemaGraph covers schema-syntax locations and arbitrary local JSON Pointer
  reference closure. Only URI fragments decoding to `#` or absolute
  `#/...` pointers are accepted; empty and non-local references, anchors,
  `id`/`$id`, external registries, file access, and network I/O fail closed.
  Every actual root is validated offline against a digest-pinned Draft 7 metaschema.
  Static admission has exact boundaries of 2,048 schema nodes, 512
  collection entries, 256 enum values, 16 KiB encoded literal, 4 KiB Go RE2
  pattern, 1,024 same-instance visits, and 64 Mi cumulative numeric compilation
  work units. Flag-only validation has 64 Mi work units, 4,096 mismatch events, and 4,096 active evaluation frames,
  exposes no dependency diagnostic tree, and
  maps evaluation exhaustion to `LIMIT_EXCEEDED`. MCP preserves literal data and
  retains every Action limit while adding only 128 framework wrapper nodes, a
  1 Mi numeric wrapper allowance, and 4,096 compile-only JSON value nodes.
  The pinned official Draft 7 mandatory corpus accounts for all 37 files, 257
  cases, and 927 tests: 223 cases and 856 tests execute, and an exact manifest
  verifies 34 policy exclusions covering 71 tests that require identifiers, URI
  bases, anchors, or non-local resources.
- [x] HTTP and MCP reject a missing `input` member before Runtime and audit while
  preserving explicit `null`, object, array, and scalar values for Action schema
  validation.
- [x] Public database row lifecycle defines `Next`, `Scan`, `Err`, `Columns`, and idempotent `Close`, with sticky terminal errors and dependency panic containment.
- [x] Sibling-rename, verified-Root mutation, load-captured filesystem-identity locking, canonical outside-project staging, Linux/Darwin ACL policy, unsupported-platform fail-closed Build, cross-compile-only platform claims, and same-UID pathname boundaries are explicit.
- [x] Compiler cancellation and inherited-pipe waits are bounded for cooperative output writers; blocked caller `io.Writer.Write` remains an explicit trusted dependency.
- [x] Go module/toolchain/work-file/flags isolation, child-environment deduplication, canonical `TMPDIR`/`GOTMPDIR` pinning, and the remaining trusted environment, toolchain, and consumer-source boundary are explicit.
- [x] Linux/Darwin waitid-before-reap process-group cleanup, successful-Build `exec.ErrWaitDelay` residual-pipe handling, failure precedence, and the daemonized or re-grouped descendant limitation are explicit.
- [x] Linux/Darwin CLI token-path policy, other-platform pre-filesystem rejection, and standard-input fallback are explicit.
- [x] Consumer prototype preservation precedes removal.
- [x] Technical acceptance, remote distribution, version tag, and license claims are distinct.
- [x] Every F0 requirement has automated or reviewable acceptance evidence.
- [x] Deferred platform features are explicit non-goals.
- [x] User approval authorizes autonomous execution through F0 acceptance.

Findings: 0 Critical, 0 High, 0 unresolved Medium.
