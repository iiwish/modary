# Modary Delivery Constitution

- Version: 2.0
- Status: Confirmed
- Last updated: 2026-07-31
- Approval source: user-approved framework decoupling analysis

## Product Boundary

Modary is an independent Go-first governed application framework. Consumer
applications depend on public Modary packages. Modary production source,
configuration, generation, CI, release assets, and active product contracts do
not depend on any consumer domain.

## Principles

- A business Action has one Handler and may be projected through HTTP, CLI,
  tasks, and Agent channels only through the governed Runtime.
- Module composition is explicit Go code. Definitions are pure and inspectable;
  installation owns runtime side effects.
- Authorization, Preview, plan binding, idempotency, transaction, and audit
  semantics fail closed and remain consistent across channels.
- Public APIs expose sealed capabilities and read-only catalogs, not raw
  handlers, registries, database handles, transaction control, or lifecycle
  internals.
- Module lifecycle is deterministic, reversible for process resources, and
  observable. Forward-only migrations are atomic and retryable per module.
- Official adapters are neutral and configured explicitly by consumers. They do
  not create users, roles, permissions, grants, domain records, or secrets by
  default.
- The default F0 runtime is a single durable SQLite store. Distributed
  transaction or arbitrary storage transparency is not claimed.
- Headless consumers build and run with Go alone. UI assets and branding belong
  to the consumer; optional Modary UI packages remain domain-neutral.
- Generated artifacts are deterministic, consumer-owned, and produced without
  starting modules or touching persistent state. Each changed file is installed
  by sibling rename, which is atomic only where the host filesystem guarantees
  rename atomicity; a multi-file set is not claimed to be a filesystem-wide or
  crash-atomic transaction.

## Quality Gates

Behavior changes use focused RED/GREEN/REFACTOR tests. Framework acceptance
requires public API tests, lifecycle and race tests, independent-module
conformance, deterministic generation, transport contract tests, neutrality
checks, vet, race, and build evidence.

## Git And Review Policy

Preserve user work, retain a verified copy of consumer-owned prototype assets
before removing them from this repository, avoid destructive commands, and
complete spec-compliance and engineering-quality reviews before acceptance.

## Change Process

Constitution changes require explicit user approval. The canonical product
contract is `.ai-platform/specs/002-framework-decoupling/spec.md`.
