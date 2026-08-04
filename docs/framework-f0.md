# Modary v0.2 F0 Framework Contract

- Product: lightweight, componentized Go backend framework
- Source target: `v0.2.0-alpha.1`
- Distribution status: not released
- Frozen published baseline: `v0.1.0-alpha.3`
- License: Apache-2.0

This document is the canonical technical contract for the v0.2 F0 source. It
defines what Core guarantees, which official components exist, what each
Starter Profile selects, and where consumer ownership begins.

## 1. Product Boundary

Modary helps a Go team build a modular monolith without accepting a bundled
product. It provides explicit Module composition, typed capabilities, ordered
lifecycle, narrow infrastructure contracts, selectable official components,
and create-only starters. A generated project is ordinary consumer-owned Go
and, for the Admin Profile, React source.

Modary is not an admin product, low-code platform, ORM, workflow engine,
runtime plugin marketplace, or mandatory architecture for every application.
It does not own consumer domain models, menus, routes, deployment manifests, or
release cadence.

The governing rule is:

> Core defines composition and lifecycle. Components add capabilities.
> Profiles select a coherent starting set. The application owns the product.

## 2. Core

Core consists primarily of `module`, `appkit`, `httpkit`, and their narrow
contract packages. It is database-free and does not require PostgreSQL, River,
Identity, RBAC, Audit, governed Actions, MCP, React, Node.js, or a project
manifest.

### 2.1 Module Definition

A consumer defines Modules in Go. Each Module has a stable manifest and may
declare dependencies, capabilities, migrations, lifecycle callbacks, and
governed Actions. Go source is the only runtime composition source.

The graph:

- rejects duplicate IDs and invalid dependencies;
- detects cycles before runtime effects;
- resolves dependencies in deterministic order;
- starts providers before dependants;
- cleans up in reverse dependency order;
- validates required capabilities before invoking Module startup;
- retains no source-scanning or hidden registration path.

Module identity is a persisted contract when used in migrations, Actions,
permissions, or generated metadata. Consumers do not rename it casually.

### 2.2 Capability Model

Capabilities are typed keys resolved through the Module assembly boundary.
They express actual dependencies rather than enabling a service locator.
Providers are installed explicitly by the composition root or a selected
component. Missing required capabilities fail startup.

Consumer code imports the narrow public contract it needs. Official adapters
do not import sibling adapters; the composition root joins them.

### 2.3 Lifecycle

Definition inspection is pure. It opens no database, starts no worker, creates
no Handler, and performs no migration. Runtime assembly and startup are
separate operations.

Startup is single-use. A failed start is terminal for that Host. Shutdown
revokes new facade leases, cancels active leased contexts, and performs
exactly-once cleanup. Trusted callbacks must honor context cancellation; Go
cannot forcibly terminate a callback that does not cooperate.

### 2.4 Optional Runtime Facades

`appkit.Application` exposes only assembled capabilities. Database, Identity,
Authorizer, Action Runtime, and task behavior are optional. A database-free
application receives no synthetic database facade. Declaring a governed Action
without its required providers fails closed.

### 2.5 HTTP Composition

`httpkit` composes explicit standard-library handlers under bounded prefixes.
It does not scan Modules or infer routes. Consumer modules own route paths,
request models, handlers, and presentation behavior.

## 3. Public Infrastructure Contracts

### 3.1 Ordinary Business Store

`database.Store` is the provider-neutral contract for ordinary application
data. It supports reads, writes, and explicit `WithinTransaction` units. The
consumer owns repositories, SQL, migrations, and transaction boundaries.

`components/postgres` is the official PostgreSQL implementation. It does not
install River, Action persistence, audit tables, or governed transaction
authority.

### 3.2 Governed Database Access

`database.Access` is a deliberately narrower contract used inside governed
Action execution. It cannot begin its own transaction. Mutations are accepted
only while the Runtime owns the transaction. This prevents a Handler from
escaping idempotency, audit, and task atomicity.

