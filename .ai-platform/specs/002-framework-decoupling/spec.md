# Modary Framework Decoupling F0

- Version: 1.0
- Status: Confirmed
- Source: user-approved framework-decoupling analysis and autonomous execution request
- Last updated: 2026-07-31
- Governing constitution: `../../memory/constitution.md`

## Objective

Deliver Modary as an independently consumable governed application framework.
A separate Go module must be able to define, inspect, generate, build, start,
and exercise its own application without modifying or copying Modary source.

## Users

- Application developers building private or local-first operational products.
- Module authors contributing typed governed Actions and durable data behavior.
- Operators exposing the same business behavior to people, HTTP clients, CLI
  automation, and Agents under consistent policy and audit controls.

## Functional Requirements

- FR-001: Public packages use the canonical module path
  `github.com/iiwish/modary`; consumers import no Modary `internal` package.
- FR-002: `appkit.DefinitionProvider` is the canonical consumer-owned
  error-returning Go composition contract;
  `appcmd.DefinitionProvider` and `projecttool.DefinitionProvider` are aliases.
  One provider is the sole Module composition source for both application and
  project commands. Module Definitions are pure and include metadata, Action
  descriptors, migrations, and handler factories without starting resources. A
  pure application metadata value is reused by the Definition and application
  command options.
- FR-003: Module services use typed keys and capability-scoped installation.
  `module.Capability` is an open validated named string type with standard
  database, identity, authorization, and audit constants; consumers may define
  namespaced capabilities for their own typed services. A provider and every
  consumer share one package-level typed key; same-name key recreation fails
  closed. The HandlerFactory Resolver is valid only during the factory call,
  and retained use returns `module.ErrInvalidResolver`.
  A module cannot access undeclared capabilities, retain an active installation
  scope, resolve Host services directly, forge framework-reserved service names,
  reach privileged services through reflection, or forge Action ownership.
- FR-004: Module lifecycle validates before side effects, rolls back partial
  process resources, joins cleanup errors, and is exactly-once. Cleanup
  guarantees invocation-start order in reverse dependency/LIFO order. A
  timed-out cleanup callback may overlap later callbacks and provider cleanup;
  trusted callbacks honor cancellation and stop using dependencies. Hosts are
  constructed through framework
  constructors; nil, zero, forged, or copied values are unavailable and fail
  closed. The Host defensively copies Registrations and releases its Start
  callbacks, handler factories, and migration filesystem references after the
  sole terminal Start attempt without mutating the caller-owned values.
  Transaction extensions invoke the framework
  callback synchronously and at most once; missing, repeated, concurrent,
  swallowed-panic, and escaped invocation fail closed. Panic detection depends
  on callback completion rather than the recovered value. A business failure is
  preserved only with exact private proof that its own callback was rolled back;
  uncertain atomicity is internal.
- FR-005: Public AppKit assembles the governed Runtime without exposing Host,
  mutable Registry, raw handlers, a raw database, or transaction control. Host
  assembly resolves required and optional services under one lifecycle snapshot.
  The first assembly result, including failure, is cached, and every returned
  facade shares the Host-owned lifecycle domain. No exported signature
  references a Modary `internal` type.
- FR-006: Public HTTP and MCP transports are mounted explicitly and receive
  consumer metadata and assets. They do not inspect concrete Module IDs.
- FR-007: Consumer application commands provide serve and governed CLI Action
  execution authenticated by an installed identity adapter; Modary project
  tooling provides pure verify, generate, check, and build behavior.
  The Serve Handler factory receives the active command context and fully
  started Application and observes cancellation cooperatively.
  Help, version, and invalid command paths do not invoke the Definition provider.
  Runtime command paths invoke it exactly once after pure preflight, preserve
  ordinary construction errors with context, contain panic values, and reject
  command/Definition metadata drift. Build sends
  compiler output to an outside-project operating system temporary directory.
  Modary canonicalizes `TMPDIR`, rejects it when it is inside or resolves through
  a symlink into the project, and retains a file descriptor and filesystem
  identity for that directory and every ancestor through `/`. Every retained
  directory is revalidated, must be owned by the effective UID or root, and may
  be group- or other-writable only when root-owned and sticky. Darwin rejects any
  extended ACL at every retained level. The child staging directory must be
  effective-UID-owned with exact mode `0700`; Darwin also rejects an extended
  ACL on that child. Build removes every inherited case-variant `TMPDIR` and
  `GOTMPDIR` entry from the child environment, then sets both exactly once to the
  same canonical staging parent whose descriptor and ancestry are retained and
  revalidated. An ambient `GOTMPDIR` cannot override that parent.
  Every other platform, including other Unix variants and Windows,
  fails Build because F0 has no validated ACL policy there.
