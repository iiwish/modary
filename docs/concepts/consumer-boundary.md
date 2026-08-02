# Framework And Consumer Boundary

Modary owns mechanisms that must remain consistent across application surfaces.
The consumer owns product meaning and deployment decisions.

## Modary Owns

- module manifest validation, dependency graph, lifecycle, and cleanup order;
- capability declaration, typed service identity, and scoped resolution;
- Action schema admission and immutable descriptor catalogs;
- identity, authorization, Preview and Execute binding, impact reauthorization,
  idempotency, transaction ownership, and required audit semantics;
- domain-neutral application assembly, command orchestration, HTTP/MCP
  projections, project verification, generation, and build policy;
- explicit official adapters for the supported PostgreSQL/local profile.

## The Consumer Owns

- domain entities, rules, migrations, handlers, permissions, roles, bindings,
  users, credentials, scopes, bootstrap data, and error vocabulary;
- the composition root and all optional capability providers;
- routes, TLS, proxy policy, UI assets, branding, API compatibility, and
  application commands beyond the framework projections;
- configuration loading, secret management, backup policy, deployment,
  observability integration, binaries, containers, and release cadence.

## Mechanical Boundary

Consumers import only public Modary packages. They never receive raw Action
handlers, mutable registries, a raw SQL database, transaction control, Host
internals, or framework-private persistence services. The application assembles
an opaque `appkit.Application` and uses read-only facades.

Definitions are explicit Go values. The framework does not discover modules by
directory, filename, build tag, database catalog, or runtime plugin scan. This
makes inspection deterministic and keeps product ownership in the consumer.

## Why The Boundary Matters

Cross-channel governance works only when transports cannot bypass the Runtime.
Domain independence works only when the framework does not understand a
consumer's entities or workflow. Release independence works only when the
framework publishes source contracts and consumers publish their own products.

## Extension Rule

Add a capability when multiple modules need one typed service contract. Add a
consumer transport when a product needs another surface. Contribute to Modary
only when the mechanism is domain-neutral, must be consistent for multiple
consumers, and can be tested without a consumer product.
