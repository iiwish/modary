# Modary Delivery Constitution

- Version: 3.0
- Status: Confirmed
- Last updated: 2026-08-01
- Approval source: owner-approved PostgreSQL-first rewrite without compatibility constraints

## Product Boundary

Modary is an independent Go-first governed application framework. Consumer
applications depend on public Modary packages. Modary production source,
configuration, generation, CI, release assets, and active product contracts do
not depend on a consumer domain.

## Principles

- A business Action has one Handler and reaches HTTP, CLI, Agent, task, or
  consumer-owned surfaces only through the governed Runtime.
- Module composition is explicit Go code. Definitions are pure and inspectable;
  installation owns runtime side effects.
- Authorization, Preview, plan binding, idempotency, transaction, and audit
  semantics fail closed and remain consistent across channels.
- Public APIs expose sealed capabilities and read-only catalogs, not raw
  handlers, registries, database handles, transaction control, River types, or
  lifecycle internals.
- Module lifecycle is deterministic, reversible for process resources, and
  observable. Forward-only migrations are atomic and checksum protected per
  module.
- Official adapters are neutral and configured explicitly by consumers. They do
  not create users, roles, permissions, grants, domain records, or secrets by
  default.
- The official F0 durable profile uses one PostgreSQL control database with
  distinct owned application and River schemas. Action writes and task enqueue
  share one transaction. Cross-database and external-effect atomicity are not
  claimed.
- Durable task delivery is at least once. Consumers use stable task identity and
  idempotent external effects; River remains an adapter implementation detail.
- Business data belongs to the consumer and may live behind consumer-owned
  Connector contracts in any suitable database, warehouse, or API.
- Headless consumers build and run with Go alone. UI assets and branding belong
  to the consumer; optional Modary UI packages remain domain-neutral.
- Generated artifacts are deterministic and consumer-owned. Each changed file
  is installed by sibling rename; a multi-file set is not claimed to be one
  filesystem-wide or crash-atomic transaction.

## Quality Gates

Behavior changes use focused RED/GREEN/REFACTOR tests. Framework acceptance
requires public API and lifecycle tests, real PostgreSQL transaction and
migration tests, River enqueue/retry/recovery/concurrency tests, independent
consumer conformance, deterministic generation, transport contracts,
neutrality checks, vet, race, repetition, fuzz, build, and documentation
evidence.

## Git And Review Policy

Preserve user work, avoid destructive commands, retain historical evidence as
read-only records, and complete spec-compliance and engineering-quality reviews
before acceptance.

## Change Process

Constitution changes require explicit owner approval. The current durable
architecture and acceptance contract is
`.ai-platform/specs/005-postgres-task-runtime/spec.md`.
