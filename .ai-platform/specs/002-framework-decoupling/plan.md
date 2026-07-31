# Framework Decoupling F0 Technical Plan

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-07-31

## Decisions

1. Use `github.com/iiwish/modary` as the canonical Go module path.
2. Move public kernel APIs to top-level packages and add public `appkit`,
   `transport/httpapi`, `appcmd`, and `projecttool` packages.
3. Use one explicit error-returning consumer Definition provider for application
   and project commands. Reuse one pure application metadata value in both the
   Definition and application command options. `modary.yaml` contains application
   and output metadata but does not list Modules.
4. Model Module registration as a pure Definition plus runtime Start and Action
   bindings. Handler ownership and migration timing are assigned by the Host;
   Handler factories receive a read-only Resolver valid only during the factory
   call and retain resolved service values rather than the Resolver.
5. Use generic service keys, a sealed startup Scope, a sealed read-only Resolver,
   and a strict Host state machine. Providers and consumers share one
   package-level typed key, so same-name recreation fails closed. The Host owns
   mutable Runtime state, assembles
   public facades under one lifecycle snapshot, and exposes no privileged method
   through Scope or Resolver reflection.
6. Represent tenancy as `scope.Execution{Kind, ID}`. The SQLite F0 adapter stores
   both values and remains the documented durable profile.
7. Keep migrations forward-only and atomically retryable per module. Expose
   consumer SQL through the narrow method-only `database.Access` contract;
   retain official facade construction, SQL policy, migration, and transaction
   control behind an internal type and private typed service key shared by Host
   assembly and official adapters. F0 custom durable adapters are framework
   contributions rather than application plug-ins. Lifecycle rollback covers
   process resources and service bindings, not committed schema. For file-backed
   SQLite, require every directory ancestor to be owned by the effective UID or
   root; accept a group- or other-writable ancestor only when root-owned and
   sticky. Keep the final database directory effective-UID-owned and
   non-writable by group or other users.
8. Make transports explicit application wiring. Consumer assets are passed as an
   `fs.FS` or handler; Modary does not embed consumer UI.
9. Preserve consumer-owned prototype assets under independent downstream
   repository ownership, with a transfer manifest and checksums, before removing
   them from Modary. The active Modary repository remains fully neutral and
   independently testable.
10. Validate every authenticator-produced Actor before discovery or execution.
    Put every extension and dependency cause behind one opaque public
    error-chain boundary. Standard `errors.Is` and `errors.As` perform bounded matching
    without calling caller-defined `Error`, `Is`, `As`, or `Unwrap`; external
    consumers need no internal helper import.
11. Key same-root in-process serialization by the filesystem identity captured
    when the Project is loaded, not by pathname spelling. Separate processes
    remain a consumer automation boundary.
12. Bound compiler cancellation and inherited output-pipe waiting when output
    writers cooperate. Caller-supplied `io.Writer.Write` must return because Go
    cannot interrupt a blocked call; it remains a trusted callback boundary.
13. Support native Build only on Linux and Darwin. Retain and revalidate file
    descriptors and identities for canonical `TMPDIR` and every ancestor through
    `/`. Each level is owned by the effective UID or root; a group- or other-
    writable level is accepted only when root-owned and sticky. Darwin rejects
    extended ACLs at every level. The child staging directory is effective-UID-
    owned with exact mode `0700`, and Darwin also rejects its extended ACL. Other
    platforms fail closed because their ACL policy is not validated.
14. Isolate ambient Go controls with `GO111MODULE=on`, `GOTOOLCHAIN=local`,
    `GOENV=off`, `GOWORK=off`, an empty `GOFLAGS`, `-mod=readonly`, and
    `-buildvcs=false`. Remove every inherited case-variant `TMPDIR` and
    `GOTMPDIR` entry from the child environment, then set both exactly once to
    the same canonical staging parent whose descriptor and ancestry are retained
    and revalidated. An ambient `GOTMPDIR` cannot override that parent. Treat the
    remaining environment, selected Go executable and toolchain, and
    consumer source as trusted rather than sandboxed.
