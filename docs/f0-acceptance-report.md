# Modary Framework F0 Acceptance Report

- Acceptance object: independently consumable Modary Go framework
- Governing contract: `.ai-platform/specs/005-postgres-task-runtime/spec.md`
- Date: 2026-08-02
- Status: Accepted
- Accepted source: current reviewed working source; release commit not created
- Consumption status: Local source checkout
- Distribution status: Not_released
- Version tag: None
- License: Apache-2.0

## Verdict

The PostgreSQL-first Modary F0 is technically accepted for coordinated local
source consumption. PostgreSQL is the only official durable profile; the public
task API is domain-neutral; River and raw transaction authority remain private;
and the independently copied-out Counter consumer uses only public contracts.

This verdict does not claim a new remote version tag or published module. The
latest historical release does not contain this profile.

## Acceptance Matrix

| Area | Current result | Evidence gate |
|---|---|---|
| Public Kernel and lifecycle | Accepted | Public API, lifecycle, capability, cleanup, race, and unavailable-Host tests |
| Governed Action runtime | Accepted | Transaction, rollback-only nesting, idempotency, audit, bounded JSON, and schema tests |
| PostgreSQL durable profile | Accepted | Real PostgreSQL startup, owned schemas, migrations, restart, corruption, and transaction tests |
| Durable task runtime | Accepted | Exact-transaction enqueue, retry, recovery, multi-runner, lifecycle, and secret-safety tests |
| Standard adapters | Accepted | PostgreSQL-native Identity, RBAC, SQL Audit, migration, plan, and idempotency tests |
| Independent consumer | Accepted | Copied-out, `GOWORK=off`, Node-free PostgreSQL Counter conformance, transactional enqueue, restart, and worker consumption |
| Architecture and neutrality | Accepted | Public-boundary, import-direction, generated-drift, and active-tree neutrality gates |
| Repository gates | Accepted | `make acceptance`, `make ci`, race, repetition, fuzz, build, and source-diff checks |
| Engineering review | Accepted | Two current post-remediation review passes with no unresolved P0, P1, or P2 finding |

## Evidence

- PostgreSQL and task contracts: `.ai-platform/evidence/T024/`
- Standard durable persistence: `.ai-platform/evidence/T025/`
- Consumer, deletion, and final acceptance: `.ai-platform/evidence/T026/`
- Exact operational boundaries: `docs/f0-known-limitations.md`

## Release Boundary

Publishing `v0.1.0-alpha.3` requires a clean committed candidate, successful tag
CI against PostgreSQL 17, normal Go module resolution, and copied-out remote
consumer verification. Consumers use the current source checkout until those
external gates pass.
