# Rulary MVP Delivery Contract

- Milestone: F1
- Product: Rulary
- Status: Ready for implementation
- Delivery model: Modary monorepo modules

## Objective

F1 delivers the smallest Chat-first RuleOps workflow that tests whether a
business user can describe an address-label rule, understand the generated
rule, correct it with real samples, and publish a governed version without
editing SQL or RuleSpec JSON.

The Modary Module and Action Runtime remain the execution and governance
foundation. The F0 address-specific RuleSpec is a contract fixture for kernel
acceptance, not the Rulary v1 domain schema.

## Product Flow

```text
Import a bounded CSV or select the local sample dataset
-> describe the desired labels in Chat
-> receive a structured Rule Draft
-> inspect a business-language rule card
-> preview current and candidate results side by side
-> correct a sample or request a focused change
-> inspect the RulePatch and result diff
-> validate and publish an immutable version
-> inspect result evidence and the Action audit chain
```

## Scope

- Workspace-owned local CSV import with explicit source schema and entity key.
- Rulary RuleSet v1 envelope and versioned RuleSpec documents.
- A minimal typed expression subset: field references, constants, boolean
  composition, equality, contains, regex match, and deterministic assignment.
- Structured `RuleDraft` and base-version-bound `RulePatch` contracts.
- RuleSpec validation, canonical hashing, and immutable published versions.
- Business-language rule cards derived from RuleSpec.
- Sample preview with current/candidate values, evidence, and row-level diff.
- One model-provider adapter behind a provider-neutral interface, plus a
  deterministic fake used by contract tests.
- Human confirmation before applying a patch or publishing a version.
- Action-based authorization, plan validation, idempotency, and audit.
- Password rotation and non-loopback demo-secret rejection for pilot safety.

## Non-Goals

- Scheduled or distributed execution.
- PostgreSQL, MySQL, SQL Server, or Oracle connectors.
- Production writeback to an external database.
- SQL pushdown, DuckDB, Python workers, or AI batch execution.
- Full decision tables, approval workflows, review queues, or model routing.
- A standalone Modary SDK, module marketplace, or independent app scaffold.

## Architecture Boundary

- `core` contains no Rulary terms or RuleSpec semantics.
- Rulary domain logic remains inside Rulary feature modules.
- React routes and navigation are loaded from generated Module UI entries.
- Chat produces typed drafts or patches and never calls stores directly.
- Preview and publish use the same RuleSpec validator and evaluator.
- Model output is untrusted input and must pass schema and semantic validation.
- F0 `rulary.ruleset.f0` data remains readable only as a migration/test fixture;
  new F1 records use the Rulary v1 schema.

## Acceptance

1. A fresh database and a user-supplied CSV complete the full Product Flow.
2. The user-facing default path does not require editing JSON, SQL, or an AST.
3. Invalid model output and invalid patches fail closed with structured errors.
4. A patch with a stale base version is rejected.
5. Preview shows source, current result, candidate result, evidence, and diff.
6. The business rule card and evaluator derive from the same RuleSpec hash.
7. Published versions are immutable and every write is idempotent.
8. Model calls cannot write results, publish versions, or bypass Action authz.
9. Unit, contract, integration, and desktop/mobile E2E tests pass.
10. The release remains one Go binary, embedded React assets, and one SQLite
    system database in the default local mode.

## Exit Decision

Continue when pilot users can create and correct the address rule through Chat
and samples faster than their current SQL/document workflow, while correctly
explaining why changed rows will change. Revisit the product problem when that
workflow does not produce a measurable usability or confidence improvement.
