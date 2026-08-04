# Current Product Contract

- Version: 9.0
- Status: Confirmed
- Last updated: 2026-08-04
- Source: explicit owner approval for the component framework and v0.3 Production Foundation

## Product Definition

Modary is a lightweight, componentized Go framework for business backends and
administrative systems. It provides a maintainable modular-monolith foundation
with explicit, optional components and a complete path from project creation to
a running application.

The primary promise is:

> Start with a small Go application. Add only the components the product needs.

Modary combines the fast start expected from an administrative-system starter
with stronger ownership, replaceability, testing, and module boundaries than a
copied all-in-one template.

## Target Users And Jobs

The primary consumer is a Go team building a SaaS backend, enterprise business
system, operational tool, rule or configuration platform, or administrative
application as a modular monolith.

The framework helps that team:

- create a runnable project without assembling unrelated libraries by hand;
- select only the database, identity, authorization, task, audit, Admin, or
  governed-operation capabilities it needs;
- implement each business area as a self-contained component with explicit
  dependencies;
- remove or replace infrastructure without editing every business package;
- retain ordinary Go code, tests, builds, and deployment ownership;
- add stricter Action governance only to operations that justify it.

Modary is not optimized for a single small HTTP endpoint, a distributed
microservice platform, a runtime plugin marketplace, or a low-code CRUD
product.

## Product Layers

1. **Core** provides Module manifests, typed capabilities, deterministic graph
   assembly, lifecycle, metadata, and narrow service resolution. Core has no
   database or product feature requirement.
2. **Standard Components** provide optional HTTP, configuration, logging,
   PostgreSQL, identity, RBAC, audit, River task, health, and related adapters.
3. **Starter Tooling** creates an independent consumer project from a named
   Profile. It emits explicit composition and never copies Modary internals.
4. **Admin UI** provides an optional React 19 and TypeScript application shell,
   authentication flow, navigation contract, standard business-page
   primitives, task visibility, and audit visibility.
5. **Governed Operations** provide the existing typed Action, Preview/Execute,
   idempotency, transaction, audit, HTTP, CLI, and MCP capabilities as optional
   advanced components.
6. **Production Foundation** provides replaceable identity flows, process
   health and drain, migration execution, container reference artifacts,
   structured logs, and optional OpenTelemetry integration without making them
   requirements of Core or API Profile applications.

## Profiles

- `api` creates the smallest runnable HTTP application with health and one
  consumer-owned example module. It requires no database, queue, identity,
  governed Action, UI, or Node.js.
- `admin` adds PostgreSQL, development identity, RBAC, sessions, the Admin UI,
  and one consumer-owned business module. Optional task and audit components
  remain explicit selections.
- `governed` adds the accepted PostgreSQL transaction, River task, SQL audit,
  typed Action, HTTP Action API, CLI, and MCP composition to an application
  whose high-impact operations need those guarantees.

Profiles are presets implemented as visible source composition. Consumers can
remove, replace, or add components after generation without invoking a hidden
runtime profile engine.

## Component Contract

A component is an ordinary Go package that returns an inspectable registration.
It declares identity, version, type, required and provided capabilities,
optional migrations, and lifecycle callbacks. Components may contribute HTTP
routes or Admin metadata only through the corresponding optional capability.

An unselected component must have no observable effect. Component removal is
validated through the assembled graph and through absence tests for routes,
migrations, processes, configuration requirements, and generated artifacts.

Framework services are passed through typed capabilities. Business packages do
not read framework-global database, logger, router, configuration, or identity
variables.

## Identity And Authorization Boundary

Identity establishes an active principal with stable ID, type, and display
metadata. It does not assign the product's current workspace, tenant, project,
or resource scope. Consumer routes derive a validated scope from trusted
product context; the Authorizer evaluates durable bindings between that actor
and the requested scope.

