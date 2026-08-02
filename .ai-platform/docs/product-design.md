# Current Product Contract

- Version: 6.0
- Status: Confirmed
- Last updated: 2026-08-02
- Source: explicit owner approval for the PostgreSQL-first rewrite

Modary is a Go-first modular application kernel, SDK, and build tool.
Independent applications compose public Module packages and execute typed
Actions through one governed Runtime across human, Agent, and background-task
channels. Consumers own domain language, business schemas, external data,
branding, UI, policy, bootstrap data, deployment, and release artifacts.

The current durable architecture and acceptance contract is
`.ai-platform/specs/005-postgres-task-runtime/spec.md`. PostgreSQL is the single
official durable profile. One control database contains an owned application
schema and a distinct River queue schema so Action state and task insertion can
commit atomically. Business data remains behind consumer-owned Connector
contracts and may use PostgreSQL, MySQL, warehouses, APIs, or other systems.

The public `task` contract exposes bounded transactional enqueue, immutable
runner configuration, retry metadata, and lifecycle without exposing River or
database transaction types. Delivery is at least once, so consumers use stable
job identity and idempotent external effects.

The PostgreSQL and task profile is published as the `v0.1.0-alpha.3` pre-v1 release contract.
`v0.1.0-alpha.2` remains the historical embedded-storage release and does not
contain this profile. Alpha 3 passes the release, PostgreSQL, remote-consumer,
documentation, and tag-CI gates.

Framework acceptance is self-contained. The Counter conformance application is
copied outside the repository, runs as its own Go module with `GOWORK=off`, and
proves public PostgreSQL, transactional task, restart, and worker contracts. No
downstream product repository is required to complete Modary acceptance.

Modary-owned source and documentation use Apache-2.0. Applicable third-party
licenses and notices remain preserved in the repository notice.
