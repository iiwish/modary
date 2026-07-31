# T015 Evidence Summary

- Status: Completed
- Date: 2026-07-30
- Packet: `.ai-platform/specs/002-framework-decoupling/packets/T015.yaml`

## Changed And Removed Files

- Independent consumer: `testdata/external-consumer/**`, including its own Go
  module, composition, Counter feature, migration, command, project tool,
  generated contracts, embedded UI, and conformance tests.
- Repository gates: `Makefile`, `.github/workflows/ci.yml`,
  `scripts/check-neutrality.sh`, `scripts/check-docs.sh`, and `.gitignore`.
- Canonical framework documentation: `README.md`, `docs/framework-f0.md`,
  `docs/f0-known-limitations.md`, and ADR-001 through ADR-004.
- Application-owned paths removed from the active tree: `cmd/**`, `core/**`,
  `modules/**`, `web/**`, `tests/**`, `internal/{app,generated,transport,webui}`,
  root application manifests, Node workspaces, container files, and former
  application release scripts.
- Preservation evidence: `.ai-platform/evidence/T010/*-archive.inventory`,
  `.ai-platform/evidence/T010/check-archives.sh`, and the external source and
  governance archives recorded below.

## Red Result

The initial copied-out and boundary checks exposed framework coupling to an
integrated application, private/internal composition, a Node-owned UI and
release pipeline, implicit application provisioning, and project tooling that
could not be exercised by an independent Go module. Archive review later
exposed three evidence gaps: manifests lacked a versioned digest anchor, exact
membership and file modes were not verified, and excluded historical files and
`.DS_Store` metadata were not explained.

## Green Result

- `example.com/modary-counter-consumer` composes the framework through public
  packages only and owns its domain, policy, provisioning, migration, UI,
  executable, generated outputs, and release boundary.
- One governed durable Counter write is proven through Runtime, CLI, HTTP, and
  MCP; tests cover Preview/Execute, restart, scope isolation, stale plans,
  idempotent replay, SQL Audit, explicit provisioning, and empty defaults.
- CLI `--token-file <path>` is Linux/Darwin-only and requires a regular,
  effective-UID-owned file with exact mode `0400` or `0600`; Darwin queries the
  retained open file descriptor and rejects extended ACLs. Every other operating
  system rejects a token path before any filesystem access and supports only
  `--token-file -` through standard input.
- Extension and dependency causes cross one opaque public error-chain boundary.
  The copied consumer uses standard `errors.Is` and `errors.As` for bounded
  matching without invoking caller-defined `Error`, `Is`, `As`, or `Unwrap` and
  without importing an internal helper. `appcmd.Options.Stdout` and `Stderr` are
  trusted cooperative dependencies whose `Write` calls must return;
  context cancellation and shutdown timeouts cannot interrupt a blocked writer.
- The file-backed SQLite profile requires every directory ancestor to be owned
  by the effective UID or root. A group- or other-writable ancestor is accepted
  only when root-owned and sticky; the final database directory remains
  effective-UID-owned and non-writable by group or other users.
- The copied-out gate creates a module outside the framework checkout, rewrites
  only the development `replace`, sets `GOWORK=off`, puts failing `node`, `npm`,
  and `pnpm` shims first in `PATH`, and passes tidy, test, vet, project verify,
  generated check, build, and binary execution.
- The active repository contains only neutral framework production packages,
  framework documentation, and the clearly isolated conformance fixture.
- The Node-free Go framework Make and CI gates cover root and consumer tests,
  vet, race, repetition, fuzz smoke, generation drift, neutrality, native builds,
  Linux/Windows cross-builds, and a post-gate clean-source assertion.
- Project artifact mutation uses a verified `os.Root`; compiler output is staged
  outside the project. Build canonicalizes `TMPDIR`, rejects it when it is inside
  or resolves through a symlink into the project, and retains a file descriptor
  and filesystem identity for that directory and every ancestor through `/`.
  Every retained directory is revalidated, must be owned by the effective UID or
  root, and may be group- or other-writable only when root-owned and sticky.
  Darwin rejects any extended ACL at every retained level. The child staging
  directory must be effective-UID-owned with exact mode `0700`; Darwin also
  rejects an extended ACL on that child. Build removes every inherited
  case-variant `TMPDIR` and `GOTMPDIR` entry from the child environment, then sets
  both exactly once to the same canonical staging parent whose descriptor and
  ancestry are retained and revalidated.
  An ambient `GOTMPDIR` cannot override that parent. A fake Go regression
  observes this parent in both child variables when ambient `TMPDIR` is a
  symlink alias and ambient `GOTMPDIR` points into the project.
  Every other platform, including other Unix variants and Windows,
  fails Build because F0 has no validated ACL policy there.
  Unsupported platforms are cross-compile-only where covered.
  F0 claims no native Build, ACL, or rename runtime validation for them.
  A sibling rename is atomic only where the host filesystem guarantees rename atomicity.
  The Go subprocess `command.Dir`
  pathname and same-UID replacement are not a sandbox boundary.