15. On Linux and Darwin, run Go in an independent process group. After `Start`,
    use `waitid(WEXITED|WNOWAIT)` to observe but not reap the leader. While its
    PID and PGID remain reserved, kill residual same-group descendants after zero
    and non-zero exits, then call `Cmd.Wait`. When the leader and pre-reap group
    cleanup succeeded and the context remains active, treat `exec.ErrWaitDelay`
    only as a residual inherited-pipe close backstop, not a Build failure. Writer,
    process-exit, cancellation, and cleanup errors remain failures. Do not claim
    containment for a trusted descendant that
    daemonizes or enters another process group.
16. Support CLI `--token-file <path>` only on Linux and Darwin. Require a regular,
    effective-UID-owned file with exact mode `0400` or `0600`; on Darwin, query
    the retained open file descriptor and reject every extended ACL. On every
    other operating system, reject a token path before any filesystem access and
    support only `--token-file -` through standard input.
17. Treat `appcmd.Options.Stdout` and `Stderr` as trusted cooperative
    dependencies. `Write` must return; context cancellation cannot interrupt a
    blocked writer, and neither can a shutdown timeout.
18. Bind consumer-owned Action error codes and closed semantic kinds into the
    descriptor hash and generated contracts. Normalize extension errors once in
    Runtime, enforce source authority, and project one code/kind/message contract
    through HTTP, MCP, CLI, and audit. Internal or malformed errors remain opaque.
19. Accept only one public SQL read or mutation statement and reject transaction
    control and executable rollback forms. Bound migration roots to 256 entries,
    names to 255 bytes, files to 1 MiB, aggregate retained SQL to 16 MiB, and
    applied history to 256 rows; validate the complete set before effects.
20. Require exact framework-private transaction outcome proof correlated with the
    callback. Preserve business classification only after confirmed rollback;
    treat rollback uncertainty and commit failure as internal. SQLite nesting
    joins the outer rollback-only unit and uses driver hooks to detect premature
    SQL transaction completion.
21. Detect panics by incomplete callback return, never by `recover() != nil`, and
    run the complete suite with `GODEBUG=panicnil=1` in acceptance.
22. Parse help, version, and invalid command paths without evaluating consumer
    composition. Invoke the Definition provider exactly once only after pure
    runtime or project-command preflight. Preserve ordinary construction errors
    with operation context and safe identity, contain provider panics without
    formatting recovered values, and reject command/Definition metadata drift
    before Module startup.
23. Treat Action JSON and JSON Schema as bounded framework contracts rather
    than transport implementation details. Apply the same 1 MiB byte, 256
    nesting-level, 65,536-node, and 4,096-byte numeric-token limits to every
    Action JSON value. Compile Draft 7 object or boolean schemas once during
    static preflight from one immutable executable SchemaGraph. The graph owns
    schema-syntax locations and the unique closure of arbitrary local JSON Pointer
    references. Only URI fragments decoding to `#` or absolute `#/...`
    pointers are accepted; valid percent-encoding is preserved canonically.
    Empty, relative, query, named-fragment, file, and network references fail
    closed. Every actual schema node prohibits `id` and `$id`, while literal data
    remains inert unless a local reference targets it as a schema.
    Validate the graph root and every hidden reference root offline against a
    digest-pinned Draft 7 metaschema before compiling an engine program. Reject
    oversized structure, collections, literals, Go RE2 patterns, cumulative
    numeric work, or same-instance visits before startup. Each schema is limited
    to 2,048 schema nodes, 512 entries in one collection, 256 enum values, a
    16 KiB encoded literal, a 4 KiB pattern, 1,024 same-instance visits, and
    64 Mi cumulative numeric compilation work units.
    Validation uses a flag-only engine with 64 Mi work units, 4,096 mismatch events, and 4,096 active evaluation frames
    and never constructs an unbounded
    diagnostic tree. MCP rebases the same SchemaGraph, leaves source literals
    untouched, and reserves exactly 128 additional schema nodes, a 1 Mi numeric wrapper allowance,
    and 4,096 compile-only JSON value nodes without reducing
    any Action limit. Pin the official Draft 7 mandatory corpus by commit and
    digest; execute 223 of 257 cases and 856 of 927 tests, with 34 exact
    reason-checked policy exclusions for the 71 tests requiring identifiers, URI
    bases, anchors, or non-local resources. Static exhaustion rejects the
    descriptor; evaluation exhaustion maps to `LIMIT_EXCEEDED`.
