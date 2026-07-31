# Modary

Modary is a Go-first framework for building governed modular applications.
Consumers define their own modules and typed Actions, then expose the same
Action runtime through application commands, HTTP, MCP, or their own adapters.
Modary owns lifecycle, authorization, Preview/Execute binding, idempotency,
transactions, and audit semantics; the consumer owns its domain, composition,
policy, data, UI, and release artifacts.

Modary F0 is a pre-v1 alpha contract. Pin an exact version after tags are
published and review API changes before upgrading. The current checkout proves
consumption through an independent Go module using a local `replace`; it does
not claim a remote release. This checkout has no owner-selected redistribution license,
so F0 acceptance does not grant public redistribution rights.

## Architecture

Every governed business Action state change converges on `action.Runtime`:

```text
consumer transport / appcmd
            |
            v
      governed Runtime
  intent authorization
  Preview + plan binding
  impact authorization
  idempotency reservation
  transaction + reauthorization
  handler + required audit
```

Module composition is explicit consumer Go code. A `module.Registration`
contains a pure, inspectable Definition and runtime callbacks. The Host validates
the complete dependency graph before startup, gives each module only its
declared capabilities, applies its declared migrations, assigns Action ownership
itself, and starts partial or completed cleanup in deterministic reverse
dependency order. Handler factories receive a sealed read-only service resolver
rather than an installation scope.
`module.Capability` is an open named string contract:
framework services use the standard database, identity, authorization, and
audit constants, while consumers may define validated namespaced capabilities
for their own typed service keys. A provider and every consumer share one
package-level typed key; recreating the same name does not recreate its identity.
The HandlerFactory Resolver is valid only during the factory call. Factories
resolve and retain service values, never the Resolver itself; later use fails
with `module.ErrInvalidResolver`.

`module.Host` values are constructed with `module.NewHost` or
`module.NewHostWithOptions`; nil, zero, or copied values are unavailable and
fail closed. Registration is defensive: the Host owns its copy and releases
Start callbacks, handler factories, and migration filesystem references after
its single terminal Start attempt. The caller's original Registration remains
unchanged and under consumer lifetime management.

## Packages

| Package | Responsibility |
|---|---|
| `action` | Typed Action descriptors, schema validation, plans, idempotency, Runtime |
| `module` | Pure definitions, dependency graph, typed capabilities, lifecycle Host |
| `appkit` | Opaque application assembly and read-only runtime projections |
| `appcmd` | Consumer-owned `serve`, `run`, and `version` command orchestration |
| `transport/httpapi` | Explicit health, Action HTTP API, MCP, and static asset handlers |
| `projecttool` | Pure verify, deterministic generate/check, and consumer build workflow |
| `scope`, `identity`, `authz`, `audit`, `database` | Narrow framework contracts |
| `adapters/*` | Explicit SQLite, local Identity, RBAC, and SQL Audit implementations |
| `internal/*` | Framework-owned runtime mutation, database control, callback guards, assembly keys, and quality gates; never a consumer import surface |

Official adapters provision no principals, credentials, roles, bindings,
permissions, secrets, or domain records unless the consumer supplies them.

## Verify The Framework

Go 1.26 or newer is required by the current module contract. Repository
contributors also need Make, Git, a POSIX shell, `find`, `xargs`, and `rg`
(ripgrep) to run the complete acceptance gates. The framework and independent
consumer gates are Node-free; Node.js, npm, and pnpm are not prerequisites.

```bash
make bootstrap
make acceptance
make ci
```

`make acceptance` runs formatting and module-drift checks, framework and copied-
out consumer tests, vet, project verification, generated-artifact checks,
neutrality enforcement, `panic(nil)` compatibility, native builds, and
Linux, Darwin, and Windows cross-builds for amd64 and arm64. It also compiles
the platform-specific Windows command, project-tool, and SQLite tests and the
Darwin file-policy tests without writing artifacts into the repository.
`make ci` adds race tests, shuffled repeated high-risk tests, and fuzz smoke
tests for project manifests, Action JSON decoding, JSON Schema compilation and
evaluation, HTTP/MCP protocol decoding, and, on Darwin, both ACL parsers. The
CI wrapper compares a complete content-and-mode source snapshot before and
after the complete gate suite. GitHub CI runs the complete gate on Ubuntu and
a native Darwin/arm64 contract gate on `macos-15`.

