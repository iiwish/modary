# F0 Known Limitations

1. Public Go APIs, generated formats, and Modary-specific HTTP/MCP schemas are
   alpha. Exact-version pinning and deliberate upgrades are required before v1.
2. The durable profile is one process and one SQLite database. It does not claim
   distributed transactions, multi-writer scaling, high availability, failover,
   or arbitrary storage-adapter atomicity. Privileged migration and transaction
   control is internal to the Host and official adapters, so an external custom
   durable adapter is not an F0 application-level extension point.
3. File-backed SQLite is supported only where ownership and mode metadata can be
   verified. Every directory ancestor is owned by the effective UID or root; a
   group- or other-writable ancestor is accepted only when it is root-owned and
   sticky. The final database directory remains effective-UID-owned and
   non-writable by group or other users. The official adapter fails closed on
   Windows; an ACL-aware adapter is required there. Root, same-UID processes,
   ACLs, mount replacement, and filesystems with untrustworthy stat metadata
   remain deployment boundaries.
4. Local Identity is suitable for bounded local or private deployments, not a
   complete internet-facing IAM system. MFA, SSO, password recovery, breached-
   password screening, IP/account rate limiting, and centralized revocation are
   consumer or future-adapter responsibilities.
5. RBAC provides explicit roles, actor/scope bindings, permissions, and row
   limits. It is not a general policy language and does not model delegation,
   hierarchy, attribute policy, or cross-service policy distribution.
6. Preview plans use a configurable in-process cleanup path and a five-minute
   default TTL. F0 provides no scheduler for retention, archival, or bulk purge.
7. Audit provides bounded structured events and transactional success records.
   Retention policy, export, signing, immutability outside SQLite, external SIEM
   delivery, and operator-facing audit UI are not framework features in F0.
8. HTTP API sessions and CSRF are included, but TLS termination, trusted proxy
   policy, security headers, origin policy, request-rate limiting, and deployment
   monitoring remain application or infrastructure responsibilities.
9. MCP implements the bounded initialization, discovery, and tool-call surface
   required for governed Actions. Streaming, resources, prompts, resumability,
   and broader protocol features are outside F0.
10. Module composition is static Go code. Dynamic plugins, runtime package
    discovery, marketplaces, hot reload, schedulers, and workflow engines are
    intentionally outside the framework contract.
11. Modary ships no application UI, scaffold generator, global CLI, container,
    or release executable. Consumers own those surfaces and may use the public
    AppKit, transport, appcmd, and projecttool packages to build them.
12. The current checkout proves local independent consumption. A remote module
    download is not part of F0 acceptance until the repository has a published
    version tag. The checkout has no owner-selected redistribution license, so
    local technical acceptance is not a public distribution grant.
13. Action handlers, Runtime dependencies, and installed actor-resolution,
    session, and bearer-token implementations are trusted process code and are
    expected to honor their contexts. Every Runtime and identity facade
    invocation holds a lifecycle lease. `Shutdown` revokes new leases and cancels
    active leased contexts, but Go cannot forcibly terminate a call that ignores
    cancellation. A `Shutdown` caller context bounds only that caller's wait;
    expiration returns without starting Host cleanup while a lease remains
    active. The gate stays revoked, new calls through retained facades fail
    closed, and the exactly-once shutdown coordinator starts Host cleanup
    automatically when the non-cooperative call returns. Another `Shutdown`
    call is not required.
    Cleanup guarantees invocation-start order, not completion order: LIFO within
    a Module and reverse dependency order across Modules.
    A timed-out cleanup callback may overlap later callbacks and provider
    cleanup. Trusted cleanup
    callbacks must honor cancellation and stop using dependent services before
    returning; a callback that ignores cancellation can retain its goroutine and
    resources until it returns. Detached Audit and each `AuditFailure` callback
    retain separate bounded waits.
    `AuditFailure` receives an independent deadline context and cannot replace
    the primary Runtime result.
    The HandlerFactory Resolver is valid only during the factory call; retaining
    it is a contract violation and later resolution fails with
    `module.ErrInvalidResolver`.
14. Generated files are each installed with one sibling rename, which is atomic
    only where the host filesystem guarantees rename atomicity. A multi-file
    generated set is not a filesystem-wide or crash-atomic transaction. Rollback
    is best-effort within the running process. Same-root operations serialize
    within one process by the filesystem identity captured when the Project is
    loaded, not by pathname spelling. The lock remains process-local, so
    consumer automation must prevent concurrent tool processes.
    Unsupported-platform portability is cross-compiled only where covered;
    native rename runtime behavior is not verified there.
15. CLI `--token-file <path>` is supported only on Linux and Darwin. The token
    must remain a regular file owned by the effective UID with exact mode `0400`
    or `0600`; Darwin also queries the retained open file descriptor and rejects
    any extended ACL. Every other operating system rejects a token path
    before any filesystem access. Only `--token-file -`, reading
    standard input, remains available there.
16. Project filesystem validation and artifact mutation use a verified
    `os.Root`. Compiler output first lands in an outside-project operating system
    temporary directory and is validated before being copied through that Root.
    Before Go runs, Modary canonicalizes `TMPDIR`, rejects it when it is inside
    or resolves through a symlink into the project, and retains a file descriptor
    and filesystem identity for that directory and every ancestor through `/`.
    Every retained directory is revalidated, must be owned by the effective UID
    or root, and may be group- or other-writable only when root-owned and sticky.
    Darwin also rejects any extended ACL at every retained level. The child
    staging directory must be effective-UID-owned with exact mode `0700`; Darwin
    also rejects an extended ACL on that child. Build removes every inherited
    case-variant `TMPDIR` and `GOTMPDIR` entry from the child environment, then
    sets both exactly once to the same canonical staging parent whose descriptor
    and ancestry are retained and revalidated. An ambient `GOTMPDIR` cannot
    override that parent. Every other platform, including other Unix variants
    and Windows, fails Build because F0 has no validated ACL policy there. Such
    platforms are cross-compile-only where covered.
    F0 claims no native Build, ACL, or rename runtime validation for them.
