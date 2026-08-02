# Modary PostgreSQL Alpha Readiness Report

- Report version: 2.1
- Status: Distribution_ready
- Technical F0 acceptance: Accepted
- Engineering readiness: Accepted
- Onboarding readiness: Accepted for source-checkout consumption
- Current source version: v0.1.0-alpha.3 release candidate
- Target version: v0.1.0-alpha.3
- Distribution status: Not_released
- Version tag: None
- Remote consumer verification: Not_run
- Latest published historical version: v0.1.0-alpha.2
- Canonical remote: https://github.com/iiwish/modary
- Owner-selected redistribution license: Apache-2.0
- Private security reporting channel: https://github.com/iiwish/modary/security/advisories/new
- Last updated: 2026-08-02

## Scope

The acceptance object is the independent Modary Go framework: public Kernel,
AppKit, application-command and HTTP/MCP integrations, pure project tooling,
neutral official adapters, PostgreSQL control storage, the River-backed public
task contract, and the independent Counter consumer.

PostgreSQL is the only official durable profile. One control database uses a
consumer-selected application schema and a distinct River queue schema.
The schemas are durably paired one-to-one and concurrent process bootstrap is
serialized. Consumers own business data and external Connector behavior.

## Current Result

T024 through T026 implement the PostgreSQL and task contract, port all standard
durable services and Counter, and remove the active embedded-database profile
without a compatibility layer. Real PostgreSQL tests cover migration integrity,
rollback-only nesting, Action-write/task-insert atomicity, task retry, restart,
simultaneous startup, exclusive schema pairing, duplicate suppression,
multi-runner claiming, credentialless service principals, cancellation, and
shutdown. Framework and independent
consumer acceptance, race, repetition, fuzz, build, neutrality, documentation,
and source-stability gates pass for the reviewed source state.

The copied-out Counter consumer proves that a governed Action can transactionally
enqueue durable work, stop its producer application, and consume the job after a
restart through public contracts alone. This self-contained conformance boundary
does not depend on a downstream product repository.

The current source is suitable for coordinated source-checkout development by
independent consumers and is the accepted `v0.1.0-alpha.3` candidate. It must
not be advertised as remotely consumable until the immutable tag, tag CI,
normal Go module resolution, and copied-out remote consumer gate all pass.

## Release Boundary

Publication freezes and commits the accepted source, runs release preflight and
full CI against PostgreSQL 17, creates the immutable `v0.1.0-alpha.3` annotated
tag, verifies normal Go proxy consumption, and publishes matching release
notes. Until those external checks pass, distribution remains unclaimed.