24. Preserve protocol input presence independently from its JSON value.
    Missing `input` is a protocol validation error and never enters Runtime;
    explicitly present `null`, object, array, or scalar `input` reaches Runtime
    and is accepted or rejected only by the Action schema. HTTP and MCP apply
    bounded envelope decoding, reject duplicate members, and project the same
    normalized Runtime error contract. Malformed requests that enter Runtime
    remain auditable; requests rejected by the protocol boundary do not create
    Runtime or audit effects.
25. Freeze final acceptance by complete source content and mode, not only by a
    Git commit or tracked diff. The frozen digest includes tracked,
    untracked, deleted, executable, and symbolic-link state while excluding
    only its own T016 evidence directory. Full tests run after the freeze, and
    two fresh independent reviewers inspect the same digest. CI separately
    compares a complete before/after source snapshot around the aggregate gate
    suite.
26. Make Host construction and composition-reference ownership explicit.
    `NewHost` and `NewHostWithOptions` install a self-bound initialization
    marker; nil, zero, forged, or copied Host values are unavailable and fail
    closed. `Register` owns defensive copies. The one terminal Start attempt
    releases Host-owned Start callbacks, handler factories, and migration
    filesystem references on success, failure, or cancellation without mutating
    the caller's Registration. `module.Capability` remains an open validated
    namespaced string type with standard database, identity, authorization, and
    audit constants rather than a closed application taxonomy.

## Consumer Shape

```text
consumer/
  go.mod
  modary.yaml
  cmd/<app>/main.go
  tools/modary/main.go
  internal/project/project.go
  modules/<domain>/
  internal/generated/
```

Both entry points pass the same `internal/project.Definition` error-returning
provider. The application entry uses `appcmd`; the tool entry uses `projecttool`.
`internal/project.ApplicationMetadata()` supplies the same immutable value to
the Definition and `appcmd.Options`.

## Project Commands

- `verify`: validate metadata, Definitions, dependency graph, capabilities,
  Action schemas, migration declarations, and configured outputs without writes
  or startup. SQL migration contents are validated by the Host during startup.
- `generate`: write module graph, Action catalog, and optional TypeScript types.
- `generate --check`: compare expected bytes and report drift without writes.
- `build`: run verify and generated checks, then build the consumer command to
  `dist/<app.id>` or its configured output. Tests and optional frontend builds
  remain consumer CI responsibilities. The trusted Go compiler writes to a
  outside-project operating system temporary directory.
  Modary canonicalizes `TMPDIR`, rejects it when it is inside or resolves through
  a symlink into the project, and retains a file descriptor and filesystem
  identity for that directory and every ancestor through `/`. Every retained
  directory is revalidated, must be owned by the effective UID or root, and may
  be group- or other-writable only when root-owned and sticky. Darwin rejects any
  extended ACL at every retained level. The child staging directory must be
  effective-UID-owned with exact mode `0700`; Darwin also rejects an extended
  ACL on that child. Every other platform, including other Unix variants and Windows,
  fails Build because F0 has no validated ACL policy there. Unsupported
  platforms are cross-compile-only where covered; F0 claims no native Build,
  ACL, or rename runtime validation for them. Modary validates the staged file
  and copies it through the verified project Root before one sibling rename
  installs the configured output.

