# T016 Evidence Summary

- Status: Completed
- Date: 2026-07-31
- Packet: `.ai-platform/specs/002-framework-decoupling/packets/T016.yaml`
- Frozen tree: sha256:f3e127a3c957a9dab2dba06342f08b7f7c9ae24bc2dfcbe4729aa7924ea052f4
- Frozen at: 2026-07-31T07:41:02Z

## Changed Files

- Public framework contracts: `action/**`, `appcmd/**`, `appkit/**`,
  `audit/**`, `authz/**`, `database/**`, `identity/**`, `module/**`,
  `projecttool/**`, `scope/**`, and `transport/**`.
- Official neutral adapters and their durable schema: `adapters/**`.
- Private implementation boundaries: `internal/**`, including lifecycle,
  persistence, SQL policy, safe error inspection, filesystem policy, strict
  JSON values, and the offline Draft 7 SchemaGraph engine.
- Independent public consumer conformance project:
  `testdata/external-consumer/**`.
- Node-free quality and release gates: `Makefile`, `.github/**`, `scripts/**`,
  `.gitattributes`, and Go module metadata.
- Canonical framework contracts and current-state documentation:
  `.ai-platform/**`, `README.md`, and `docs/**`.
- The former integrated application, consumer domain, embedded UI, Node
  workspace, application executable, and application release surface are
  removed from the active framework tree.

## Red Result

Independent review and full-gate pressure exposed four release-significant
gaps: public SQL did not reject quoted and unquoted `temp.` references before
backend resolution; typed-nil Definition-provider errors dispatched `Error`
and lost their safe identity; synchronization tests used load-sensitive
wall-clock assumptions; and one project-build cancellation test treated PID
file creation as completed PID publication.

## Green Result

- One shared token-level SQL policy rejects every supported quoted and
  unquoted `temp.` qualifier for reads and mutations before either resolver or
  executor invocation, and remains shared with migration validation.
- Definition-provider typed-nil errors receive a stable opaque diagnostic,
  preserve standard `errors.Is` and `errors.As` identity through the bounded
  opaque boundary, and invoke none of the caller's `Error`, `Is`, `As`, or
  `Unwrap` methods.
- Password-rotation concurrency uses a deterministic verifier barrier, while
  asynchronous test timeouts serve only as deadlock guards rather than
  performance assertions.
- Project build cancellation waits for a parsed positive child PID and drains
  cleanup on synchronization failure. Focused count-50, race count-20, full
  count-20, and Windows amd64/arm64 test compilation pass.
- The independent consumer composes a custom typed capability, governed
  Counter Action, official adapters, CLI, HTTP, MCP, generated contracts,
  embedded UI, restart behavior, and project tooling using public Modary
  packages only.

## Refactor Result

The final framework has explicit composition and ownership: pure Definitions,
typed capabilities, Host-owned lifecycle and migrations, a governed Action
Runtime, consumer-owned application surfaces, neutral opt-in adapters, pure
project tooling, local-only bounded SchemaGraph validation, and a copied-out
consumer gate. Shared semantics live behind focused internal packages rather
than command or adapter aliases.

## Acceptance Matrix

| Review pass | Frozen-tree evidence |
| --- | --- |
| Spec compliance | Confirmed SSOT, requirements checklist, and two independent reviews |
| Architecture | Neutral public packages, explicit internal ownership, import matrix |
| Public API | External consumer uses canonical public imports and a custom capability |
| Security | Fail-closed auth, SQL, filesystem, process, opaque-error, and schema boundaries |
| Concurrency | Race suite, count-20 suite, lifecycle drain, transaction, and cancellation tests |
| Filesystem | Verified-root mutation, SQLite ancestry, token-file, TMPDIR, and ACL policy |
| Protocol | Strict HTTP, MCP, JSON-RPC, CLI, CSRF, and error projections |
| Consumer conformance | In-tree and copied-out consumer test, vet, generate, check, build, and execution |
| Build and release | Native build, six target cross-build matrix, source stability, no Node dependency |
| Engineering quality | Normal, panicnil, vet, race, repetition, fuzz, neutrality, and generated drift gates |

## Review

Two completely fresh independent reviewers started after the freeze, verified
the same live and stored SHA-256 digest before and after review, covered the
complete F0 contract, and reported `P0 = 0`, `P1 = 0`, and `P2 = 0`.

## Full Validation

The post-freeze component gate passed normal, panicnil, vet, native and
cross-build, race, count-20, fuzz, copied-out consumer, neutrality, generated
drift, and complete source-diff checks. After evidence assembly, exact
`make acceptance` and `make ci` both passed. CI's complete source snapshot was
unchanged across every gate.

## Residual Risk

- Source-checkout consumption through a local `replace` does not establish a
  published remote module version or compatibility promise.
- Native adversarial filesystem, ACL, rename, and Build behavior is claimed
  only on Linux and Darwin; other covered targets are compile checks.
- Cooperative callbacks and writers must return. Same-UID replacement,
  selected toolchains, consumer source, mount behavior, and regrouped or
  daemonized descendants remain documented deployment or trust boundaries.
- Distribution remains `Not_released`; a public release requires an
  owner-selected license, compatibility policy, and version tag.