### 3.3 Identity And Authorization

`identity` and `authz` are contracts. `components/postgres/localidentity` and
`components/postgres/rbac` are selectable implementations for development and
bounded internal deployments. Core does not select an identity model or default
policy. Principal Identity and browser-session authentication are distinct
capabilities: session-aware HTTP/Admin contributions declare
`module.CapabilitySessions` instead of relying on broad Identity availability.

`transport/sessionhttp` supplies standalone login, current-session, logout,
CSRF, and authenticated middleware for Admin applications. It does not require
the Action Runtime.

### 3.4 Governed Actions

The governed component is for high-impact mutations that need Preview,
authorization over intended effects, optimistic state binding, idempotency,
transactional audit, and durable follow-up work.

The flow is:

1. authenticate actor and validate input;
2. authorize intent;
3. preview current and intended effects;
4. bind an expiring plan to actor, scope, input, and state;
5. execute the exact plan in one framework-owned transaction;
6. commit business control state, idempotency, required audit, and task insert
   together;
7. return stable public results and errors.

This path is optional. Ordinary Admin CRUD does not need Preview or River.

### 3.5 Durable Tasks

`task` contains provider-neutral enqueue, runner, and bounded inspection
contracts. Inspection exposes the closed states `queued`, `pending`,
`scheduled`, `running`, `retrying`, `succeeded`, `failed`, and `cancelled`;
queue-specific states never cross the public contract.
`components/governedpostgres` implements the governed PostgreSQL control store and River
queue. It uses distinct application and queue schemas in the same physical
database so a governed write and job insertion share one PostgreSQL
transaction. Delivery is at least once; task consumers must be idempotent.

River tables need a schema, not a dedicated database. PostgreSQL provisioning,
TLS, backup, failover, and monitoring remain operator responsibilities.

## 4. Official Profiles

Profiles are create-time selections, not runtime modes. Generated composition
and source contain the selected concrete component set. Omitted adapters and
infrastructure libraries are absent from the consumer package graph. Core's
small provider-neutral contract packages may remain because Module and AppKit
expose typed optional capabilities; their presence installs no service.

### 4.1 API Profile

`modary new <destination> --profile api` creates a database-free HTTP service
with:

- explicit application Definition and lifecycle;
- health and sample feature routes;
- Go tests and buildable command;
- no database, Identity, RBAC, Action, Audit, River, MCP, or frontend.

Use it for small APIs, gateways, or services whose storage and auth decisions
are not yet selected.

### 4.2 Admin Profile

`modary new <destination> --profile admin` creates a source-owned internal admin
application with:

- ordinary PostgreSQL Store;
- local Identity and scoped RBAC;
- session and CSRF HTTP component;
- React 19, React Router, Lucide React, and TypeScript source;
- small typed context providers instead of a mandatory global state library;
- an explicit frontend module registry;
- responsive login, shell, and records CRUD vertical slice;
- permission-aware contribution navigation and record commands;
- optional `--with tasks` and `--with audit` read-only operational surfaces;
- deterministic prebuilt assets embedded by the Go binary.

Default Admin contains no River schema, worker, Action Runtime, SQL Audit, MCP,
or governed endpoint. Optional task and audit selections add only their declared
backend capability, route, React source, and selection-specific bundle. The
sample records module is instructional application code, not a framework domain
model. Consumers replace it with their own Modules.

Node.js and pnpm are needed when changing or rebuilding Admin frontend source.
They are not needed to run the built Go artifact.

### 4.3 Governed Profile

`modary new <destination> --profile governed` creates a headless governed service
with:

- governed PostgreSQL and River components;
- local Identity, RBAC, and SQL Audit;
- a required-preview `limits.set` consumer Action;
- HTTP, CLI, and MCP Action exposure;
- a durable worker consuming `limits.changed` tasks;
- integration tests for Preview, Execute, replay, restart, audit, and task
  consumption.

