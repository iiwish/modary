# Production Foundation F0 Specification

- Version: 1.0
- Status: Confirmed
- Date: 2026-08-04
- Approval source: owner approval of the proposed identity, deployment, and operations roadmap
- Target release: v0.3.0-alpha.1

## Purpose

Modary Production Foundation closes the gap between an accepted componentized
application framework and a production-capable backend foundation. It provides
secure replaceable identity, observable process behavior, and executable
deployment references without turning Core into an IAM, telemetry distribution,
container platform, or database operator.

## User Stories

- US-001: A product team can use local credentials for development and select a
  generic OIDC provider for production without changing business Modules.
- US-002: One authenticated person can be authorized independently in several
  product scopes without duplicating or changing the principal identity.
- US-003: An operator can distinguish a live process from an instance ready for
  traffic and observe readiness become false before graceful shutdown.
- US-004: An operator can run forward migrations separately from serving traffic
  and build/run the generated project as a non-root OCI container.
- US-005: A team can receive correlated structured logs and optionally export
  traces and metrics through OTLP without importing telemetry dependencies into
  unselected applications.
- US-006: A framework maintainer can prove every production capability from
  copied-out consumers and remote versioned modules.

## Functional Requirements

- FR-001: `identity.Actor` contains stable principal identity and display
  metadata but no product execution scope.
- FR-002: authorization evaluates an exact validated request scope against
  actor/type/scope role bindings and remains default deny.
- FR-003: password credential verification, revocable server-side session
  storage, bearer authentication, and browser login transport are independently
  selectable contracts.
- FR-004: Local Identity retains explicit provisioning, Argon2id verification,
  hashed credentials, rotation, revocation, bounded concurrency, and development
  login while using the new contracts.
- FR-005: an optional OIDC module supports discovery, Authorization Code with
  PKCE, state, nonce, exact redirect policy, issuer/audience/time verification,
  stable issuer/subject actor identity, bounded claim mapping, and revocable
  server-side application sessions.
- FR-006: OIDC does not grant Modary roles directly from claims. Consumer policy
  explicitly provisions or maps actors and RBAC bindings.
- FR-007: session cookies remain host-only, HttpOnly, SameSite, Secure by
  default, bounded, revocable, and CSRF-bound for application mutations.
- FR-008: generated applications expose separate `/livez` and `/readyz`
  endpoints with bounded public output; `/healthz` is not an ambiguous second
  readiness authority.
- FR-009: readiness is false until assembly completes, becomes false before
  drain, and can include bounded selected dependency checks without making
  liveness depend on remote systems.
- FR-010: generated API/Admin/Governed processes share deterministic signal,
  server-drain, application-shutdown, timeout, and exit semantics.
- FR-011: database-backed generated projects expose a distinct migration-only
  command or mode. Serving can be configured not to apply migrations.
- FR-012: generated projects include consumer-owned multi-stage OCI build
  source, non-root execution, minimal runtime files, build metadata, and local
  PostgreSQL Compose where the Profile requires it.
- FR-013: runtime logs are structured through `log/slog`, carry bounded request
  and trace correlation where present, and never intentionally record credentials,
  session tokens, authorization codes, database URLs, or complete request bodies.
- FR-014: an optional independently versioned OpenTelemetry module configures
  OTLP traces and metrics with bounded resource attributes, explicit lifecycle,
  context propagation, batch flush, and no hidden global initialization.
- FR-015: selected HTTP instrumentation records stable route templates, method,
  status class, duration, in-flight work, and server spans without actor IDs,
  scope IDs, raw paths, query strings, or request bodies as metric labels.
- FR-016: selected PostgreSQL/River operations expose bounded pool/task health
  and operational metrics through owned adapters without exporting raw SQL,
  payloads, or secrets.
- FR-017: every heavy component is absent from the root/API/default-Admin module
  graph unless explicitly selected.
- FR-018: Starter source and Admin UI adapt to local password or redirect-based
  OIDC sign-in without parallel hidden application modes.

## Non-Functional Requirements

- NFR-001 Security: OIDC negative tests cover state, nonce, PKCE, issuer,
  audience, expiry, redirect, duplicate parameter, oversized response, and
  upstream failure behavior. Authentication errors reveal no secret or account
  enumeration detail.
- NFR-002 Reliability: probes and shutdown are race-safe, context-aware,
  deterministic, and tested under concurrent requests and dependency failure.
