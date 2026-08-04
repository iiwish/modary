# Modary v0.2 F0 Known Limitations

These boundaries are part of the contract rather than an informal backlog.

## Product And Compatibility

1. Public Go APIs, generated source, frontend structure, and Modary HTTP/MCP
   schemas are alpha. Pin an exact version and review every upgrade before v1.
2. `modary new` is create-only. F0 has no patch, merge, eject, or generated
   application upgrade command. Consumer source remains consumer-owned.
3. Profiles are curated starting points, not every possible component
   combination. F0 has no interactive configurator, runtime plugin discovery,
   component marketplace, page builder, or low-code metadata engine.
4. Module composition is static Go code. Hot loading and unloading are outside
   the contract.
5. Core retains small provider-neutral contract packages for typed optional
   database, identity, authorization, Action, and task facades. Selecting API
   does not install those services, but F0 does not claim a separate Go module
   download or zero contract-package linkage for every component. Concrete
   PostgreSQL, River, Identity, RBAC, Audit, and transport selections remain
   independently absent.

## Data And Tasks

6. PostgreSQL 17 is the tested database line. F0 provides no official MySQL,
   SQLite, or other Store adapter. Consumers may implement `database.Store`,
   but adapter quality and migration behavior remain their responsibility.
7. The Admin Profile uses ordinary PostgreSQL transactions. It contains no
   transactional outbox or durable task service by default. `--with tasks`
   explicitly selects governed PostgreSQL and River-backed task inspection;
   `--with audit` explicitly selects scope-bound SQL audit inspection.
8. The Governed Profile uses one physical PostgreSQL database with separate
   application and River schemas. It does not provide distributed transactions,
   cross-database atomicity, automatic failover, or cross-service rollback.
9. River delivery is at least once. Task consumers and external side effects
   must be idempotent. A dedicated River database is unsupported where atomic
   governed write plus enqueue is required.
10. Database provisioning, role management, TLS, connection routing, backups,
   replication, failover, capacity, monitoring, and PostgreSQL upgrades are
   operator responsibilities.
11. Nested `postgresdb` transactions join the outer transaction and provide no
    savepoint. An inner error or panic marks the whole outer unit rollback-only.
12. Published migrations are forward-only. Recovery from an unsafe data change
    uses a new migration or a verified database restore, not an edited applied
    file.

## Identity And Security

13. Local Identity is for development and bounded private deployments. It is
    not a complete internet-facing IAM system: OAuth/OIDC, SSO, MFA, recovery,
    breached-password screening, centralized revocation, and abuse controls are
    not included.
14. RBAC supports explicit scoped roles, permissions, and row limits. It is not
    a general policy language and does not model delegation, hierarchy, or
    distributed attribute policy.
15. TLS termination, trusted-proxy rules, security headers, WAF, rate limiting,
    secrets delivery, and deployment monitoring remain application or
    infrastructure responsibilities.
16. Consumer callbacks, handlers, repositories, SQL, task consumers,
    configuration providers, and output writers are trusted process code.
    Modary is not a hostile plugin, source, or compiler sandbox.

## Governed Runtime

17. Preview plans have a five-minute default TTL and in-process cleanup. F0 has
    no retention scheduler, archival service, or operator UI for plans.
18. Audit provides bounded structured events and transactional success records.
    The optional Admin Audit log is a bounded, scope-bound metadata view.
    Retention, export, signing, external immutability, SIEM delivery, and audit
    administration are product or future-component work.
19. MCP implements bounded initialization, Action discovery, and tool calls.
    Resources, prompts, streaming, resumability, and broader protocol surfaces
    are outside F0.
20. Action JSON, schema, envelope, collection, pattern, numeric-work, and
    evaluation limits are enforced. Large documents or schemas must be split;
    increasing an HTTP envelope budget does not increase Action document
    limits. Draft 7 support excludes remote references, URI bases, anchors, and
    external resource retrieval.
21. Governed Access cannot begin a transaction. External databases or APIs are
    outside the local atomic unit and require product-level idempotency and
    integration design.

## Lifecycle And Tooling

22. Shutdown cancels active facade leases, but Go cannot forcibly terminate a
    trusted callback that ignores cancellation. Cleanup order is guaranteed by
    invocation, not by completion of non-cooperative callbacks.
23. Generation installs each output with a sibling rename. A complete multi-file
    set is not filesystem-wide or crash atomic. Cross-process serialization is
    a consumer CI responsibility.
24. The optional `projecttool build` has validated native filesystem and process
    behavior only on Linux and Darwin. Other listed platforms are compile-only.
    It executes trusted Go source and a trusted local toolchain; it is not a
    strong sandbox or a byte-reproducible build service.
25. CLI token-path hardening is supported on Linux and Darwin. Other operating
    systems accept token input through standard input only.
26. Node.js and pnpm are required to modify or rebuild Admin frontend source.
    They are not required by the API or Governed Profiles, or to run the
    prebuilt Admin Go binary.

## Distribution

27. The v0.2 F0 source is technically accepted but not yet released. Normal
    remote module resolution remains a release gate. The immutable published
    baseline is `v0.1.0-alpha.3` and has a different Governed-first onboarding
    contract.