It includes no Admin UI. A product may adopt governed capabilities later for
only the operations that justify them.

## 5. Starter And CLI Contract

`starter.Create` and `modary new` are create-only. They accept a valid Go
module path, one known Profile, and a new or empty destination. They reject
non-empty destinations, symlink traversal, unsafe paths, invalid names, and
repeat creation. Module paths containing a `vendor` segment are rejected because
Go assigns that directory special import semantics. A failed creation rolls back
files created by that attempt.

The generated project is intentionally visible rather than hidden behind
configuration:

- the composition root lists selected adapters and Modules;
- routes are mounted explicitly;
- migrations and SQL remain in consumer source;
- frontend modules are explicitly registered;
- `go.mod` shows the actual dependency set.

There is no patch or automatic upgrade command in F0. Consumers upgrade the
pinned module deliberately and review source changes. This avoids a generator
silently rewriting product-owned code.

`projecttool` and `modary.yaml` remain optional advanced tooling for consumers
that want deterministic graph, Action catalog, TypeScript contract, and
constrained build outputs. Starter projects do not need them.

## 6. Admin UI Contract

The Admin UI is an optional baseline, not a universal backend console. It must
remain useful after generation and easy to delete or replace.

F0 guarantees:

- authentication restoration and protected routes;
- desktop and mobile navigation;
- explicit module navigation entries and route registration;
- list, create, edit, delete, loading, error, and empty states;
- keyboard-operable dialogs, focus restoration, and accessible labels;
- bounded responsive layouts without horizontal overflow;
- deterministic frontend build and embedded asset verification.

F0 does not provide a page builder, schema-driven form engine, visual policy
editor, dashboard marketplace, theme marketplace, or runtime frontend plugin
registry.

## 7. Security Boundary

Modary treats consumer Modules, callbacks, SQL, handlers, task consumers,
configuration, and output writers as trusted process code. It validates its
own public boundaries but is not a hostile plugin or build sandbox.

Local Identity credentials generated for development must be replaced before
deployment. Internet-facing products require an appropriate external identity
adapter plus product-specific session, recovery, MFA, proxy, rate-limit, and
abuse controls.

Profile absence is a security property: selecting API or Admin cannot expose
governed Action, task, audit, or MCP surfaces that were not compiled into the
application.

## 8. Persistence Ownership

Each consumer Module owns its schema objects and ordered forward migrations.
Applied migration history is immutable. The controller validates complete
history and commits one Module's pending migrations atomically. Cross-Module
migrations are ordered, not one global transaction.

Ordinary Admin data may use the selected PostgreSQL Store. A consumer can
implement another `database.Store` without changing Core. F0 provides no
official MySQL or embedded adapter and makes no cross-database transaction
claim.

## 9. Acceptance Contract

F0 acceptance requires all three Profiles to be generated outside the source
tree and verified with `GOWORK=off`:

- API tests and builds without optional component packages;
- Admin tests, real PostgreSQL CRUD/restart, frontend frozen install, lint,
  types, tests, build, asset parity, and absence of River and governed
  adapters/routes/services;
- Governed tests, real PostgreSQL/River Preview-to-task flow, restart, worker,
  CLI/HTTP/MCP parity, and absence of Admin UI;
- full repository tests, race tests, vet, formatting, tidy, repeat, cross-build,
  documentation, generated-state, import-direction, and diff checks;
- product and engineering review with no unresolved P0, P1, or P2 finding;
- visual QA at desktop and mobile viewports with no browser errors.

The detailed current result is in
[F0 acceptance report](f0-acceptance-report.md). Operational exclusions are in
[known limitations](f0-known-limitations.md).

## 10. Release Boundary

`v0.2.0-alpha.1` is the current component-framework release.
`v0.1.0-alpha.3` remains the immutable historical Governed-first baseline.
Consumers pin the complete v0.2 release train and review the documented
breaking migration before adoption. No acceptance document moves or rewrites
a published tag.
