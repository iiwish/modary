# Modary PostgreSQL Alpha Readiness Report

- Report version: 2.2
- Status: Remote_verified
- Technical F0 acceptance: Accepted
- Engineering readiness: Accepted
- Onboarding readiness: Accepted for source-checkout consumption
- Current source version: v0.1.0-alpha.3
- Target version: v0.1.0-alpha.3
- Distribution status: Released
- Version tag: v0.1.0-alpha.3
- Remote consumer verification: Passed
- Latest supported release: v0.1.0-alpha.3
- Candidate and tag commit: f39457a52c10ceecd8defb77e0def1b331c45dd2
- Annotated tag object: 55e0a5b0d7b8a8422f4e9bd2e504b7d61d50d9c0
- Main CI: https://github.com/iiwish/modary/actions/runs/30728949673
- Tag CI: https://github.com/iiwish/modary/actions/runs/30729209127
- Release: https://github.com/iiwish/modary/releases/tag/v0.1.0-alpha.3
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

The accepted source is published and remotely consumable as
`v0.1.0-alpha.3`. The immutable annotated tag, hosted main and tag CI, normal Go
module resolution, and hosted and local copied-out remote consumer gates all
identify candidate commit `f39457a52c10ceecd8defb77e0def1b331c45dd2`.

## Release Boundary

Alpha 3 is `Remote_Verified`. Future releases repeat the clean candidate,
immutable tag, hosted CI, normal module resolution, copied-out consumer, and
matching release-note process. Published tags and migrations remain immutable.
