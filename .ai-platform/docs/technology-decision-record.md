# Current Technology Decisions

- Version: 5.0
- Status: Confirmed
- Last updated: 2026-08-01

The current storage, transaction, task, schema-topology, deletion, and
validation decisions are recorded in
`.ai-platform/specs/005-postgres-task-runtime/plan.md` and
`docs/adr/ADR-003-postgresql-and-module-migrations.md`.

The framework kernel and consumer boundary remain governed by
`.ai-platform/specs/002-framework-decoupling/`. Release policy, documentation
architecture, Apache-2.0 licensing, onboarding, and immutable tag rules remain
governed by `.ai-platform/specs/003-release-readiness/` and
`.ai-platform/specs/004-onboarding-release/` where they do not conflict with
the current PostgreSQL-first contract.

PostgreSQL uses `pgx/v5/stdlib` behind `database/sql`; River uses the same pool
and exact governed transaction for insertion. Application and queue schemas are
distinct and explicitly configured. River owns queue mechanics, Modary owns
domain-neutral task contracts and lifecycle, and consumers own business state
and external effects.
