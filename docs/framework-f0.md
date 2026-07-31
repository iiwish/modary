# Modary Framework F0 Contract

- Status: Accepted implementation contract
- Stability: Alpha
- Go module: `github.com/iiwish/modary`
- Runtime profile: single-process, single SQLite database

## Product Boundary

Modary is an independently consumable application framework. It defines the
governance and lifecycle mechanics shared by modular applications; it does not
define a consumer product, business schema, navigation model, bootstrap user,
default policy, frontend build, or release executable.

A consumer owns:

- one explicit Go composition function;
- feature and adapter selection;
- domain Actions, migrations, policy, provisioning, and configuration;
- HTTP route mounting, UI assets, branding, and deployment;
- a pinned project-tool entry point and its release command.

The framework owns:

- pure Module definitions and dependency verification;
- typed, capability-scoped services and lifecycle cleanup;
- typed Action contracts and compiled schema validation;
- authorization, Preview plans, idempotency, transactions, and audit ordering;
- opaque application assembly and explicit transport adapters;
- bounded deterministic project inspection, generation, and checking, plus a
  cancelable root-verified consumer build workflow.

The production import matrix uses an explicit public package inventory and a
per-package privileged-internal allowlist. Unknown public packages fail closed;
`appcmd` cannot depend on transports or project tooling, and official Adapters
cannot depend on sibling Adapters. Action and transport packages reach the JSON
Schema implementation only through `internal/jsonschema`; direct engine imports
are not allowlisted.

## Composition

`appkit.DefinitionProvider` is the canonical error-returning composition
contract. `appcmd.DefinitionProvider` and `projecttool.DefinitionProvider` are
aliases of that contract, so one consumer function is used by both the
application and the consumer-owned project-tool entry point. The command
identity is a pure value reused by the Definition and `appcmd` options:

```go
func ApplicationMetadata() appkit.Metadata {
	return appkit.Metadata{
		ID: "example-app", Name: "Example App", Version: "0.1.0",
	}
}

func Definition() (appkit.Definition, error) {
	storage, err := sqlite.Module(sqlite.Options{Path: "data/example.db"})
	if err != nil {
		return appkit.Definition{}, fmt.Errorf("configure SQLite: %w", err)
	}
	return appkit.Definition{
		Metadata: ApplicationMetadata(),
		Modules: []module.Registration{
			storage,
			identities,
			authorization,
			auditLog,
			feature,
		},
	}, nil
}

func CommandOptions() appcmd.Options {
	return appcmd.Options{
		Metadata: ApplicationMetadata(),
		Handler:  NewHTTPHandler,
	}
}
```

Official Adapter factories validate and copy their options while constructing a
Registration, but perform no I/O, random generation, password hashing, migration
read, handler construction, or Module startup. Consumer Registrations are
validated by the Host and `projecttool`; inspection never invokes their runtime
callbacks. `projecttool` can therefore inspect the same Definition without
creating runtime state. Consumers return Adapter construction errors with
operation context instead of converting them to panics.

Every Module declares a `modary.module/v1alpha2` manifest with a SemVer version,
typed `module.ModuleType`, required capabilities, and provided capabilities.
`module.ModuleTypeFeature` and `module.ModuleTypeAdapter` are the supported
manifest types. `module.Capability` is an open named string type. The standard
framework values are `module.CapabilityDatabase`, `CapabilityIdentity`,
`CapabilityAuthorization`, and `CapabilityAudit`; consumer modules may declare
validated namespaced capabilities and bind them to typed `module.Key[T]`
values. A provider and its consumers import one package-level typed key;
recreating its name and type produces a different identity and fails closed.
The Host verifies the complete graph and Action catalog before side
effects. During startup a Module
can resolve only declared requirements and capabilities, provide only declared
capabilities, and register process cleanup only while its Start callback is
active. The Host applies each matching migration set after the database becomes
available and before constructing that Module's Action handlers. Handler
factories receive a sealed read-only `module.Resolver`, not the mutable startup
scope. The HandlerFactory Resolver is valid only during the factory call.
Factories resolve and retain service values rather than the Resolver; retained
use returns `module.ErrInvalidResolver`.

