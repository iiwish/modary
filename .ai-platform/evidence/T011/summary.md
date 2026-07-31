# T011 Evidence Summary

- Status: Completed
- Date: 2026-07-30
- Packet: `.ai-platform/specs/002-framework-decoupling/packets/T011.yaml`

## Changed Files

- Public Kernel packages: `action`, `audit`, `authz`, `database`, `identity`,
  `module`, and `scope`.
- Host-only construction capability: `internal/authority`.
- Canonical module path and dependency metadata: `go.mod`, `go.sum`.
- T011 tests and delivery evidence.

## RED Result

The focused tests express the missing contracts before their implementations:
pure Definition inspection, strict descriptors and schemas, typed service-key
identity, capability-scoped installation, lifecycle rollback and drain,
integrity-checked contract-bound plans and idempotency, replay authorization, owned
values, migration history integrity, and opaque execution scope. Each group was
implemented only after its failure mode was represented by a regression test.

## GREEN Result

- The Host defensively snapshots Manifest, Action, migration, and factory
  declarations. Definitions can be validated without invoking Start or factories.
- The Host owns Action binding and Runtime construction. Registry and Runtime
  mutation require an internal construction token; catalogs are defensive,
  read-only projections without Handler values.
- Service keys are typed and namespaced. Installation scope is sealed and grants
  only Manifest-declared capabilities.
- Startup validates before side effects. Partial starts and panics roll back in
  reverse dependency and per-Module LIFO order; cleanup errors are joined;
  shutdown revokes and drains executions before releasing resources.
- Action descriptors use strict SemVer, bounded identifiers, Draft-7 local-only
  schemas, canonical contract hashes, compiled validators, and immutable data.
- Plans bind contract version/hash, actor identity/type, channel, scope, input,
  payload, impact, authorization fingerprint, and expiry.
- Idempotency records bind scope, actor ID/type, channel, Action contract,
  canonical input, original PlanHash, impact, and execution grant fingerprint.
  Replays rerun intent and impact authorization, enforce current constraints,
  reject identity/channel/plan/contract/grant drift, and audit the original plan
  and stored impact without executing the Handler again.
- Migration validation enforces ordered immutable history and atomic application.

## Public API Review

- Ordinary external consumers use only top-level packages. The Go `internal`
  boundary keeps construction authority out of supported consumer imports.
- `CatalogEntry` exposes Descriptor, Module owner, and ContractHash only.
- `PreparedDescriptor` owns schema/channel bytes and reuses compiled validators.
- Identity represents authentication identity; Authorization owns policy and
  constraints; isolation uses `scope.Execution{Kind, ID}`.
- Runtime dependencies are explicit and typed-nil checked. No production
  governance dependency silently falls back to an in-memory implementation.

## Lifecycle And Integrity Review

- Host state transitions, concurrent Start/Shutdown, Runtime construction races,
  held/in-flight execution drain, nil contexts, and callback panics are covered
  by race-enabled repeated tests.
- Memory stores and Runtime boundaries deep-copy JSON, slices, references, plans,
  impacts, and idempotent results.
- JSON parsing rejects duplicate object members and trailing values. Numeric
  canonicalization preserves integers beyond 2^53 while normalizing equivalent
  number spellings.
- Audit events carry ActionVersion and ContractHash and apply bounded persistence
  normalization to every envelope field.

## Residual Risk

- Go `internal` is an architectural package boundary, not an adversarial security
  primitive against a malicious module that deliberately impersonates the
  canonical module path. AppKit is the supported composition boundary; T012
  proves that normal consumers need no construction authority.
- The integrated application, transports, CLI, and concrete adapters still target
  the pre-decoupling API. T012 through T015 replace those surfaces; T014 supplies
  durable persistence for the new plan, idempotency, scope, and audit provenance
  contracts.