Generation renders the complete bounded set before mutation and installs each
changed file with one sibling rename. The rename is atomic only where the host
filesystem guarantees rename atomicity. It uses best-effort in-process rollback
rather than claiming a filesystem-wide or crash-atomic transaction. Same-root
operations serialize within one process by the filesystem identity captured when
the Project is loaded, not by pathname spelling; consumer automation serializes
separate tool processes. Filesystem mutation uses a verified `os.Root`. Build
sets `GO111MODULE=on`, `GOTOOLCHAIN=local`, `GOENV=off`, `GOWORK=off`, and an
empty `GOFLAGS`, then passes `-mod=readonly` and `-buildvcs=false`.
Build removes every inherited case-variant `TMPDIR` and `GOTMPDIR` entry from
the child environment, then sets both exactly once to the same canonical staging
parent whose descriptor and ancestry are retained and revalidated. An ambient
`GOTMPDIR` cannot override that parent.
Other inherited environment, the selected Go executable and toolchain,
consumer source, verified project pathname used for `command.Dir`, and same-UID pathname
replacement remain trusted inputs or deployment boundaries; Build is not a sandbox.
On Linux and Darwin, Build starts Go in an independent process group. After
`Start`, `waitid(WEXITED|WNOWAIT)` observes the leader without reaping it. While
the leader PID and PGID remain reserved, Build kills residual same-group
descendants after either a zero or non-zero exit, then calls `Cmd.Wait`.
When the leader exited successfully, pre-reap group cleanup succeeded, and the
context remains active, `exec.ErrWaitDelay` from `Cmd.Wait` is only a residual
inherited-pipe close backstop and does not fail Build. Writer, process-exit,
cancellation, and cleanup errors still fail. Context cancellation also targets
the same group. A trusted descendant that daemonizes or enters
another process group can escape cleanup, so this is not a strong process sandbox.

## Migration Strategy

- Preserve consumer-owned prototype assets in an independently owned downstream
  repository with a transfer manifest and checksums.
- Remove consumer-domain modules, UI, fixtures, contracts, CI, and release
  behavior from active Modary paths.
- Keep Modary's pre-release Adapter schema on the neutral execution-scope
  contract. The preserved downstream prototype owns its original migration
  lineage for future application work.

## Validation Strategy

- Focused TDD for lifecycle, typed capabilities, AppKit, project tooling,
  transports, and Adapter provisioning.
- Concurrency and cancellation conformance for Runtime dependencies,
  independent-deadline `AuditFailure`, non-cooperative startup rollback, and
  cleanup timeout overlap.
- A fail-closed public package inventory, privileged-internal allowlist, and
  forbidden sibling-Adapter and appcmd dependency fixtures.
- External module copied to a temporary directory with `GOWORK=off` and an
  absolute local `replace` to the current Modary checkout.
- Independent external provider and feature Modules sharing one consumer-owned
  package-level typed key, including undeclared, recreated-key, retained
  Resolver, and race conformance.
- Fake Node executables for the Node-free Go framework gate.
- HTTP/MCP/CLI conformance against one neutral write Action.
- Exact-boundary and one-above-boundary matrices for Action JSON byte, depth,
  node, and numeric-token limits through Runtime, HTTP, MCP, SQLite plans, and
  completed idempotency records.
- Draft 7 object/boolean, local-reference, schema structure, numeric-work,
  evaluation-work, mismatch-event, and MCP-wrapper boundary matrices, including
  flag-only fail-closed fuzzing.
- Missing input versus explicitly present `null`, object, array, and scalar
  conformance through HTTP and MCP, with Runtime and audit call-count checks.
- Linux/Darwin CLI token-path policy, other-platform pre-filesystem rejection,
  and standard-input fallback tests.
- A fake Go regression receives a symlink alias through ambient `TMPDIR` and a
  malicious project-path `GOTMPDIR`, and observes the same canonical staging
  parent in both child variables.
- Public `errors.Is`/`errors.As` conformance from an external consumer without an
  internal error helper.
- Repository neutrality scan and generated drift scan.
- Full Go tests, vet, native race, shuffled count-20 high-risk repeats, every
  registered fuzz target, six-target Linux/Darwin/Windows cross-builds, Windows
  platform-test compilation, native Darwin ACL tests, and optional neutral
  Console checks.
- A content-and-mode source snapshot around the complete CI gate suite and a
  content-addressed frozen-tree review snapshot for final acceptance.

## Risks

- Large API rewrite can hide regressions: retain Action Runtime tests and add an
  external end-to-end conformance test before deleting the integrated app.
- Partial-start cleanup can obscure the primary error: attempt every cleanup and
  return joined, inspectable errors.
- Consumer transfer can lose uncommitted work: checksum and compare every copied
  file before source removal.
- Global CLI cannot import arbitrary project code safely: use a consumer-owned
  pinned tool entry for F0 rather than runtime discovery.
- Technical F0 acceptance is local source proof. The current repository has no
  published version tag and no owner-selected redistribution license; remote
  distribution is a separate owner-approved release decision.
