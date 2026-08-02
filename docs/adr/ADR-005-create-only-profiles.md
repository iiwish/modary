# ADR-005: Create-Only Profiles And Optional Admin Source

- Status: Accepted
- Date: 2026-08-02
- Scope: Starter generation, component selection, and Admin UI ownership

## Context

A componentized framework needs a fast path from an empty directory to a useful
application without turning one product bundle into the framework. A generator
that continually patches consumer source would need to own application layout,
merge arbitrary edits, and preserve product intent across versions. A runtime
component registry would hide package dependencies and weaken static review.

The framework also needs a practical response to the weight of all-in-one admin
systems: developers should select only the infrastructure and UI they actually
need, while still receiving a coherent starting point.

## Decision

Modary provides three named create-time Profiles:

- API selects database-free Core and explicit HTTP composition;
- Admin selects ordinary PostgreSQL, local Identity, RBAC, session HTTP, and a
  source-owned React admin baseline;
- Governed selects governed PostgreSQL, River, local Identity, RBAC, SQL Audit,
  Action transports, and a worker.

Profiles are regular source templates rendered by `starter.Create` and
`modary new`. Generated composition lists concrete Modules and adapters. A
component omitted from a Profile is absent from its source imports and package
graph. Profiles do not activate latent runtime plugins.

Creation accepts only a new or empty bounded destination and never patches an
existing project. A second creation fails without altering any file. Consumers
own every generated file and upgrade the pinned framework dependency
deliberately.

The Admin frontend is optional consumer-owned React source with an explicit
module registry and deterministic prebuilt assets. The framework does not scan
backend Modules to infer screens, menus, or forms. Adding a product Module
requires an explicit backend route/repository and, when applicable, an explicit
frontend module entry.

## Consequences

- A first project is useful without requiring developers to assemble every
  adapter from scratch.
- Core remains database-free and no Profile becomes a mandatory product shape.
- Package graphs express actual component selection and can be tested for
  absence.
- Consumer source stays readable, forkable, and removable.
- Framework upgrades may require manual source adaptation before v1.
- There is no promise that arbitrary edits can be merged by a future generator.
- The Admin baseline needs Node.js and pnpm only when its frontend is rebuilt;
  other Profiles and the deployed Go artifact do not.

## Rejected Alternatives

- One maximal starter makes unused databases, workers, auth, menus, and UI part
  of every product.
- An interactive matrix of every component combination creates an untestable
  compatibility explosion.
- Runtime plugin discovery hides composition, weakens compile-time absence, and
  introduces lifecycle and security complexity.
- Repeated template patching makes the framework a co-owner of product source
  and creates unsafe merge semantics.
- Schema-driven automatic admin screens couple product UX to backend metadata
  and recreate a low-code platform rather than a Go framework.
