# Product And Competitor Research

- Version: 1.0
- Status: Complete
- Last updated: 2026-08-02
- Research snapshot: Gin-Vue-Admin commit
  `c69a66b07da787f0206a604ce4ae17f715f737b0`

## Research Question

What product and architecture constraints let Modary deliver the fast start of
an administrative-system framework without requiring every consumer to carry
an all-in-one platform?

## Owner Signal

The owner identified the primary Gin-Vue-Admin problem as weight: many bundled
features are unnecessary for a particular product, but the template and its
initialization model make selective ownership and removal harder than composing
only the required capabilities.

This is the first product constraint, not a request to reproduce every
Gin-Vue-Admin feature behind flags.

## Gin-Vue-Admin Repository Evidence

The current repository is intentionally a comprehensive full-stack platform.
At the research snapshot it contains 592 Go files and about 74,000 lines of Go,
161 Vue files and about 45,000 frontend source lines. Its server declares 56
direct requirements, including multiple SQL drivers, object-storage providers,
MongoDB, Redis, cron, Excel, Swagger, MCP, and code-generation dependencies.
Its frontend declares 46 production dependencies and 22 development
dependencies. These counts describe breadth, not quality, but confirm that a
consumer starts from a large product surface.

The snapshot also contains 1,251 source references matching `global.GVA_*`.
That number is not a defect count, but it is evidence that configuration,
database, cache, router, and logging ownership is often application-global
rather than injected through per-component capability boundaries.

Primary source:

- [Gin-Vue-Admin repository](https://github.com/flipped-aurora/gin-vue-admin)
- [server/go.mod at the research snapshot](https://github.com/flipped-aurora/gin-vue-admin/blob/c69a66b07da787f0206a604ce4ae17f715f737b0/server/go.mod)
- [web/package.json at the research snapshot](https://github.com/flipped-aurora/gin-vue-admin/blob/c69a66b07da787f0206a604ce4ae17f715f737b0/web/package.json)

## Issue Themes

The issues below span different project versions. They are not assertions that
every reported bug remains present. They are product signals showing recurring
friction around trimming, generated-code ownership, initialization coupling,
and replacement boundaries.

### 1. Consumers Want A Smaller Starting Point

- [Issue #457: request for a more minimal template](https://github.com/flipped-aurora/gin-vue-admin/issues/457)
- [Issue #1308: how to exclude the code generator from production](https://github.com/flipped-aurora/gin-vue-admin/issues/1308)

The second discussion exposes a real product tradeoff: a browser generator is
easy for beginners, while a production application should not expose or carry
development tooling by default. Modary resolves this by keeping creation and
generation in developer tooling and emitting a runtime that contains only
selected components.

### 2. Generated And Handwritten Code Need Separate Ownership

- [Issue #1557: repeated generation should preserve handwritten code](https://github.com/flipped-aurora/gin-vue-admin/issues/1557)
- [Issue #1534: repeated writes to generated enter files](https://github.com/flipped-aurora/gin-vue-admin/issues/1534)
- [Issue #1362: route registration remained after generator rollback](https://github.com/flipped-aurora/gin-vue-admin/issues/1362)

These reports show the structural cost of a generator that mutates many
framework-owned registration files. Modary generation therefore creates new
projects or complete dedicated generated artifacts. It does not use markers,
regular expressions, or partial source rewrites to preserve handwritten code.

### 3. Global Conventions Make Integration And Removal Expensive

- [Issue #498: hard-coded and inconsistent table conventions complicate integration](https://github.com/flipped-aurora/gin-vue-admin/issues/498)
- [Issue #1523: database changes need explicit migration records](https://github.com/flipped-aurora/gin-vue-admin/issues/1523)
- [Issue #2012: frontend visibility and backend permission behavior diverge](https://github.com/flipped-aurora/gin-vue-admin/issues/2012)

The Modary response is not more configuration switches. Each selected
component owns its schema, migrations, routes, UI contribution, and permission
contract. The application composes components explicitly and tests that an
omitted component is absent.

## Adjacent Framework Lessons

- [GoFrame](https://goframe.org/en/docs/components) demonstrates the value of a
  broad component catalog, but Modary keeps a smaller product-level scope and
  avoids framework-global object access as the primary integration model.
- [Buffalo](https://gobuffalo.io/) demonstrates that a coherent CLI, starter,
  database path, frontend path, and background work produce a complete developer
  experience. Modary adopts the complete path while keeping components explicit.
- [Kratos](https://go-kratos.dev/) is optimized for microservices, Protobuf,
  HTTP/gRPC, middleware, and observability. Modary targets business-oriented
  modular monoliths instead of competing for the same deployment architecture.
- [Encore](https://encore.dev/go) combines a backend framework with
  infrastructure automation and a development dashboard. Modary remains a
  library and consumer-owned application framework without requiring a cloud
  control plane.

## Product Decisions Derived From Research

1. The smallest application is genuinely small and database-free.
2. Features are selected by explicit composition, not disabled after a complete
   platform starts.
3. A component owns its runtime and migration surface; no central mutable
   registration file is patched by generators.
4. Project generation is create-only. Handwritten business source is never a
   regeneration target.
5. Admin is an optional first-party Profile because a framework inspired by an
   Admin product needs a visible, complete developer experience.
6. Governed Actions, River, audit, and MCP remain differentiators but are
   advanced opt-in capabilities rather than the definition of every application.
7. Modary competes on progressive disclosure and long-term maintainability, not
   on shipping the greatest number of bundled features.

## Owner Input Still Useful

Implementation can proceed from the confirmed weight and componentization
signal. The owner can improve prioritization later by identifying which
unneeded Gin-Vue-Admin features caused the most removal work and which three
features remained indispensable in real projects. This input is useful but not
blocking for the v0.2 F0 contract.
