# Modary Framework F0 Acceptance Report

- Acceptance object: independently consumable Modary Go framework
- Governing contract: `.ai-platform/specs/002-framework-decoupling/spec.md`
- Date: 2026-07-31
- Status: Accepted
- Distribution status: Not_released
- Version tag: None
- Owner-selected redistribution license: None

## Verdict

Modary Framework F0 is technically accepted for local source consumption.
T010 through T016 satisfy their governed contracts. The frozen review source is
captured in `.ai-platform/evidence/T016/diff.patch`; the matching test and review
records are in the same evidence directory.

This verdict does not claim a remote version tag, a published module release, or
a consumer product release.

## Acceptance Matrix

| Area | Current result | Evidence gate |
|---|---|---|
| Public Kernel and lifecycle | Accepted | Public API, lifecycle, capability, cleanup, race, and unavailable-Host tests |
| Governed Action runtime and schemas | Accepted | Transaction, idempotency, audit, bounded JSON, SchemaGraph, and official Draft 7 corpus tests |
| AppKit, commands, and transports | Accepted | Assembly-gate, CLI, HTTP, MCP, protocol, shutdown, and error-boundary tests |
| Pure project tooling | Accepted | Determinism, cancellation, verified Root, outside-project build, cross-build, and fuzz tests |
| Neutral official Adapters | Accepted | Empty provisioning, migration, transaction, restart, ownership, and security tests |
| Independent consumer | Accepted | Copied-out, `GOWORK=off`, Node-free, multichannel conformance with a consumer-owned capability |
| Architecture and neutrality | Accepted | AST package/import gates, public API documentation, generated drift, and active-tree neutrality |
| Repository gates | Accepted | `make acceptance`, `make ci`, strict artifact validators, and source-diff checks |
| Independent engineering review | Accepted | Two fresh reviewers inspected one frozen digest and reported zero P0, P1, and P2 findings |

## Evidence

- Frozen source and acceptance summary:
  `.ai-platform/evidence/T016/summary.md`
- Complete post-freeze command record:
  `.ai-platform/evidence/T016/test-results.md`
- Independent reviews:
  `.ai-platform/evidence/T016/review-1.md` and
  `.ai-platform/evidence/T016/review-2.md`

## Residual Risk

The accepted limitations are recorded in `docs/f0-known-limitations.md`.
Public APIs and generated formats remain alpha; the durable profile remains
single-process SQLite; trusted callbacks and writers must cooperate with
cancellation; and native Build security is supported only on validated
platforms.

## Release Boundary

F0 publishes a framework contract and local source-consumption proof. Consumers
own their domain, composition, policy, provisioning, UI, executable, deployment,
and release artifacts. The public APIs and generated formats remain alpha until
a versioned compatibility policy is published.