Hosts are constructed only through `module.NewHost` or
`module.NewHostWithOptions`. Nil, zero, forged-initialization, or copied Host
values report `module.StateUnavailable`; stateful public operations return
`module.ErrHostUnavailable` without reaching lifecycle state. `Register`
defensively copies the manifest, Action descriptors, callback slices, and
migration declarations. After the one permitted Start attempt reaches success,
failure, or cancellation, the Host releases its Start callbacks, handler
factories, and migration filesystem references while retaining only static
catalog metadata. It never mutates the caller's Registration. The consumer owns
the lifetime of its original Definition and any credentials captured by its
callbacks.

Startup failure revokes partially constructed services and Actions and attempts
all cleanup. Cleanup guarantees invocation-start order: LIFO within a Module and
reverse dependency order across Modules.
A timed-out cleanup callback may overlap later callbacks and provider cleanup,
so trusted callbacks honor cancellation and stop using dependent services
before returning. Completion order is not guaranteed after a timeout. The first
`Host.Assemble` call caches either its immutable facade set or its failure.
Every returned Runtime, actor-resolution, session, and bearer-token
facade shares one Host-owned assembly gate and lifecycle domain. `Shutdown`
delegates to the Host shutdown sequence, which rejects new leases, cancels active
leased contexts, and waits for all active leases to release before invoking
cleanup exactly once. New calls through a retained Runtime or identity facade
fail closed after revocation and never reach a cleaned Module service. Each
caller context bounds only that caller's wait for shutdown. If an active facade
ignores cancellation, `Shutdown` returns the caller context error while the gate
stays revoked. The Host shutdown sequence waits independently and starts cleanup
automatically when the final lease is released; another `Shutdown` call is not
required. Once cleanup begins, each cleanup callback has an independent timeout
so a non-cooperative callback cannot prevent later callbacks from being
attempted.

`action.Runtime` methods may run concurrently. Handlers, authorization, audit,
identity, database, and Clock implementations are trusted concurrency-safe
dependencies that honor their supplied contexts and return promptly after
cancellation. `AuditFailure` has the same contract and receives an
independent deadline context for every notification; it cannot replace the
primary Runtime result.

Panic containment uses callback completion state rather than the recovered value.
This keeps lifecycle, Runtime, transaction, transport, command, and tooling
boundaries correct for both ordinary panic values and `panic(nil)`, including
the legacy behavior selected by `GODEBUG=panicnil=1`.

## Governed Actions

An Action descriptor fixes its ID, version, permission, allowed channels,
Preview policy, idempotency requirement, audit level, strict input, Preview, and
output schemas, and its consumer-owned public error codes. `action.Channel` is
an open named string type; the built-in surfaces use `action.ChannelCLI`,
`action.ChannelHTTP`, and `action.ChannelMCP`, while consumers may declare
additional validated channels. Each custom error is
a bounded namespace-qualified code paired with one closed semantic `ErrorKind`.
Framework-owned codes cannot be redeclared, and custom `internal` errors are not
valid contracts. Registration canonicalizes and compiles the descriptor once;
the contract hash binds every governance field, including error code and kind.
The public catalog returns defensive copies and never exposes a Handler.

An Action schema, request input, or Handler plan payload is one separate Action
JSON document. A Preview summary, Result data, or persisted Action JSON value is
also one separate Action JSON document. Every Action JSON document has
independent limits of at most 1 MiB (1,048,576) source bytes,
256 nested object or array containers including the root container,
65,536 JSON value nodes including containers and scalar values but excluding
object member names, and 4,096 source bytes for any one JSON number token. Every
document is valid UTF-8, contains exactly one JSON value, and has no
duplicate object member names. HTTP and MCP request envelopes have
independent byte budgets. Their 2 MiB defaults can carry one complete
maximum-size Action JSON document plus required envelope fields.
Every extracted Action document is revalidated against the
per-document Action limits.