- FR-008: Generated graph, Action catalog, and optional TypeScript contracts are
  deterministic, consumer-owned, prepared as one bounded batch, and require no
  Start callback or handler factory. Each changed file is installed with one
  sibling rename, which is atomic only where the host filesystem guarantees
  rename atomicity. Filesystem-wide, crash, and cross-process atomicity are not
  claimed. Unsupported platforms are cross-compile-only where covered; F0 does
  not claim native Build, ACL, or rename runtime validation for them.
- FR-009: SQLite, local Identity, RBAC, and Audit adapters accept explicit
  options and install no users, policy, grants, secrets, or domain data by
  default. The file-backed SQLite profile requires every directory ancestor to
  be owned by the effective UID or root; a group- or other-writable ancestor is
  accepted only when root-owned and sticky. The final database directory remains
  effective-UID-owned and non-writable by group or other users.
- FR-010: The public execution boundary uses an opaque execution scope rather
  than a consumer-specific context field.
- FR-011: An independent neutral consumer contributes its own migration and a
  governed write Action exercised through Runtime, HTTP, CLI, and MCP.
- FR-012: Modary production code, active contracts, CI, build, and release paths
  contain no consumer-product or consumer-domain dependency.
- FR-013: CLI `--token-file <path>` is supported only on Linux and Darwin. The
  token remains a regular file owned by the effective UID with exact mode `0400`
  or `0600`; Darwin queries the retained open file descriptor and rejects any
  extended ACL. Every other operating system rejects a token path
  before any filesystem access. Only `--token-file -` reading standard input
  remains available there.

## Non-Functional Requirements

- NFR-001: Public contracts fail closed with deterministic errors. Authenticator
  results are validated before discovery or execution. All extension and
  dependency causes cross one opaque public error-chain boundary. Standard
  `errors.Is` and `errors.As` perform bounded matching without calling caller-
  defined `Error`, `Is`, `As`, or `Unwrap`; an external consumer needs no
  internal helper import. A typed-nil error remains a failure and retains safe
  `errors.Is`/`errors.As` identity without method dispatch. `appcmd.Options.Stdout`
  and `Stderr` are trusted,
  cooperative dependencies whose `Write` calls must return; context cancellation
  and shutdown timeouts cannot interrupt a blocked writer. Action descriptors
  bind consumer error codes to a closed semantic kind; Runtime validates code
  ownership, source, kind, and bounded public message before HTTP, MCP, CLI, or
  audit projection. Definition provider failures preserve ordinary diagnostics
  and safe error identity, while provider panics never expose the recovered value.
- NFR-002: Definition inspection and generation perform no database, migration,
  seed, network, or module-start side effect.
- NFR-003: Headless consumer validation installs and invokes no Node tooling.
- NFR-004: Startup failure and shutdown remain race-safe and leak-free for
  cooperative callbacks. Non-cooperative cleanup is time-bounded so all later
  callbacks are attempted, and its unavoidable overlap and goroutine limitation
  are explicit. Runtime, Handler, authorization, audit, identity, database,
  Clock, and `AuditFailure` contracts are concurrency-safe and context-aware.
  Each `AuditFailure` receives an independent deadline context.
  Same-root tooling operations serialize within one process by the filesystem
  identity captured when the Project is loaded, not by pathname spelling. With
  cooperative output writers, compiler cancellation and inherited output pipes
  have a bounded wait. Caller-supplied `io.Writer.Write` must return because Go
  cannot interrupt a blocked call.
