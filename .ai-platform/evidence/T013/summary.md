# T013 Evidence Summary

- Status: Completed
- Date: 2026-07-30
- Packet: `.ai-platform/specs/002-framework-decoupling/packets/T013.yaml`

## Changed Files

- Pure project configuration and verified root: `projecttool/config.go`.
- Side-effect-free inspection and deterministic identity checks:
  `projecttool/inspect.go`.
- Deterministic complete-batch generation, host-filesystem-qualified sibling-
  rename installation, rollback, and drift checks: `projecttool/generate.go`.
- Node-free build with outside-project compiler staging and a verified `os.Root`
  artifact-mutation boundary: `projecttool/build.go`.
- Consumer-owned command dispatch and errors: `projecttool/run.go`,
  `projecttool/errors.go`.
- TypeScript schema projection: `action/typescript.go`.
- Focused config, context, inspection, generation, build, command, public API,
  fixture, numeric, and string-escape tests.

## RED Result

Tests reproduced Definition side effects, partial generation failures, symlink
and root replacement races, context cancellation after validation or while
waiting for a project lock, non-portable and ancestor-aliasing paths, build
output replacement races, input-order-dependent duplicate diagnostics,
JavaScript string escape changes, inexact numeric literals, and nondeterministic
nested numeric errors. Additional boundary tests cover pathname aliases for one
loaded filesystem identity, Linux/Darwin staging policy, inherited Darwin ACLs,
unsupported-platform Build rejection, project-owned or project-linked `TMPDIR`,
a symlinked ambient `TMPDIR` alias, a malicious project-path `GOTMPDIR`, ambient
Go controls, retained parent identities and insecure writable ancestors,
zero/non-zero compiler-leader exits, same-group descendants, inherited pipes,
cooperative versus blocked caller output writers, and hostile writer error
methods.

## GREEN Result

- `LoadContext`, inspection, verification, generation, checking, and build
  honor cancellation before mutation and while waiting for serialization.
- One verified `os.Root` handle spans filesystem validation and artifact
  mutation. Existing components are non-symbolic, configured paths use a
  portable ASCII grammar, output/package ancestry conflicts fail, and pathname
  replacement is checked before and after external build work and commit. The Go
  subprocess still uses the verified project pathname as `command.Dir`; that
  pathname and same-UID replacement are not a sandbox boundary.
- Same-root operations serialize within one process by the filesystem identity
  captured when the Project is loaded, not by pathname spelling. Aliased paths
  therefore join one gate; separate processes remain externally coordinated.
- Module, Action, owner, and migration duplicate identities are pre-scanned in
  sorted order before internal validation, producing stable errors across
  shuffled input.
- Generation renders a complete deterministic artifact set before mutation,
  installs each changed file with one sibling rename, and attempts in-process
  rollback after a commit failure. A sibling rename is atomic only where the host filesystem guarantees rename atomicity.
  Check mode never writes and reports
  sorted drift. The generated set is not a filesystem-wide, crash, or cross-
  process atomic transaction.
- Build clears inherited Go flags/config, invokes only the Go tool for exactly
  one package, gives the compiler only an outside-project operating system
  temporary output, verifies the regular non-empty output and root identity,
  copies bytes through the verified Root, and installs with one sibling rename.
  Build canonicalizes `TMPDIR`, rejects it when it is inside or resolves through
  a symlink into the project, and retains a file descriptor and filesystem
  identity for that directory and every ancestor through `/`. Every retained
  directory is revalidated, must be owned by the effective UID or root, and may
  be group- or other-writable only when root-owned and sticky. Darwin rejects any
  extended ACL at every retained level. The child staging directory must be
  effective-UID-owned with exact mode `0700`; Darwin also rejects an extended
  ACL on that child. Build removes every inherited case-variant `TMPDIR` and
  `GOTMPDIR` entry from the child environment, then sets both exactly once to the
  same canonical staging parent whose descriptor and ancestry are retained and
  revalidated. An ambient `GOTMPDIR` cannot override that parent. The fake Go
  regression observes this parent in both child variables when ambient `TMPDIR`
  is a symlink alias and ambient `GOTMPDIR` points into the project.
  Every other platform, including other Unix variants and Windows,
  fails Build because F0 has no validated ACL policy there. Unsupported
  platforms are cross-compile-only where covered; F0 claims no native Build,
  ACL, or rename runtime validation for them.
- Build sets `GO111MODULE=on`, `GOTOOLCHAIN=local`, `GOENV=off`, `GOWORK=off`,
  and an empty `GOFLAGS`, and passes `-mod=readonly` and `-buildvcs=false`.
  Other inherited environment, the selected Go executable and toolchain,
  consumer source, `command.Dir`, and same-UID replacement remain trusted inputs
  or deployment boundaries; Build is not a sandbox.
- On Linux and Darwin, Build starts Go in an independent process group. After
  `Start`, `waitid(WEXITED|WNOWAIT)` observes the leader without reaping it.
  While the leader PID and PGID remain reserved, Build kills residual same-group
  descendants after either a zero or non-zero exit, then calls `Cmd.Wait`.
  When the leader exited successfully, pre-reap group cleanup succeeded, and the
  context remains active, `exec.ErrWaitDelay` from `Cmd.Wait` is only a residual
  inherited-pipe close backstop and does not fail Build. Writer, process-exit,
  cancellation, and cleanup errors still fail. Context cancellation also
  targets the same group. A trusted descendant that daemonizes or enters
  another process group can escape cleanup, so this is not a strong process sandbox.
- With cooperative output writers, compiler cancellation and inherited output
  pipes have a bounded wait. Caller-supplied `io.Writer.Write` must return; Go
  cannot interrupt a blocked call.
- Writer and other dependency causes cross one opaque public error-chain
  boundary. Standard `errors.Is` and `errors.As` perform bounded matching without
  calling caller-defined `Error`, `Is`, `As`, or `Unwrap`; consumers need no
  internal helper import.
- TypeScript strings use JSON-compatible ECMAScript escapes. Exact numeric
  literals must remain inside the JavaScript safe range and survive the shortest
  float64 textual round trip without changing their rational value. Nested
  validation uses sorted keys and escaped JSON-pointer locations.

## Independent Review

The independent reviewer closed the original three P1 and three P2 findings,
then found and verified fixes for three additional TypeScript P2 findings. The
final review reports P0/P1/P2 = 0 and confirms that the tooling satisfies the
pure, deterministic, cancelable inspection/generation/check contract and the
verified-Root artifact-mutation boundary without treating the Go subprocess as
a sandbox.

## Residual Risk

- Generated TypeScript is an alpha, intentionally bounded JSON Schema mapping,
  not a complete general-purpose schema compiler.
- Unsupported-platform path and build portability is cross-compiled only where
  covered. F0 does not claim native Build, ACL, or rename runtime validation on
  those platforms, including adversarial root-replacement execution.
- Consumers execute their own trusted Definition provider and Go compiler. The
  remaining environment and consumer source are also trusted; the project tool
  is not a sandbox for untrusted inputs or escaped descendants.