Action schemas implement JSON Schema Draft 7 with object and boolean roots. The
compiler clones the source once and builds one immutable executable SchemaGraph.
That graph contains every schema-syntax location plus the unique closure reached
through arbitrary local JSON Pointer references; the Action compiler, static profiler, and
MCP embedder consume the same graph.

A `$ref` must be a URI fragment that decodes to the root JSON Pointer (`#`) or
an absolute JSON Pointer (`#/...`, including valid percent-encoding). Empty,
relative, query, named-fragment, file, and network references are rejected.
Every actual schema node prohibits `id` and `$id`. An object containing those
keys inside `const`, `enum`, or another annotation remains literal data unless a
local reference targets it as a schema. The graph root and every hidden
reference root are validated offline against an embedded Draft 7 metaschema
whose SHA-256 is pinned before compilation. There is no external schema
registry, file access, or network I/O.

The executable profile allows at most 2,048 schema nodes, 512 entries in one
schema collection, 256 enum values, 16 KiB for one encoded `const` or `enum`,
4 KiB for one Go RE2 regular-expression pattern, 1,024 same-instance schema
visits, and 64 Mi cumulative numeric compilation work units across constraints
and numeric `const` or `enum` literals. Draft 7 integer-valued limits accept
mathematically integral forms such as `1.0` and `1e1`. Schema numbers retain
exact native JSON equality and rational comparison semantics.

Each validation call receives 64 Mi work units, 4,096 mismatch events, and 4,096 active evaluation frames.
Work is charged for recursive visits,
collection iteration, actual key and string bytes, Go RE2 evaluation, instance
numbers, and compiled numeric operands. Validation is flag-only and never
constructs or exposes the dependency engine's diagnostic tree. Static profile
exhaustion rejects Action descriptor construction. Evaluation resource
exhaustion maps to `LIMIT_EXCEEDED`; a completed non-match maps to
`VALIDATION_FAILED`. Validators are immutable and concurrency-safe.

MCP rebases references from that SchemaGraph. Structural targets remain direct;
a target originating inside literal or unknown-keyword data is copied to one
collision-free framework annotation while its original literal is unchanged.
Tool-schema compilation retains every Action collection, enum, literal,
pattern, same-instance, and evaluation limit. Its fixed wrapper adds exactly
128 schema nodes, a 1 Mi numeric wrapper allowance, and 4,096 compile-only JSON
value nodes for wrapper structure and hidden-reference copies.

The vendored official Draft 7 mandatory corpus is pinned by commit and snapshot
digest. It contains 37 files, 257 cases, and 927 instance tests. Modary executes
223 cases and 856 tests. An exact manifest accounts for the 34 excluded cases
and 71 tests that require schema identifiers, URI bases, anchors, or non-local
resources, and verifies that each excluded schema still fails for its declared
F0 policy.

HTTP and MCP require an explicit `input` member. A missing member is a protocol
validation failure and does not enter or audit Runtime. Present `null`, `{}`,
array, string, number, and boolean values remain distinct JSON documents and
reach Runtime, where the Action schema decides whether they are valid. Direct
Runtime callers receive the same Action JSON and schema validation, including a
rejected audit event for a malformed request.

Preview execution canonicalizes and validates the envelope and input before
planning, then performs intent authorization, handler planning, impact
authorization and constraint checks. Plan persistence and its required Preview
audit record share a transaction. Plans bind the Action contract, actor, channel,
execution scope, canonical input, impact, snapshot, authorization fingerprint,
and expiration.

Write execution validates the request and current intent before looking up an
idempotency result. A new write resolves and authorizes its plan, then repeats
intent and impact authorization inside the write transaction before reserving
the idempotency key or calling the Handler. The business mutation, completed
result, and allowed audit event share that transaction. A completed replay is
reread and reauthorized inside a transaction before its stored result is
disclosed. Denied and failed audit events use a detached bounded context after
rollback and report persistence failure without replacing the primary error.