- NFR-005: Action schemas compile once under JSON Schema Draft 7 and accept
  object or boolean roots. One immutable executable SchemaGraph owns every
  schema-syntax location and the unique closure of arbitrary local JSON Pointer
  references. A reference is a URI fragment decoding to `#` or an absolute
  `#/...` pointer, including valid percent-encoding. Empty, relative, query,
  named-fragment, file, and network references fail closed. Actual schema nodes
  prohibit `id` and `$id`; matching keys in literal data remain inert unless
  locally referenced as a schema. The graph root and every hidden reference root
  are validated offline against a digest-pinned Draft 7 metaschema. Compilation
  has no external registry, file access, or network I/O.
  Static admission bounds schema nodes, collection entries, enum values, literal
  bytes, Go RE2 pattern bytes, same-instance expansion, and cumulative numeric
  compilation work across constraints and numeric literals. Validation is
  flag-only, concurrency-safe, allocates no dependency diagnostic tree, and has
  fixed work, mismatch-event, and active-frame budgets. Static exhaustion
  rejects descriptor construction; evaluation exhaustion maps to
  `LIMIT_EXCEEDED`. MCP reuses SchemaGraph, preserves literal data, and receives
  only fixed wrapper-owned schema-node, numeric-work, and JSON-value-node
  allowances. A pinned official Draft 7 mandatory corpus executes every
  supported case and requires exact policy exclusions for identifiers, URI
  bases, anchors, and non-local resources. Runtime Preview, authorization
  recheck, plan binding, idempotency, transaction callback cardinality, exact
  completion proof, error normalization, dependency-output validation, and
  audit invariants remain covered.
- NFR-006: The SQLite F0 profile provides one durable transactional store. No
  distributed or arbitrary-adapter atomicity claim is made. Public SQL is one
  read or mutation statement under a fail-closed policy. Migration input and
  applied history have explicit entry, name, file, and aggregate bounds and are
  fully validated before database effects. Nested calls join the outer
  rollback-only unit without savepoints.
- NFR-007: Public APIs and generated formats are documented as alpha contracts
  and covered by AST boundary checks and copied-out external-consumer tests.
- NFR-008: Build sets `GO111MODULE=on`, `GOTOOLCHAIN=local`, `GOENV=off`,
  `GOWORK=off`, and an empty `GOFLAGS`, and passes `-mod=readonly` and
  `-buildvcs=false`. Build removes every inherited case-variant `TMPDIR` and
  `GOTMPDIR` entry from the child environment, then sets both exactly once to the
  same canonical staging parent whose descriptor and ancestry are retained and
  revalidated. An ambient `GOTMPDIR` cannot override that parent.
  Other inherited environment, the selected Go executable and toolchain, and consumer source
  remain trusted inputs; Build is not a sandbox.
  On Linux and Darwin, `waitid(WEXITED|WNOWAIT)` observes the started process-
  group leader without reaping it. While its PID and PGID remain reserved, Build
  kills residual same-group descendants after either a zero or non-zero leader
  exit and then calls `Cmd.Wait`. When the leader exited successfully, pre-reap
  group cleanup succeeded, and the context remains active, `exec.ErrWaitDelay`
  from `Cmd.Wait` is only a residual inherited-pipe close backstop and does not
  fail Build; writer, process-exit, cancellation, and cleanup errors still fail.
  Daemonized or re-grouped trusted descendants can escape cleanup, so this is not
  a strong process sandbox.
- NFR-009: An Action schema, request input, or Handler plan payload is one
  separate Action JSON document. A Preview summary or Result data is also one
  separate Action JSON document. A persisted Action JSON value is likewise one
  separate Action JSON document. Every Action JSON document has independent
  limits of at most 1 MiB
  (1,048,576) source bytes, 256 nested object or array containers including the
  root container, 65,536 JSON value nodes including containers and scalar values
  but excluding object member names, and 4,096 source bytes for any one
  JSON number token. Every document is valid UTF-8 and contains exactly one JSON value.
  It has no duplicate object member names. HTTP and MCP request envelopes have
  independent byte budgets. Their 2 MiB defaults can carry one complete
  maximum-size Action JSON document plus required envelope fields.
  Every extracted Action document is revalidated against the
  per-document Action limits. HTTP and MCP require a present `input` member
  before Runtime dispatch. Missing input is a protocol validation failure with
  no Runtime call or audit; explicit `null`, object, array, and scalar values are
  present documents governed by the Action schema.