## Independent Consumer

[`testdata/external-consumer`](testdata/external-consumer) is a separate Go
module and the executable conformance contract. It owns its composition root,
migration, governed Action, explicit provisioning, static assets, command, and
project tool. The copied-out test moves it outside this repository, disables
`go.work` discovery, installs failing Node-family shims, and completes the full
Node-free Go framework verify/generate/check/test/build/run workflow.

```bash
cd testdata/external-consumer
GOWORK=off go run ./tools/modary verify
GOWORK=off go run ./tools/modary generate --check
GOWORK=off go test ./...
GOWORK=off go run ./cmd/counter-console version
```

Consumers use the canonical `appkit.DefinitionProvider` contract from both their
application entry point and pinned project-tool entry point;
`appcmd.DefinitionProvider` and `projecttool.DefinitionProvider` are aliases. A
pure metadata value is shared by the Definition and application command options,
so help and version do not construct adapters. Project metadata and artifact
paths live in `modary.yaml`; module discovery never scans source files or starts
a module during inspection or generation.

## Contract Boundaries

- Public APIs are available only from non-`internal` packages. Consumers never
  import `internal` packages or receive raw handlers, registries, databases, or
  lifecycle authority from `appkit.Application`. The repository import matrix
  explicitly inventories public production packages, allowlists each privileged
  internal dependency, and rejects command-to-transport/tooling and sibling
  Adapter coupling.
- Runtime methods may run concurrently. Handlers, authorization, audit,
  identity, database, Clock, and `AuditFailure` implementations are trusted
  concurrency-safe dependencies that honor their supplied context and return
  promptly after cancellation.
  Each `AuditFailure` call receives an independent deadline context and cannot
  replace the primary Runtime result.
- Cleanup guarantees invocation-start order: LIFO within a Module and reverse
  dependency order across Modules. A timed-out cleanup callback may overlap
  later callbacks and provider cleanup. Trusted callbacks honor cancellation
  and stop using dependent services before returning; completion order is not
  guaranteed after a timeout.
- The first `Host.Assemble` call caches its immutable facade set or failure.
  Every governed Runtime call and every actor-resolution, session, or bearer-
  token facade call holds a lease in the shared Host-owned assembly gate.
  `Application.Shutdown` delegates to the Host shutdown sequence, which rejects
  new leases, cancels active leased contexts, and drains all active leased calls
  before Host cleanup begins. New calls through a retained Runtime or identity
  facade fail closed after revocation and never reach a cleaned Module service.
  The authenticated
  `httpapi.NewAPI` and `httpapi.NewMCP` constructors require `application.Ready()`
  to be true. If an active facade ignores cancellation, the caller context bounds
  only that caller's wait. The Host-owned gate remains revoked; the shutdown
  sequence waits independently and starts Host cleanup automatically when the
  final lease is released.
- Execution isolation uses the validated `scope.Execution{Kind, ID}` value;
  Modary treats both identifiers as opaque. Domain tenancy concepts remain
  consumer-owned.
- The durable F0 profile uses one SQLite database so the idempotency record,
  business mutation, successful result, and allowed audit event can share one
  transaction. The official adapter installs plan storage, idempotency storage,
  and transaction ownership as one framework-private persistence bundle.
  Consumer modules receive a narrow governed `database.Access`: reads are
  available through its bounded SQL surface and writes require the Runtime's
  transaction-bound context. Raw database and transaction control stay private
  to framework assembly and official durable adapters. F0 supports the official
  SQLite durable profile; installing an external privileged storage adapter is
  not a public extension contract. In the file-backed
  SQLite profile, every directory ancestor is owned by the effective UID or
  root; a group- or other-writable ancestor is accepted only when it is root-
  owned and sticky. The final database directory remains effective-UID-owned
  and non-writable by group or other users.
- Public database reads accept one `SELECT`; governed writes accept one
  `INSERT`, `UPDATE`, or `DELETE` and reject DDL, multiple statements,
  transaction control, and rollback conflict forms before resolving the
  framework-internal write executor. Nested SQLite transactions
  join the outer unit without savepoints; an inner failure makes the outer unit
  rollback-only. Migration sources and applied history are bounded to 256
  entries, 1 MiB per file, and 16 MiB per source, and the full source is
  validated before database effects. Migration SQL also rejects every bare or
  quoted `temp.` schema reference because connection-local objects are not a
  durable migration result.