Framework-internal database control owns commit, rollback, administrative SQL,
and migration execution. Consumer handlers receive only the narrow governed
`database.Access`. Its write methods fail outside the transaction-bound context
supplied by the Runtime. Reads accept one `SELECT`; writes accept one `INSERT`,
`UPDATE`, or `DELETE`. Multiple statements, DDL, administration, transaction
control, and executable rollback conflict forms fail closed before reaching the
backend or resolving its transaction-bound write executor. Returned SQL results
and rows are guarded against nil, typed-nil,
panicking, and failing dependency implementations. The framework-provided
capability cannot be asserted to privileged control or a raw database handle. A
private typed service key connects official durable adapters to Host assembly;
the key and control type are unavailable to external Modules and cannot be
reached through the reflected method set of `module.Scope` or
`module.Resolver`.

The transaction callback is synchronous and exactly once. The official SQLite
owner supplies private operation-correlated completion proof. Runtime preserves
a business rejection only after confirmed rollback; uncertain rollback,
rollback failure, commit failure, forged or wrapped proof, and callback contract
violations are `INTERNAL_ERROR`. Nested SQLite transactions join the outer unit
without a savepoint. An inner error or panic marks the outer transaction
rollback-only, so an outer callback cannot commit by swallowing the inner
failure. SQLite commit and rollback hooks detect SQL that prematurely ends the
framework-owned transaction.

Handler failures may return exactly one governed `action.Error` through a
bounded trusted error graph. Framework business codes are restricted by source;
custom codes must appear in the current descriptor, match their declared kind,
and carry a trimmed, valid UTF-8 message of at most 512 runes with no control or
line-separator characters. Denial codes come only from a validated denied
`authz.Decision`; Handler `denied` and `internal` custom errors are rejected.
Operational failures from authorization, plan, idempotency, audit, and
transaction dependencies are always `INTERNAL_ERROR`, regardless of an
`action.Error` in their cause graph.

The normalized code, kind, and message form one channel-independent public
contract. HTTP derives status from kind, MCP returns the same structured fields,
and CLI prints the same safe code and message while retaining bounded
`errors.Is`/`errors.As` identity. Audit records use `denied` for denied kinds,
`rejected` for public business kinds, and `failed` for unavailable or internal
kinds. Internal and malformed envelopes are replaced with stable generic output;
dependency and panic detail never enters a public channel.

## Application And Transports

`appkit.Start` asks the Module Host to assemble one atomic snapshot of required
governance services and optional identity facades, then returns an opaque
`Application`. Its public surface contains immutable
metadata, a read-only Action catalog, the governed Runtime, identity facades,
readiness, and shutdown. It does not expose the Host, mutable registry, concrete
database, service container, or raw Action handlers.

Consumers mount transports explicitly:

- `httpapi.NewHealth` exposes application-owned health metadata;
- `httpapi.NewAPI` exposes session-authenticated Action routes with strict JSON,
  bounded bodies, request deadlines, CSRF protection, and secure cookies by
  default;
- `httpapi.NewMCP` exposes only Actions declaring the MCP channel and reports
  consumer-supplied application identity;
- `httpapi.NewSPA` validates and snapshots a bounded consumer-provided regular-
  file tree at construction, then serves immutable bytes without coupling routes
  to a Module ID;
- `appcmd` owns process signal, serve/drain, token-authenticated Action command,
  and version orchestration while all business execution remains inside the
  Runtime. CLI `--token-file <path>` is supported only on Linux and Darwin. The
  token must remain a regular file owned by the effective UID with exact mode
  `0400` or `0600`; Darwin also queries the retained open file descriptor and
  rejects any extended ACL. Every other operating system rejects a token path
  before any filesystem access. Only `--token-file -` remains available there
  and reads from standard input.

`appcmd.Run` parses and validates help, version, and command syntax without
invoking the Definition provider. Help and version use `Options.Metadata`.
Serve and Action commands invoke the provider exactly once after pure command
preflight, return ordinary provider errors with composition context, contain
provider panics without exposing the panic value, and require the resulting
Definition metadata to exactly match `Options.Metadata`. `appcmd.Serve` and
`appcmd.RunAction` remain lower-level entry points for callers that already own
an assembled Definition. `appcmd.HandlerFactory` receives the active Serve
context and the fully started `appkit.Application`; handler construction must
observe context cancellation cooperatively.

