# Changelog

All notable Modary changes are recorded here. Versioning follows semantic
versioning terminology; every pre-v1 release remains an Alpha contract unless
its release notes explicitly state otherwise.

## Unreleased

No changes are recorded after `v0.2.0-alpha.1`.

## v0.2.0-alpha.1 - 2026-08-04

### Added

- A create-only `modary new` Starter with database-free API, ordinary Admin,
  and Governed Profiles.
- A database-free component Core with explicit typed capabilities, dependency
  ordering, lifecycle, and HTTP composition.
- An optional React 19 Admin work surface with session/CSRF handling, backend
  RBAC, responsive navigation, records CRUD, accessible dialogs, and explicit
  frontend Module registration.
- A separate ordinary PostgreSQL `database.Store` component for business CRUD.
- Independently versioned ordinary and governed PostgreSQL component modules
  with one coordinated root/submodule release train.
- Static HTTP/Admin contribution contracts with side-effect-free bounded
  preflight, explicit capability requirements, and permission-aware metadata.
- Optional read-only task and audit Admin surfaces with shared focused React
  work-surface primitives.

### Changed

- PostgreSQL, River, Identity, RBAC, Audit, governed Actions, and frontend
  tooling are selected components rather than Core requirements.
- The Admin production bundle is deterministic, embedded in the Go binary, and
  served with ETag revalidation for its stable asset names.
- Documentation and copied-out acceptance cover all three Profiles in English
  and Chinese.
- Release gates scan production frontend advisories and reachable Go
  vulnerabilities with pinned tooling.
- Browser-session authentication is an explicit capability, and task
  inspection exposes a closed provider-neutral lifecycle vocabulary.
- Governed task inspection uses typed filter queries and component-owned
  queue-schema indexes aligned with its descending cursor.
- Starter schema defaults use distinct role-prefixed PostgreSQL namespaces,
  remain within PostgreSQL and River identifier limits, and avoid reserved
  schemas for every accepted project ID.
- Starter rejects Go Module Paths containing a `vendor` segment before writing
  files, because generated imports cannot use Go's vendoring namespace.

### Removed

- The Admin Starter has no Vue implementation, dependency, compatibility
  package, or dual frontend path.
- Product manifests and advanced governed tooling are not required by Starter
  projects.

### Compatibility

- The v0.2 source target is intentionally breaking and has no automatic Starter
  patch path. Generated source belongs to the consumer and is migrated through
  reviewed application changes.
- `v0.1.0-alpha.3` remains immutable. Pin `v0.2.0-alpha.1` exactly and repeat
  copied-out acceptance without a local replacement.

## v0.1.0-alpha.3 - 2026-08-02

### Changed

- The official durable profile uses PostgreSQL with separate application and
  River schemas and supports multiple API and worker processes.
- A domain-neutral `task` package provides transactional enqueue and immutable
  runner contracts while River and pgx remain adapter implementation details.
- Standard adapters, Counter, migrations, integration tests, CI, operations,
  and onboarding use the PostgreSQL profile.

### Removed

- The embedded database adapter, migrations, dependency graph, examples, and
  compatibility surface have been removed.

### Compatibility

- This release replaces the Alpha 2 durable profile rather than upgrading it in
  place. There is no automatic migration from the former embedded control store.
- Consumers configure PostgreSQL 17 with distinct application and River schemas,
  regenerate consumer-owned artifacts, and rehearse data migration or restoration
  before deployment.
- Public APIs and generated formats remain pre-v1 Alpha contracts. Pin
  `v0.1.0-alpha.3` exactly and review the upgrade guide and generated diff.

### Known Limitations

- See `docs/f0-known-limitations.md` and `docs/reference/support-matrix.md` for
  the supported PostgreSQL, platform, security, and task-delivery boundaries.

## v0.1.0-alpha.2 - 2026-08-01

### Fixed

- Remote example validation marks its temporary application as copied out, so
  checkout-only replacement and recursive-copy assertions are skipped while
  every application behavior, project, build, and version check still runs.

### Compatibility

- The framework runtime and public API are unchanged from `v0.1.0-alpha.1`.
- Consumers must pin `v0.1.0-alpha.2`; the earlier tag is not supported.

### Known Limitations

- The F0 limitations below remain unchanged. See
  `docs/f0-known-limitations.md` and `docs/reference/support-matrix.md`.

## v0.1.0-alpha.1 - 2026-08-01

### Release Status

- Rejected before GitHub release publication because the normal remote
  consumer exposed checkout-only tests that did not recognize the copied-out
  environment. The immutable tag remains for provenance and must not be used.

### Added

- Public Go packages for governed Actions, explicit module composition,
  application assembly, application commands, project tooling, HTTP/MCP
  projection, identity, authorization, audit, database access, and execution
  scope.
- Deterministic module dependencies, typed capabilities, lifecycle cleanup,
  Preview/Execute plan binding, authorization, idempotency, transactions,
  required audit, and bounded public errors.
- Official SQLite durability, local Identity, RBAC, and SQL Audit adapters with
  explicit provisioning and no product-owned defaults.
- Deterministic project verify, generate/check, and native build workflows with
  consumer-owned generated contracts.
- Public Counter example under `examples/counter`, including an independent Go
  module, migration, governed Action, CLI, HTTP, MCP, static UI, project tool,
  tests, copied-out conformance, and remote-consumer gate.
- Tutorials, concepts, how-to guides, package and manifest reference,
  troubleshooting, deployment, security, backup/restore, versioning, upgrade,
  contribution, and release documentation.
- Candidate and tag preflight, read-only GitHub tag CI, cross-platform compile
  contracts, race and repetition gates, fuzz smoke, generated drift,
  neutrality, and source-stability verification.

### Compatibility

- Go 1.26 or newer is required.
- The tag contents use the exact `v0.1.0-alpha.1` dependency identity and public
  packages only, but consumers must select the supported `v0.1.0-alpha.2` fix.
- Public APIs, generated formats, and migration policies are pre-v1 Alpha
  contracts. Upgrades require changelog and generated-diff review.
- Native project build is supported on Linux and Darwin under the documented
  filesystem and process policy. Windows amd64/arm64 is compile-only.

### Known Limitations

- The durable profile is one process and one SQLite database. High
  availability, distributed transactions, arbitrary privileged storage
  adapters, schedulers, and hostile plugin isolation are outside F0.
- Local Identity and RBAC are suitable for explicit local or private deployment
  profiles. Public-internet OAuth, SSO, MFA, TLS, proxy trust, and rate limiting
  remain consumer responsibilities.
- Modary provides no generic application binary, container, hosted UI,
  scaffold generator, global CLI, or downstream product release path.

See `docs/f0-known-limitations.md` and `docs/reference/support-matrix.md` for the
complete supported boundary.

## F0 Technical Acceptance - 2026-07-31

The framework source candidate completed local technical acceptance. This
milestone has no corresponding distributed version; release identity begins
with `v0.1.0-alpha.1` above.