- NFR-010: Every governed Runtime method and every actor-resolution, session,
  and bearer-token facade method holds one lease in the Host-owned assembly gate.
  `Application.Shutdown` delegates to the Host shutdown sequence, which marks the
  Application not ready, rejects new leases, cancels active leased contexts, and
  waits for every active lease to release before Host cleanup begins. New calls
  through a retained Runtime or identity facade fail closed after revocation and
  never reach a cleaned Module service.
  The authenticated `httpapi.NewAPI` and `httpapi.NewMCP` constructors require
  `application.Ready()` to be true. Each caller context bounds only that caller's
  wait for the shared shutdown result. If an active facade ignores cancellation,
  `Shutdown` returns the context error without starting Host cleanup while that
  lease remains active and keeps the gate revoked. The Host shutdown sequence
  waits independently and starts Host cleanup automatically when the final lease
  is released; another `Shutdown` call is not required. Retained HTTP and MCP
  handlers report lifecycle revocation or lifecycle-canceled authentication as
  unavailable rather than internal failure.

## Scope

- Public kernel packages, Module Definition and lifecycle, AppKit, HTTP/MCP
  transport, application command helper, project tool helper, neutral adapters,
  deterministic generators, independent consumer fixture, CI, and documentation.
- Transfer-safe removal of consumer-owned source and delivery artifacts from the
  Modary product boundary.

## Non-Goals

- Downstream consumer-product feature development.
- Dynamic plugins, module marketplace, workflow engine, scheduler, PostgreSQL,
  distributed transactions, SSO, or advanced IAM.
- Automatic runtime discovery of arbitrary Go packages.
- Go-driven cross-repository React route imports. Consumer applications own UI
  composition and pass built assets or handlers explicitly.
- Claiming that tagged remote installation has occurred before a release exists.
- Claiming public redistribution rights while the repository has no published
  version tag and no owner-selected redistribution license.

## Acceptance Criteria

1. A copied-out independent fixture with its own `go.mod` imports only public
   canonical Modary packages and completes verify, generate, check, test, build,
   and run using a local `replace` during source validation.
2. Definition inspection succeeds when every Start callback and handler factory
   is configured to fail if invoked.
3. Repeated generation is byte-identical; check mode reports drift without
   modifying files.
4. Headless validation succeeds with failing `node`, `npm`, and `pnpm` shims.
5. Module lifecycle contract tests cover partial failure, cancellation cleanup,
   reverse order, LIFO invocation starts, timeout overlap with provider cleanup,
   non-cooperative Start and HandlerFactory eventual rollback, joined errors,
   invalid state transitions, and race safety.
6. The neutral Action uses Preview, authorization, plan binding, idempotency,
   transaction, and Audit consistently through Runtime, HTTP, CLI, and MCP.
   Built-in and declared custom error codes retain one normalized code, semantic
   kind, safe message, and audit outcome across those channels.
7. Installing official adapters with empty provisioning produces no user,
   policy, binding, grant, or domain record. File-backed SQLite tests reject a
   foreign-owned ancestor and any group- or other-writable ancestor unless it is
   root-owned and sticky; the final directory keeps its stronger effective-UID-
   owned, non-group/other-writable policy.
8. MCP initialization reports consumer-provided name and version. MCP and UI
   routes are absent unless explicitly mounted.
9. Modary CI and release checks pass with no consumer-product source, schema,
   fixture, database, E2E, binary, or downstream repository dependency.
10. `go test ./...`, `go vet ./...`, `go test -race ./...`,
    `GODEBUG=panicnil=1 go test ./...`, generator drift, neutrality,
    external-consumer, and framework build gates all pass.