The authenticated `httpapi.NewAPI` and `httpapi.NewMCP` constructors require
`application.Ready()` to be true. Handlers constructed while ready retain leased
Runtime and identity facades, so requests fail closed once shutdown revokes the
Application gate. Lifecycle revocation and lifecycle-canceled authentication are
reported as unavailable rather than as internal server failures.

HTTP sessions, MCP bearer authentication, and command bearer authentication
validate every actor returned by the installed Identity adapter before catalog
discovery or Action execution. Invalid adapter output fails closed as a
dependency failure. All extension and dependency causes cross one opaque public
error-chain boundary. Standard `errors.Is` and `errors.As` perform bounded
matching without calling caller-defined `Error`, `Is`, `As`, or `Unwrap`; an
external consumer needs no internal helper import. `appcmd.Options.Stdout` and
`Stderr` are trusted, cooperative dependencies: `Write` must return because
context cancellation and shutdown timeouts cannot interrupt a blocked writer.

Unconfigured MCP and UI routes do not exist. The framework never discovers or
embeds consumer assets implicitly.

## Project Tooling

The consumer pins a small Go entry point that calls `projecttool.Run` with the
same error-returning Definition provider. The tool validates command syntax and
`modary.yaml` before invoking that provider exactly once. `modary.yaml` declares
application metadata, generated artifact paths, and one build target; it never
declares the Module list.

- `verify` validates metadata, definitions, graph, capabilities, schemas,
  migration declarations, and output policy without writing or starting a
  Module. Migration source contents are opened and validated by the Host's
  internal migration controller during application startup.
- `generate` renders the graph, Action catalog, and optional TypeScript contract
  deterministically, prepares the complete batch, and installs each changed file
  with one sibling rename. That rename is atomic only where the host filesystem
  guarantees rename atomicity. The TypeScript contract includes the complete
  framework error map plus Action-specific declared codes, code unions, and
  code-to-kind lookup types.
- `generate --check` and `check` report drift without modifying files.
- `build` verifies current artifacts and builds exactly one configured consumer
  command before installing the output binary with the same sibling-rename
  boundary. Build does not claim byte-for-byte reproducible binaries.

The tool validates one strict bounded YAML document, rejects aliases, duplicate
semantic identities, non-portable paths, path ancestry conflicts, symlinks, and
project-root replacement. Filesystem validation and artifact mutation use one
verified `os.Root`, honor context cancellation, and clean temporary files on
failure. Same-root operations serialize within one process by the filesystem identity
captured when the Project is loaded, not by pathname spelling.
The generated set is not a filesystem-wide or crash-atomic transaction; a commit failure attempts
in-process rollback, and separate processes must be serialized by consumer
automation. Definition inspection never opens a migration filesystem or invokes
Start or Handler factories.

The Go build subprocess uses the verified absolute project pathname as its
working directory, but its `-o` target is an outside-project operating system
temporary directory rather than the configured project output.
Before invoking Go, Modary canonicalizes `TMPDIR`, rejects it when it is inside
or resolves through a symlink into the project, and retains a file descriptor
and filesystem identity for that directory and every ancestor through `/`.
Every retained directory is revalidated, must be owned by the effective UID or
root, and may be group- or other-writable only when root-owned and sticky.
Darwin also rejects any extended ACL at every retained level. The child staging
directory must be effective-UID-owned with exact mode `0700`; Darwin also
rejects an extended ACL on that child. Build removes every inherited
case-variant `TMPDIR` and `GOTMPDIR` entry from the child environment, then sets both
exactly once to the same canonical staging parent whose descriptor and ancestry
are retained and revalidated. An ambient `GOTMPDIR` cannot override that parent.
Every other platform,
including other Unix variants and Windows, fails Build.
F0 has no validated ACL policy there. Such platforms are cross-compile-only where
covered. F0 claims no native Build, ACL, or rename runtime validation for them.