- Build sets `GO111MODULE=on`, `GOTOOLCHAIN=local`, `GOENV=off`, `GOWORK=off`,
  and an empty `GOFLAGS`, and passes `-mod=readonly` and `-buildvcs=false`.
  Other inherited environment, the selected Go executable and toolchain, and
  consumer source remain trusted inputs; Build is not a sandbox.
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
- Same-root operations serialize within one process by the filesystem identity
  captured when the Project is loaded, not by pathname spelling. With
  cooperative output writers, compiler cancellation and inherited output pipes
  have a bounded wait. Caller-supplied `io.Writer.Write` must return; Go cannot
  interrupt a blocked call.

## Archive Preservation

The integrated prototype is preserved outside the active framework tree:

```text
/Users/iiwish/self/rulary/prototype/modary-integrated-f0/             167 files
/Users/iiwish/self/rulary/prototype/modary-integrated-f0-governance/   51 files
```

Both external manifests pass `shasum -a 256 -c`. Versioned inventories record
the exact relative path, SHA-256 digest, and file mode of all 218 contracted
files and detect additions, removals, content changes, and mode changes. The
four versioned digest anchors are:

```text
source manifest       143d934c29693fe799f230fd04c1647b1f9ff41ac8b292fd7ca965f7e83d43fa
governance manifest   68593d9ad930e4e5c12fef8d6fb0abe55ac11e10a423563d43c96233fd586745
source inventory      119c2280023a59bcb2ac548ed9fba4ffcad0860a13991bea6bca228a1a6e9c38
governance inventory  fa15a300998571ba703f04e49294339b86212af2caf03ff183153293ad0086b0
```

`.DS_Store` is declared platform metadata and excluded from the source
contract. The snapshot represents the T010 working tree. `docs/f1-proposal.md`
and two superseded hashed Web bundles were already absent at capture time and
remain recoverable from baseline commit
`970a2d4b32193562bbbd60dbd53bd1d0857523b7`; the replacement bundles and
executable UI index are present in the source archive.

## Refactor Result

One consumer composition definition feeds the command and project tooling.
Generated contracts are deterministic and checked rather than regenerated by
acceptance. Neutrality now scans the constitution, canonical docs, Make/CI,
scripts, production packages, and consumer imports; it rejects former domain
terms, legacy imports and trees, production executables, persisted databases,
extension-bearing binaries, and unexpected executable files anywhere outside
`.git`.

## Validation

The final implementation passed:

```text
make acceptance                                                  pass
copied-out external consumer gate                                pass
source archive checksum and exact set/mode/hash (167 files)       pass
governance archive checksum and exact set/mode/hash (51 files)    pass
all four evidence digest anchors                                 pass
maintainer-local sh .ai-platform/evidence/T010/check-archives.sh pass
scripts/check-neutrality.sh                                      pass
scripts/check-docs.sh                                            pass
git diff --check                                                 pass
```

## Boundary Review

The first independent review confirmed that the implementation and copied-out
consumer satisfy the local independent-consumption boundary, then identified
archive anchoring, exact-set, evidence-scope, status-consistency, and neutrality
regression gaps. Those findings were repaired and adversarially retested. The
final independent review reran acceptance, archive, copied-out, neutrality,
validator, patch-reversal, and diff gates and reported P0 = 0, P1 = 0, P2 = 0.

## Residual Risk

- The local `replace` proves source-checkout consumption, not installation from
  a published remote tag.
- The external archive directory is writable and is not itself versioned or
  immutable. Versioned inventories detect drift but do not replace a Git-backed
  or immutable backup.
- A public release still requires an explicit license and compatibility policy.
- Unsupported-platform cross-builds prove compile portability only where
  covered. F0 does not claim native Build, ACL, or rename runtime validation on
  those platforms, including adversarial path execution.