11. Consumer handlers can query through the narrow governed `database.Access`
    capability but cannot obtain raw database or transaction control, and writes
    outside the governed transaction fail closed. Consumers may implement the
    method-only interface for isolated tests but cannot install that value as the
    Host's canonical database service.
12. CLI Action execution derives actor and execution scope only from a validated
    bearer credential; no caller-asserted actor flag exists. Linux and Darwin
    token paths require a regular, effective-UID-owned file with exact mode
    `0400` or `0600`; Darwin rejects any extended ACL through the retained open
    file descriptor. Other operating systems reject a token path before any
    filesystem access and support only `--token-file -` through standard input.
13. HTTP, MCP, and CLI reject malformed authenticator-produced Actors before
    discovery or execution. Standard `errors.Is` and `errors.As` match bounded
    public chains without invoking a caller-defined `Error`, `Is`, `As`, or
    `Unwrap`, and the external consumer imports no internal error helper.
    `appcmd.Options.Stdout` and `Stderr` tests preserve their trusted cooperative
    boundary: cancellation and shutdown timeouts do not claim to interrupt a
    blocked `Write`.
14. Build gives Go only an outside-project temporary output. It retains and
    revalidates file descriptors and identities for canonical `TMPDIR` and every
    ancestor through `/`, rejecting a path inside or linked into the project.
    Each retained level is owned by the effective UID or root; group- or other-
    writable levels are accepted only when root-owned and sticky. Darwin rejects
    any extended ACL at every level. The child staging directory is effective-
    UID-owned with exact mode `0700`, and Darwin rejects its extended ACL. Every
    other platform fails Build for lack of a validated ACL policy. Unsupported
    platforms are cross-compile-only where covered, with no native Build, ACL,
    or rename runtime validation claim. A fake Go regression receives a symlink
    alias through ambient `TMPDIR` and a malicious project-path `GOTMPDIR`, then
    observes the same canonical staging parent in both child variables.
15. Path aliases for one loaded project share one in-process serialization gate
    by captured filesystem identity; pathname spelling does not create a second
    same-root gate. Separate processes still require external coordination.
16. Cooperative writer tests prove bounded compiler cancellation and inherited-
    pipe waiting. A caller writer that blocks inside `io.Writer.Write` remains a
    trusted dependency outside Go's interruption capability.
17. Build fixes `GO111MODULE`, `GOTOOLCHAIN`, `GOENV`, `GOWORK`, and `GOFLAGS` to
    the documented values and uses `-mod=readonly` and `-buildvcs=false`. It
    deduplicates inherited case variants of `TMPDIR` and `GOTMPDIR` and pins
    both child variables to the canonical retained and revalidated staging
    parent. Tests do not treat the remaining environment, Go toolchain, or
    consumer source as sandboxed.
18. Linux and Darwin tests prove `waitid(WEXITED|WNOWAIT)` observes but does not
    reap the started process-group leader. While its PID and PGID are reserved,
    Build kills residual same-group descendants after zero and non-zero exits,
    then calls `Cmd.Wait`. A successful leader plus successful pre-reap cleanup
    and an active context treats `exec.ErrWaitDelay` only as a residual inherited-
    pipe close backstop, not a Build failure. Writer, process-exit, cancellation,
    and cleanup errors remain failures.
    The contract excludes daemonized or re-grouped descendants from a strong
    process-sandbox guarantee.
19. Public API AST checks reject every exported signature that references a
    Modary `internal` package. Scope and Resolver reflection tests expose no
    privileged bridge method, and a copied-out consumer cannot import private
    runtime or database-control packages.
    The import matrix uses an explicit public production package inventory, fails closed
    for every unlisted package, allowlists privileged internal imports, and rejects
    appcmd-to-transport/tooling and sibling-Adapter coupling. The copied-out
    consumer provides and consumes its own capability through one package-level
    typed key, rejects undeclared access and recreated same-name keys, and proves
    retained Resolver failure. Action and transport packages import the
    framework JSON Schema wrapper and cannot import its engine directly.