- NFR-003 Performance: probe handlers allocate bounded output and complete local
  checks within their configured timeout; high-cardinality telemetry is rejected
  during construction.
- NFR-004 Portability: OCI output runs on Linux amd64 and arm64; generated Go
  projects continue to build on the existing platform matrix.
- NFR-005 Replaceability: OIDC and telemetry use explicit interfaces and source
  composition. Removing either removes its routes, configuration, dependencies,
  lifecycle, and generated source from a fresh project.
- NFR-006 Observability: startup, readiness transitions, shutdown, migrations,
  HTTP server behavior, and exporter failure have structured diagnostics.
- NFR-007 Compatibility: the breaking v0.3 Alpha contract has an explicit v0.2
  upgrade guide; published tags and applied migrations stay immutable.
- NFR-008 Quality: each behavior task uses RED/GREEN/REFACTOR, race tests where
  concurrency matters, copied-out acceptance, source-digest evidence, strict
  artifact validation, and four-pass final review.

## Scope

Production Foundation includes generic OIDC relying-party login, local
server-side application sessions, multi-scope RBAC, process probes/drain,
migration-only execution, generated OCI/Compose source, structured logs, optional
OTLP traces/metrics, operational documentation, and release automation.

## Non-Goals

- Password reset, MFA implementation, enrollment, account recovery, SCIM, user
  directory UI, social-provider-specific logic, or a hosted IAM service.
- Automatic trust of reverse-proxy identity headers or arbitrary OIDC claims.
- Kubernetes operators, hosted deployment, TLS issuance, WAF, secret-manager
  clients, PostgreSQL HA, backup storage, or automatic restore.
- A custom telemetry protocol, bundled observability backend, unrestricted
  `/metrics`, per-user metric labels, log shipping agent, or tracing requirement
  for Core/API projects.
- MySQL, SQLite, consumer product behavior, dynamic plugins, or stable-v1 API
  compatibility.

## Edge Cases

- A principal may have zero, one, or many scope bindings; zero is a valid
  authenticated but unauthorized state.
- OIDC subjects are unique only within an issuer. Email and display name may
  change and never identify the durable principal.
- IdP discovery/JWKS can fail at startup or callback; the application fails
  closed and preserves a public-safe response.
- An instance may remain live while unready because PostgreSQL or an exporter is
  unavailable. Exporter failure must not silently grant readiness or leak secrets.
- Drain may race with probes and active requests. New work is rejected after the
  readiness transition while accepted work receives its bounded shutdown window.
- A migration succeeds before a later process failure. It is never automatically
  reversed; recovery uses a forward migration or verified restore.
- Container builds run without frontend tooling at runtime and do not copy source,
  VCS metadata, credentials, or package caches into the final image.

## Success Criteria

- SC-001: one real disposable OIDC provider and adversarial protocol fixtures
  complete login, callback, current-session, logout, revocation, restart, and
  two-scope authorization acceptance.
- SC-002: copied-out API, local Admin, OIDC Admin, telemetry Admin, and Governed
  consumers build and test with `GOWORK=off`; absent dependency assertions pass.
- SC-003: OCI acceptance proves non-root execution, immutable embedded Admin
  assets, correct probes, migration-only operation, SIGTERM drain, and no Node
  runtime in the final image.
- SC-004: a disposable OTLP Collector receives HTTP traces and metrics while
  disabled telemetry adds no module dependency or listener.
- SC-005: database interruption, exporter interruption, active-request shutdown,
  and worker interruption produce correct readiness, exit, retry, and bounded
  diagnostic behavior.
- SC-006: full repository CI, vulnerability, race, repeat, fuzz, cross-build,
  deterministic generation, copied-out remote consumption, hosted CI, and
  coordinated tag gates pass with no unresolved P0 through P2 finding.

## Clarifications

- The owner approved OIDC rather than a Modary-owned IAM lifecycle.
- The owner approved container-first, platform-neutral deployment rather than a
  Kubernetes runtime dependency.
- The owner approved structured `slog` plus optional OpenTelemetry traces and
  metrics, keeping heavy dependencies outside Core.
- The owner approved completing and publishing the work as `v0.3.0-alpha.1`.

## Open Questions

None blocking. Exact dependency versions and disposable provider/collector
images are implementation decisions pinned by the technical plan and lockfiles.
