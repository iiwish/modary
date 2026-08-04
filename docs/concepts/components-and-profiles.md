# Components And Profiles

Modary uses four distinct concepts. Keeping them separate prevents the
framework from becoming another coupled admin suite.

## Core

Core is `module`, `appkit`, lifecycle, typed capabilities, and small command/HTTP
composition helpers. An empty Core has no database and no Action Runtime.

## Standard Components

A component is an ordinary Module registration plus public contracts or
transport helpers. Examples include:

- `components/postgres`: ordinary PostgreSQL business Store;
- `components/postgres/identitystore`: explicit local principals and credentials;
- `components/postgres/rbac`: scope-aware backend authorization;
- `transport/sessionhttp`: session routes and CSRF middleware;
- `components/governedpostgres`: governed persistence plus River tasks;
- `components/postgres/sqlaudit`: SQL-backed governed audit.
- `components/oidc`: redirect-based OIDC browser authentication;
- `components/otel`: OTLP/HTTP traces and metrics with owned lifecycle.

Components declare what they require and provide. They do not auto-register
themselves and official Adapters do not import sibling Adapters.

## Consumer Modules

A consumer Module owns product behavior. It may provide typed services, declare
migrations, mount routes from its package, or declare governed Actions. The
records and limits examples demonstrate this layer.

## Profiles

A Profile is a source generator preset for a new project. It selects a coherent
initial component graph and example vertical slice. It is not a package, base
class, runtime flag, or compatibility layer.

```text
Profile + component selections -> generated consumer source
-> appkit.Definition + httpkit.Contribution plan -> Module and route graph
```

The create-only Starter renders every output before writing, creates only an
empty/new destination, and refuses to patch later. After creation, a team edits
the project normally.

## Absence By Construction

Removing a component from a fresh Definition removes its capabilities,
migrations, routes, dependency packages, and generated frontend surface.
Feature flags that merely hide a route while retaining database and package
coupling do not satisfy this rule.

The API and default Admin Profile tests inspect their complete Go dependency
graphs to prove River and governed components are absent. Admin operational
selections generate backend contributions, source registries, and distinct
production bundles. Omitted modules leave no route, navigation, source module,
bundle artifact, configuration requirement, exporter, or provider lifecycle.