20. Public database tests reject multiple statements, DDL, transaction control,
    executable rollback conflict forms, typed-nil or panicking results, and writes
    outside the governed transaction. SQLite tests prove nested rollback-only
    behavior and hook detection of premature SQL commit or rollback.
21. Migration tests prove a 256-entry root bound, 255-byte names, 1 MiB per file,
    16 MiB per source, at most one byte of over-read, a 256-row applied-history
    bound, conforming partial directory batches, and zero database effect before
    complete source validation.
22. Exact-boundary tests accept each Action JSON limit exactly and reject the
    first byte, nested container, value node, or numeric-token byte beyond it for
    schemas, request input, Handler plan payload, Preview summary, Result data,
    and persisted values. Runtime, persistence, CLI, HTTP, and MCP use the same
    per-document contract. HTTP and MCP retain independent envelope budgets, and
    each 2 MiB default carries one complete maximum-size Action JSON document plus
    its required envelope fields before the extracted document is revalidated.
    Schema tests independently accept the exact static profile boundaries of
    2,048 schema nodes, 512 collection entries, 256 enum values, 16 KiB encoded
    literal, 4 KiB Go RE2 pattern, 1,024 same-instance visits, and 64 Mi
    cumulative numeric compilation work units, and reject the first value beyond
    each boundary. The executable SchemaGraph applies these limits to grammar
    nodes and arbitrary hidden local-reference closure, validates every actual
    root offline against the pinned Draft 7 metaschema, and rejects every
    `id`/`$id` or non-local reference before engine compilation.
    Evaluation tests enforce 64 Mi work units, 4,096 mismatch events, and 4,096 active evaluation frames
    without constructing a diagnostic tree. MCP tests
    prove that every exact-boundary Action schema embeds under only a 128-node
    framework wrapper allowance, a 1 Mi numeric wrapper allowance, and 4,096
    compile-only JSON value nodes without changing the Action evaluation budget
    or literal semantics. The pinned official Draft 7 mandatory corpus verifies
    37 files, 257 cases, and 927 instance tests: 223 cases and 856 tests execute,
    while an exact reason-checked manifest accounts for 34 policy exclusions and
    71 tests requiring identifiers, URI bases, anchors, or non-local resources.
    HTTP and MCP tests distinguish missing input from explicit `null`, `{}`,
    arrays, and scalars and prove that only the missing member is rejected before
    Runtime and audit.
23. Application lifecycle tests race shutdown with active governed Runtime and
    identity facade calls and prove that the Host-owned assembly gate either
    grants and tracks a lease or rejects the call. `Shutdown` cancels active
    leased contexts and waits for every lease to release before Host cleanup;
    retained Runtime,
    actor-resolution, session, and bearer-token facades fail closed without
    reaching Module services. Authenticated HTTP and MCP construction rejects an
    Application when `Ready()` is false. With a bounded caller context, a
    non-cooperative facade causes context expiration with zero Host cleanup while
    its lease remains active; after the facade returns, the Host shutdown
    sequence completes cleanup without requiring another `Shutdown` call.
24. Application help, version, serve help, Action help, and invalid syntax invoke
    the shared Definition provider zero times. Valid runtime and project commands
    invoke it exactly once. Ordinary Adapter and Definition construction errors
    retain operation context and safe identity; provider panics, including
    `panic(nil)`, return a stable diagnostic without exposing the panic value.
    Runtime commands reject any mismatch between pure command metadata and
    Definition metadata before Module startup.
25. Host contract tests prove nil, zero, forged-initialization, and copied values
    report `StateUnavailable` and return `ErrHostUnavailable` from every stateful
    public operation. Success, failure, and cancellation each release the
    Host-owned Start callbacks, handler factories, and migration filesystem
    references; the caller Registration remains intact, and each migration
    source is applied exactly once during one Start attempt.

## Stop Conditions

- The consumer needs a Modary source edit, copied Kernel code, or an `internal`
  import to add its Module or Action.
- Generation needs application startup or persistent state.
- A generic Adapter needs consumer domain policy or seed data.
- Framework abstraction introduces dynamic discovery or global state to avoid
  explicit Go composition.
