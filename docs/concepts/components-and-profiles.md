# Components And Profiles

Modary uses four distinct concepts. Keeping them separate prevents the
framework from becoming another coupled admin suite.

## Core

Core is `module`, `appkit`, lifecycle, typed capabilities, and small command/HTTP
composition helpers. An empty Core has no database and no Action Runtime.

## Standard Components

A component is an ordinary Module registration plus public contracts or
transport helpers. Examples include:

- `adapters/postgresdb`: ordinary PostgreSQL business Store;
- `adapters/localidentity`: explicit local principals and credentials;
- `adapters/rbac`: scope-aware backend authorization;
- `transport/sessionhttp`: session routes and CSRF middleware;
- `adapters/postgres`: governed persistence plus River tasks;
- `adapters/sqlaudit`: SQL-backed governed audit.

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
Profile -> generated consumer source -> appkit.Definition -> Module graph
```

The create-only Starter renders every output before writing, creates only an
empty/new destination, and refuses to patch later. After creation, a team edits
the project normally.

## Absence By Construction

Removing a component from a fresh Definition removes its capabilities,
migrations, routes, dependency packages, and generated frontend surface.
Feature flags that merely hide a route while retaining database and package
coupling do not satisfy this rule.

The API and Admin Profile tests inspect their complete Go dependency graphs to
prove River and governed components are absent. The Admin frontend registry is
also explicit, so omitted modules do not leave navigation or bundle artifacts.
