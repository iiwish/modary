# Current Product Contract

- Version: 8.0
- Status: Confirmed
- Last updated: 2026-08-02
- Source: explicit owner approval for the component-framework refoundation

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

## Compatibility And Release State

`v0.1.0-alpha.3` is the immutable accepted PostgreSQL and Governed Action
release. It remains available for pinned consumers. The active v0.2 contract is
pre-v1 and may replace Alpha 3 public APIs where that is necessary to make Core
lightweight and advanced capabilities optional. Retained guarantees receive
conformance tests; removed or replaced APIs receive an explicit upgrade note.

## Success Definition

The v0.2 F0 is accepted when independent copied-out consumers prove all three
Profiles, an Admin consumer can sign in and use a real business module, omitted
components are absent rather than dormant, Governed Action guarantees remain
available when selected, and the complete onboarding, test, build, and review
gates pass.