Build sets `GO111MODULE=on`, `GOTOOLCHAIN=local`, `GOENV=off`, `GOWORK=off`,
and an empty `GOFLAGS`, then invokes `go build` with `-mod=readonly` and
`-buildvcs=false`. Other inherited environment, the selected Go executable and
toolchain, and consumer source remain trusted inputs; Build is not a sandbox. Modary
validates the staged regular non-empty file, copies it through the verified
`os.Root` to a sibling temporary file, and installs it by rename. Root identity
is rechecked after the subprocess returns. The working-directory pathname and
same-UID concurrent replacement are also outside the sandbox boundary. Together
these trusted inputs and deployment conditions are not a sandbox boundary.

On Linux and Darwin, Build starts Go in an independent process group. After
`Start`, `waitid(WEXITED|WNOWAIT)` observes the group leader without reaping it.
While the leader PID and PGID remain reserved, Build kills residual same-group
descendants after either a zero or non-zero leader exit, then calls `Cmd.Wait`.
When the leader exited successfully, pre-reap group cleanup succeeded, and the
context remains active, `exec.ErrWaitDelay` from `Cmd.Wait` is only a residual
inherited-pipe close backstop and does not fail Build. Writer, process-exit,
cancellation, and cleanup errors still fail. Context cancellation also targets
the same group. A trusted descendant that daemonizes or enters
another process group can escape cleanup, so this is not a strong process sandbox.
With cooperative output writers, compiler cancellation and inherited output
pipes have a bounded wait. Caller-supplied `io.Writer.Write` must return; Go
cannot interrupt a blocked call.

## Official Adapters

The official F0 stack consists of:

- `adapters/sqlite`: pure-Go SQLite, migrations, and one framework-private
  persistence bundle containing plan storage, idempotency storage, and
  transaction ownership;
- `adapters/localidentity`: explicitly provisioned password and bearer
  principals with Argon2id, bounded password-check concurrency, sessions,
  rotation, and revocation;
- `adapters/rbac`: explicitly provisioned roles and scope-bound actor bindings,
  default deny, row constraints, and transaction-aware authorization reads;
- `adapters/sqlaudit`: bounded structured audit persistence with corruption
  checks and transaction-aware writes.

The official SQLite stack is the F0 durable adapter boundary. Public
`database.Access`, `database.Executor`, `database.Row`, and `database.Rows`
describe only the consumer-side data capability and values. Backend
construction, SQL policy wrapping, migration, Action persistence, and
transaction control are framework-internal. External packages may implement
`database.Access` for isolated consumer tests, but cannot install a substitute
as the Host's canonical database service. The Host accepts privileged services
only as one bundle owned by one official adapter, so mixed persistence owners
and non-atomic transaction substitutes are not representable. A custom durable
adapter therefore requires a framework contribution rather than an
application-level plug-in.

Each Module migration source is a root `fs.ReadDirFile` containing at most 256
entries. Names are valid single path elements of at most 255 bytes; each SQL file
is at most 1 MiB and one source retains at most 16 MiB. Reads stop one byte beyond
the applicable bound and all files are loaded and validated before any database
effect. The SQLite migration profile accepts forward DDL and DML but rejects
transaction control, savepoints, temporary objects, administrative statements,
trigger rollback expressions, `OR ROLLBACK` conflict actions, and every bare or
quoted `temp.` schema reference. Applied
history is also capped at 256 rows. One Module's complete pending suffix and its
history records commit together.

Empty adapter options create only owned schema. Options are copied and validated
before startup. Adapters do not read environment variables or global
configuration and do not create product policy, secrets, or domain data.
For the file-backed SQLite profile, every directory ancestor is owned by the
effective UID or root; a group- or other-writable ancestor is accepted only when
it is root-owned and sticky. The final database directory remains effective-
UID-owned and non-writable by group or other users.

## Compatibility And Release

All public packages are alpha. Generated JSON and TypeScript outputs are also
alpha consumer artifacts and are guarded by deterministic drift tests rather
than a long-term compatibility promise. Consumers pin an exact Modary version
and upgrade deliberately.

The F0 acceptance fixture proves a separate module by copying it outside the
framework checkout and using an absolute local `replace`. Remote installation
becomes a release claim only after this module path is published and tagged. The
current source tree has no owner-selected redistribution license, so this local
technical acceptance is not a public redistribution grant.