Local password Identity remains an explicit development and controlled-internal
component. Production browser sign-in uses an optional OIDC relying-party
component with Authorization Code, PKCE, state, nonce, exact issuer/audience
verification, stable issuer/subject identity, bounded claim mapping, and
revocable server-side application sessions. The external provider owns password,
MFA, enrollment, recovery, and upstream account policy. Claim-to-role mapping is
never implicit.

## Generation And Ownership

`modary new` creates a new project only in an empty destination. The generated
project is consumer-owned, explicit, and immediately buildable. Framework
upgrades happen through Go module versions and documented migrations, not by
regenerating over handwritten business code.

Generated catalogs or clients use deterministic whole-file outputs placed in
dedicated generated directories. Modary does not splice code into arbitrary Go
or frontend source files.

## Admin UI Boundary

The Admin UI is optional and separately buildable. The F0 surface includes:

- sign-in, sign-out, session expiry, and current-actor state;
- accessible application layout and responsive navigation;
- module-owned navigation registration;
- typed API client and consistent loading, empty, error, and permission states;
- reusable list, form, details, confirmation, and status primitives;
- task and audit views when those components are present.

The UI does not include a low-code designer, database-schema editor, generic
report builder, or a promise to generate complete business applications from
tables. Headless consumers never install or run its frontend toolchain.

## Data And Runtime Boundary

Core requires no database. PostgreSQL is the first official application
database component and the durable prerequisite for standard Identity, RBAC,
SQL Audit, and River tasks. Business modules may use that component or provide
their own Connector capabilities.

The PostgreSQL application component exposes a narrow SQL query and transaction
capability for ordinary consumer repositories. Governed Action storage and
transaction authority is layered separately so routine CRUD does not require
Preview or Action persistence. F0 does not impose an ORM; an ORM integration can
be an additional component after the SQL contract is proven.

Selecting the River task component uses a distinct queue schema in the selected
PostgreSQL database and retains at-least-once delivery. Selecting Governed
Actions permits Action writes and task insertion to share the accepted exact
transaction. Cross-database atomicity is not claimed.

Removing a component changes future assembly and migration registration. It
does not automatically drop schemas, tables, or retained product data from a
database that was initialized by an earlier composition.

## Runtime, Deployment, And Operations

The generated process exposes distinct liveness and readiness endpoints.
Liveness reports only local process progress. Readiness reports application
lifecycle and bounded selected dependency checks, becomes false before graceful
drain, and reveals no secret diagnostics. Database migrations can run through a
distinct command so operators can separate schema change from serving traffic.

Starter deployment artifacts build a minimal non-root OCI image, preserve the
embedded Admin bundle, record build identity, and support local PostgreSQL
composition. They are reference source owned by the consumer, not a managed
runtime. Kubernetes, TLS, proxy policy, PostgreSQL high availability, secret
distribution, collectors, dashboards, and backups remain deployment choices.

Generated processes use structured `slog` output with request correlation and
redaction. An optional independently versioned OpenTelemetry component provides
OTLP traces and metrics for selected applications. It is absent from Core and
unselected dependency graphs. Operational metrics use bounded stable labels;
task and audit data remain available through their existing read-only contracts.

## Compatibility And Release State

`v0.3.0-alpha.1` is the current Production Foundation release.
`v0.2.0-alpha.1` remains the immutable React component-framework baseline, and
`v0.1.0-alpha.3` remains available for consumers pinned to its historical
Governed-first contract. Public APIs and generated source remain pre-v1 Alpha
contracts. Retained guarantees receive conformance tests; removed or replaced
APIs receive an explicit upgrade note.

## Success Definition

The v0.3 F0 is accepted when independent copied-out consumers prove all three
Profiles, local and OIDC Admin identities, multi-scope authorization, distinct
probe and drain behavior, migration-only operation, non-root container
execution, structured diagnostics, optional OTLP export, and complete absence of
OIDC/OpenTelemetry dependencies from Core and unselected applications. Governed
Action guarantees, deterministic generation, release checks, and remote module
consumption must remain green.