- An Action schema, request input, or Handler plan payload is one separate Action
  JSON document. A Preview summary, Result data, or persisted Action JSON value
  is also one separate Action JSON document. Every Action JSON document has
  independent limits of at most 1 MiB
  (1,048,576) source bytes, 256 nested object or array containers including the
  root container, 65,536 JSON value nodes including containers and scalar values
  but excluding object member names, and 4,096 source bytes for any one
  JSON number token. Every document is valid UTF-8 and contains exactly one JSON value.
  It has no duplicate object member names. HTTP and MCP request envelopes have
  independent byte budgets. Their 2 MiB defaults can carry one complete
  maximum-size Action JSON document plus required envelope fields.
  Every extracted Action document is revalidated against the
  per-document Action limits.
- Action schemas use JSON Schema Draft 7 and may have an object or boolean root.
  One immutable executable SchemaGraph covers schema-syntax locations and the
  unique closure of arbitrary local JSON Pointer references. A `$ref` must be a URI fragment
  that decodes to the root JSON Pointer (`#`) or an absolute JSON Pointer
  (`#/...`, including valid percent-encoding). Empty, relative, query, anchor,
  file, and network references are rejected. Every actual schema node prohibits
  `id` and `$id`; the same keys remain inert inside literal data unless a local
  reference targets that object as a schema. The root and every hidden reference
  root are checked offline against a pinned Draft 7 metaschema, with no external
  registry, file access, or network I/O.
  In addition to the Action JSON source limits, each SchemaGraph is admitted
  under a fixed executable profile: 2,048 schema nodes, 512 entries in one schema
  collection, 256 enum values, 16 KiB for one encoded `const` or `enum`, 4 KiB
  for one Go RE2 pattern, 1,024 same-instance schema visits, and 64 Mi cumulative
  numeric compilation work units. Each validation receives 64 Mi work units,
  4,096 mismatch events, and 4,096 active evaluation frames. Validation is
  flag-only and does not construct or expose a dependency diagnostic tree.
  Static profile exhaustion rejects descriptor construction; evaluation
  exhaustion is `LIMIT_EXCEEDED`, and an ordinary mismatch is
  `VALIDATION_FAILED`.
  The pinned official Draft 7 mandatory corpus contains 37 files, 257 cases, and
  927 instance tests. Modary executes 223 cases and 856 tests; 34 exact policy
  exclusions account for the 71 tests that require identifiers, URI bases,
  anchors, or non-local resources. MCP tool compilation retains every Action
  collection, enum, literal, pattern, same-instance, and evaluation limit while
  adding only 128 framework-owned schema nodes, 1 Mi numeric wrapper allowance,
  and 4,096 compile-only JSON value nodes for its fixed wrapper and collision-free
  hidden-reference copies.
- HTTP and MCP require the `input` member at the protocol boundary. Its absence
  is a protocol validation failure and does not call or audit the Runtime.
  Explicit `null`, `{}`, arrays, and scalar JSON values are distinct present
  inputs; they reach Runtime and are accepted or rejected by the Action schema.
- Action descriptors declare namespace-qualified consumer error codes and one
  closed semantic kind. Runtime validates the source, declaration, kind, and
  public message before HTTP, MCP, CLI, or audit sees it. Authorization denial
  is owned by policy; malformed or operational errors fail closed to a generic
  internal result. Code, kind, and safe message stay aligned across channels.
- The Action command derives the actor from the installed identity adapter and
  never accepts a caller-asserted actor identity. CLI `--token-file <path>` is
  supported only on Linux and Darwin. The token must remain a regular file owned
  by the effective UID with exact mode `0400` or `0600`; Darwin also queries the
  retained open file descriptor and rejects any extended ACL. Every other
  operating system rejects a token path before any filesystem access. Only
  `--token-file -`, which reads standard input, remains available there.
