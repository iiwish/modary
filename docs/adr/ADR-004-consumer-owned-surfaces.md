# ADR-004: Consumer-Owned Tools, Transports, UI, And Releases

- Status: Accepted
- Date: 2026-07-31
- Scope: Project tooling and application integration boundary

## Context

A reusable Go framework cannot import arbitrary consumer packages into a global
binary, infer a correct application from source scanning, or own product routes,
assets, credentials, and release artifacts without coupling itself to one
consumer.

## Decision

Each consumer owns two small entry points that import the same explicit
error-returning Definition provider: an application command using `appcmd`, and
a pinned tool command using `projecttool`. A pure application metadata value is
reused by the provider and `appcmd.Options`; runtime commands reject any mismatch.
The project manifest carries application metadata, artifact paths, and one build
target; Go code remains the only Module list.

Project operations validate a strict bounded manifest and one verified project
root. Verification and generation inspect pure Definitions only. Generation
renders the complete configured output set before mutation and installs each
changed file with one sibling rename. A rename is atomic only where the host
filesystem guarantees rename atomicity. The tool does not claim filesystem-wide, crash, or cross-process atomicity;
same-root operations serialize within one process by the filesystem identity
captured when the Project is loaded, not by pathname spelling. A commit failure attempts in-process
rollback. Check mode never writes. Build executes the trusted Go tool with a constrained environment and
the verified project pathname as its working directory. Compiler output lands
only in an outside-project operating system temporary directory.
Modary canonicalizes `TMPDIR`, rejects it when it is inside or resolves through
a symlink into the project, and retains a file descriptor and filesystem
identity for that directory and every ancestor through `/`. Every retained
directory is revalidated, must be owned by the effective UID or root, and may be
group- or other-writable only when root-owned and sticky. Darwin also rejects
any extended ACL at every retained level. The child staging directory must be
effective-UID-owned with exact mode `0700`; Darwin also rejects an extended ACL
on that child. Build removes every inherited case-variant `TMPDIR` and
`GOTMPDIR` entry from the child environment, then sets both exactly once to the
same canonical staging parent whose descriptor and ancestry are retained and
revalidated. An ambient `GOTMPDIR` cannot override that parent.
Every other platform, including other Unix variants and Windows,
fails Build because F0 has no validated ACL policy there. Such platforms are
cross-compile-only where covered.
F0 claims no native Build, ACL, or rename runtime validation for them.

Build sets `GO111MODULE=on`, `GOTOOLCHAIN=local`, `GOENV=off`, `GOWORK=off`,
and an empty `GOFLAGS`, then invokes `go build` with `-mod=readonly` and
`-buildvcs=false`. Other inherited environment, the selected Go executable and
toolchain, and consumer source remain trusted inputs; Build is not a sandbox. Modary
validates the staged file, copies it through the verified Root, rechecks root
identity, and installs the configured binary with one sibling rename. It does
not claim reproducible binary bytes.

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

Filesystem validation and artifact mutation use a verified `os.Root`, but the
Go subprocess still uses a pathname for `command.Dir`. Same-UID concurrent
pathname replacement and the subprocess are not a sandbox boundary; deployment
and consumer automation must provide the stronger isolation required for
hostile local actors or build inputs.

HTTP, MCP, health, and static asset handlers are created and mounted explicitly
by consumer code. Runtime metadata comes from the consumer Definition; pure help
and version commands use the same value through `appcmd.Options` without
constructing that Definition. UI assets are copied from a bounded consumer
`fs.FS` into an immutable construction-time snapshot; the framework contains no
application route registry or frontend build dependency. `appcmd` authenticates
Action callers through a bearer token.
CLI `--token-file <path>` is supported only on Linux and Darwin and must remain
a regular file owned by the effective UID with exact mode `0400` or `0600`.
Darwin also queries the retained open file descriptor and rejects any extended
ACL. Every other operating system rejects a token path before any filesystem access;
only `--token-file -` reading standard input remains available there.
All extension and dependency causes cross one opaque public error-chain
boundary. Standard `errors.Is` and `errors.As` perform bounded matching without
calling caller-defined `Error`, `Is`, `As`, or `Unwrap`; an external consumer
needs no internal helper import. `appcmd.Options.Stdout` and `Stderr` are trusted,
cooperative dependencies: `Write` must return because context cancellation and
shutdown timeouts cannot interrupt a blocked writer. `appcmd` orchestrates
process lifecycle but does not choose routes, configuration sources, policy, or
provisioning. Top-level application and project commands validate pure command
inputs before invoking the shared Definition provider. Ordinary construction
errors retain their consumer diagnostic and error identity; recovered panics
cross a stable boundary that never formats the panic value.

The framework repository contains libraries and conformance fixtures, not a
consumer executable, container, default account, or deployment image. Technical
F0 acceptance is local source proof; this tree has no published version tag and
no owner-selected redistribution license, so it does not claim a public
distribution.

## Consequences

- A consumer can add Modules, Actions, migrations, assets, and release behavior
  without editing framework source.
- Headless framework and consumer gates need only the Go toolchain.
- The consumer tool must be built from trusted pinned source; it intentionally
  executes the consumer's Definition provider and Go compiler.
- Consumer CI must serialize generator or build processes that target the same
  project root.
- UI frameworks, deployment systems, and configuration loaders remain consumer
  choices.

## Rejected Alternatives

- A global CLI cannot safely import an arbitrary local Go composition root.
- Source-tree scanning creates a second composition model and makes behavior
  depend on repository layout.
- Automatic route mounting based on Module IDs couples transports to concrete
  products.
- Embedding one frontend or release binary turns framework distribution into an
  application release.
