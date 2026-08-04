# Building Rulary On Modary

Rulary is a separate product repository. Modary supplies reusable mechanics;
Rulary owns rule-domain models, workflows, APIs, UI, deployment, and release
decisions. No Rulary package should be added to the Modary module.

This guide defines a low-risk adoption path using the exact Modary v0.3 release
or an explicitly coordinated source dependency during framework development.

## 1. Start With The Admin Profile

Rulary is expected to need authenticated operators, scoped authorization,
ordinary business data, and a usable work surface. The Admin Profile is the
closest baseline:

```bash
modary new rulary \
  --profile admin --with oidc --with otel \
  --module github.com/your-org/rulary \
  --name Rulary
```

Do not start with Governed merely because it is the most capable Profile.
Most rule browsing, draft editing, metadata maintenance, and validation are
ordinary product operations. Keeping them on `database.Store` avoids Preview,
River, audit, and worker complexity where it adds no user value.

Use `--with oidc` when Rulary has a production identity provider; keep local
password login only for isolated development. `--with otel` is appropriate when
the deployment has a reviewed Collector. Both remain replaceable components.

## 2. Establish The Product Boundary

Rulary owns at least these concepts:

- rule identity, metadata, content, and lifecycle state;
- version and draft semantics;
- validation and evaluation contracts;
- environments, publication, activation, and rollback policy;
- run history, diagnostics, and product audit requirements;
- product roles, scopes, navigation, and messages.

Modary must not define these nouns. It supplies Module composition, lifecycle,
PostgreSQL Store, Identity/RBAC building blocks, sessions, explicit routes, and
the optional governed mechanics Rulary may later select.

## 3. Replace The Example Vertical Slice

Treat generated `records` code as a structural example. Replace it rather than
renaming it indefinitely.

A practical first Module boundary is:

```text
internal/rules/
  module.go
  migrations/
  repository.go
  service.go
  http.go
  models.go
```

The Module owns its tables and forward migrations. Repositories accept
`database.Store`; services own domain rules; HTTP handlers translate transport
input and output. Register the Module and routes explicitly in the composition
root.

On the frontend, add `web/src/modules/rules` and register it in
`web/src/modules/index.ts`. The module owns its routes, navigation entry, API
client, screens, and tests. Delete the records module after the rules vertical
slice covers its structural role.

## 4. Build One Complete Vertical Slice

The first slice should prove product value without inventing the entire rule
platform:

1. sign in and resolve an operator scope;
2. list rules with stable pagination/filter semantics;
3. create a rule draft;
4. edit draft content under optimistic concurrency;
5. validate the draft and show structured diagnostics;
6. view version history;
7. restart the application and prove persistence and authorization.

Keep evaluation in-process initially when it is bounded and deterministic. Add
a durable worker only after real execution time, retry, or isolation needs are
measured.

## 5. Model Authorization In Rulary

Define product permissions such as `rules.read`, `rules.write`,
`rules.validate`, and later `rules.publish`. Bind them to product scopes and
roles in Rulary composition. Backend authorization is authoritative; frontend
visibility is only presentation.

Do not make framework RBAC names the public domain model. Wrap authorization
checks in Rulary services where richer product semantics will evolve.

## 6. Decide Which Operations Need Governance

Add governed components per high-impact operation, not per application. Good
candidates may include:

- publishing or activating a rule in a production environment;
- changing an organization-wide default policy;
- deploying a rule set to many targets;
- destructive rollback or bulk migration;
- an AI agent proposing and executing a material rule change.

Ordinary draft save, tag edit, description update, and list filtering should
remain ordinary Store operations unless product risk proves otherwise.

When Rulary adopts a governed operation, compose the governed PostgreSQL,
Audit, Action, and task components beside the Admin surface. Define a stable
Action ID, Preview summary, affected-resource set, optimistic state binding,
idempotency key, audit policy, and idempotent worker effect. Do not migrate all
existing routes automatically.

## 7. Keep Database Roles Clear

For an initial Admin-only Rulary deployment, use one PostgreSQL application
schema through `postgresdb`.

If governed publication is added, choose deliberately whether ordinary and
governed tables share the same physical PostgreSQL database. The governed
application and River schemas must satisfy the documented ownership and pairing
rules. Modary does not provide a distributed transaction across a separate
business database or external evaluator.

## 8. Replace Development Identity

Generated local Identity is suitable for local development and bounded internal
evaluation. Before a production or internet-facing Rulary deployment, select
the product's real identity boundary, session and revocation behavior, MFA/SSO
requirements, account recovery, proxy trust, rate limits, and credential
rotation.

This work belongs in Rulary or a reusable Identity adapter, not in rule Modules.

## 9. Acceptance Stages

### Stage A: Bootstrap

- generated Admin project passes backend and frontend gates;
- Modary is pinned exactly and local replacements are documented or removed;
- empty optional governed surfaces are absent.

### Stage B: Rules Vertical Slice

- records example is replaced by product-owned rules code;
- real PostgreSQL integration covers migration, CRUD, concurrency, scope, deny,
  and restart;
- frontend covers login, loading, empty, error, create, edit, validation, and
  mobile behavior.

### Stage C: Product Evaluation

- representative users can complete draft-to-validation work;
- domain terminology and diagnostics are understandable;
- latency, rule size, version volume, and authorization needs are measured.

### Stage D: Selective Governance

- only measured high-impact operations gain Preview/Execute and durable tasks;
- audit, replay, restart, and duplicate delivery are tested end to end;
- ordinary CRUD remains independent.

## 10. Dependency Rule

Rulary may import Modary public packages. Modary never imports Rulary. The
framework repository may use a narrow copied-out Rulary-shaped acceptance
fixture, but it must contain no product implementation and must not become a
second Rulary repository.

This boundary lets Modary mature as a general component framework and lets
Rulary make product decisions at product speed.