- HTTP, MCP, and command adapters validate every actor returned by an installed
  authenticator before catalog discovery or Action execution. All extension and
  dependency causes cross one opaque public error-chain boundary. Standard
  `errors.Is` and `errors.As` perform bounded matching without calling caller-
  defined `Error`, `Is`, `As`, or `Unwrap`; an external consumer needs no
  internal helper import. `appcmd.HandlerFactory` receives the active Serve
  context and fully started Application. It must observe cancellation
  cooperatively. `appcmd.Options.Stdout` and `Stderr` are trusted, cooperative
  dependencies: `Write` must return because context cancellation and shutdown timeouts
  cannot interrupt a blocked writer.
- UI composition is explicit. Consumers pass handlers or built assets; Modary
  has no embedded application UI or frontend toolchain. `httpapi.NewSPA`
  snapshots a bounded regular-file tree during construction and serves only the
  immutable snapshot.
- Project inspection, generation, and checking are deterministic, bounded, and
  cancelable. Generation renders the complete set before mutation and installs
  each changed file with one sibling rename, which is atomic only where the host
  filesystem guarantees rename atomicity. The set is not a filesystem-wide or crash-atomic transaction.
  Same-root operations serialize within one process by the filesystem identity
  captured when the Project is loaded, not by pathname spelling. Consumer
  automation must serialize separate project-tool processes.
- Project filesystem validation and artifact mutation use a verified `os.Root`.
  Build runs the trusted Go compiler with the verified project pathname as its
  working directory and writes compiler output to an outside-project operating
  system temporary directory. Before invoking Go, Modary canonicalizes `TMPDIR`,
  rejects it when it is inside or resolves through a symlink into the project,
  and retains a file descriptor and filesystem identity for that directory and
  every ancestor through `/`. Every retained directory is revalidated, must be
  owned by the effective UID or root, and may be group- or other-writable only
  when root-owned and sticky. Darwin also rejects any extended ACL at every
  retained level. The child staging directory must be effective-UID-owned with
  exact mode `0700`; Darwin also rejects an extended ACL on that child.
  Build removes every inherited case-variant `TMPDIR` and `GOTMPDIR` entry from
  the child environment, then sets both exactly once to the
  same canonical staging parent whose descriptor and ancestry are retained and revalidated.
  An ambient `GOTMPDIR` cannot override that parent.
  Every other platform, including other Unix variants and Windows, fails Build
  because F0 has no validated ACL policy there. Such platforms are
  cross-compile-only where covered.
  F0 claims no native Build, ACL, or rename runtime validation for them.
- Build sets `GO111MODULE=on`, `GOTOOLCHAIN=local`, `GOENV=off`, `GOWORK=off`,
  and an empty `GOFLAGS`, then invokes `go build` with `-mod=readonly` and
  `-buildvcs=false`. Other inherited environment, the selected Go executable
  and toolchain, and consumer source remain trusted inputs; Build is not a sandbox.
  Modary validates the staged output, copies it through `os.Root` to a sibling
  temporary file, and installs it by rename. Build rechecks root identity but
  does not claim byte-reproducible binaries or isolate same-UID pathname changes.
- On Linux and Darwin, Build starts Go in an independent process group. After
  `Start`, `waitid(WEXITED|WNOWAIT)` observes the group leader without reaping it.
  While the leader PID and PGID remain reserved, Build kills residual same-group
  descendants after either a zero or non-zero leader exit, then calls `Cmd.Wait`.
  When the leader exited successfully, pre-reap group cleanup succeeded, and the
  context remains active, `exec.ErrWaitDelay` from `Cmd.Wait` is only a residual
  inherited-pipe close backstop and does not fail Build. Writer, process-exit,
  cancellation, and cleanup errors still fail. Context cancellation also
  targets the same group. A trusted descendant that daemonizes or enters
  another process group can escape cleanup, so this is not a strong process sandbox.
  With cooperative output writers, compiler cancellation and inherited output
  pipes have a bounded wait. Caller-supplied `io.Writer.Write` must return; Go
  cannot interrupt a blocked call.

See [`docs/framework-f0.md`](docs/framework-f0.md) for the full framework
contract, [`docs/f0-known-limitations.md`](docs/f0-known-limitations.md) for the
intentional F0 boundary, and [`docs/f0-acceptance-report.md`](docs/f0-acceptance-report.md)
for acceptance evidence.