17. With cooperative output writers, compiler cancellation and inherited output
    pipes have a bounded wait. Caller-supplied `io.Writer.Write` must return; Go
    cannot interrupt a blocked call, which can therefore keep Build waiting.
18. Build sets `GO111MODULE=on`, `GOTOOLCHAIN=local`, `GOENV=off`, `GOWORK=off`,
    and an empty `GOFLAGS`, and passes `-mod=readonly` and `-buildvcs=false`.
    Other inherited environment, the selected Go executable and toolchain, and
    consumer source remain trusted inputs. The verified project pathname used as
    `command.Dir`, root or same-UID pathname and mount replacement, and hostile
    build inputs are not a sandbox boundary.
19. On Linux and Darwin, Build starts Go in an independent process group. After
    `Start`, `waitid(WEXITED|WNOWAIT)` observes the leader without reaping it.
    While the leader PID and PGID remain reserved, Build kills residual same-group
    descendants after either a zero or non-zero leader exit, then calls
    `Cmd.Wait`. When the leader exited successfully, pre-reap group cleanup
    succeeded, and the context remains active, `exec.ErrWaitDelay` from
    `Cmd.Wait` is only a residual inherited-pipe close backstop and does not fail
    Build. Writer, process-exit, cancellation, and cleanup errors still fail.
    Context cancellation also targets the same group. A trusted descendant that
    daemonizes or enters another process group can escape cleanup, so this is not
    a strong process sandbox. Build does not claim byte-reproducible binaries or
    strong process isolation.
20. All extension and dependency causes cross one opaque public error-chain
    boundary. Standard `errors.Is` and `errors.As` perform bounded matching
    without calling caller-defined `Error`, `Is`, `As`, or `Unwrap`. An external
    consumer uses those public error chains without importing an internal helper.
21. `appcmd.Options.Stdout` and `Stderr` are trusted, cooperative dependencies.
    Their `Write` calls must return; context cancellation and shutdown timeouts
    cannot interrupt a blocked writer.
22. SQLite nested transactions join the outer transaction and do not create
    savepoints. Any inner error or panic makes the complete outer unit
    rollback-only, even when outer code handles the returned error.
23. One Module migration source supports at most 256 root entries, 1 MiB per SQL
    file, and 16 MiB in aggregate; applied history is capped at 256 rows. Larger
    schemas must be consolidated before release or require a future profile with
    a separately reviewed resource policy.
24. An Action schema, request input, or Handler plan payload is one separate
    Action JSON document. A Preview summary or Result data is also one separate
    Action JSON document. A persisted Action JSON value is likewise one separate
    Action JSON document. Every Action JSON document has independent limits of
    at most 1 MiB
    (1,048,576) source bytes, 256 nested object or array containers including the
    root container, 65,536 JSON value nodes including containers and scalar
    values but excluding object member names, and 4,096 source bytes for any one
    JSON number token. Every document is valid UTF-8 and contains
    exactly one JSON value, with no duplicate object member names.
    HTTP and MCP request envelopes have independent byte budgets.
    Their 2 MiB defaults can carry one complete
    maximum-size Action JSON document plus required envelope fields.
    Every extracted Action document is revalidated against the
    per-document Action limits. Larger Action values require decomposition or a
    separately reviewed future profile;
    increasing an envelope budget does not increase an Action document limit.
25. Action schemas are JSON Schema Draft 7 with object or boolean roots. One
    immutable executable SchemaGraph admits schema-syntax locations and the
    unique closure of local JSON Pointer references. Empty or non-local
    references, named anchors, schema `id`/`$id`, URI bases, external registries,
    remote retrieval, and file access are outside F0. Actual schema roots are
    validated offline against a pinned Draft 7 metaschema. Patterns use the Go RE2
    grammar; ECMA-262-only constructs such as look-around and backreferences
    are outside this profile.
    Static admission permits at most 2,048 schema nodes, 512 entries in one
    schema collection, 256 enum values, 16 KiB for one encoded `const` or
    `enum`, 4 KiB for one pattern, 1,024 same-instance schema visits, and 64 Mi
    cumulative numeric compilation work units. Each validation is flag-only,
    has 64 Mi work units, 4,096 mismatch events, and 4,096 active evaluation frames,
    and returns no dependency diagnostic tree. Schemas above the static
    profile must be decomposed before descriptor construction; evaluation
    exhaustion is `LIMIT_EXCEEDED`.
    MCP retains every Action collection, enum, literal, pattern, same-instance,
    and evaluation limit. Its fixed compilation profile adds 128 schema nodes,
    a 1 Mi numeric wrapper allowance, and 4,096 compile-only JSON value nodes.
    The pinned official Draft 7 mandatory corpus executes 223 cases and 856
    instance tests; an exact manifest excludes 34 cases and 71 tests requiring
    identifiers, URI bases, anchors, or non-local resources.
26. A running Host releases its copies of Start callbacks, handler factories,
    and migration filesystem references after the terminal Start attempt. The
    original Definition and Registration remain consumer-owned values. Any
    credentials captured by those original callbacks remain reachable for as
    long as the consumer retains them; Modary cannot erase caller-owned Go
    closures or their captured memory.
