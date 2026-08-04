# T041 Four-Pass Final Review

- Stage: Final
- Date: 2026-08-03
- Verdict: Pass
- P0: 0
- P1: 0
- P2: 0

## Spec Compliance

Pass. Core and generated API consumers omit concrete database and queue modules;
ordinary PostgreSQL and governed PostgreSQL/River remain separately selected.
HTTP/Admin composition is static, bounded, immutable, and preflighted before
Module startup. Optional task and audit surfaces are generated only when selected
and use public read-only contracts. No Rulary behavior, low-code generation,
runtime discovery, database breadth, or compatibility wrapper entered F0.

## Engineering Quality

Pass. Contribution dependencies and route declarations fail closed before
startup side effects. Builders run only for a matching ready application and
must return the declared route set. Physical-schema coordination uses one lock
identity and role marker across ordinary and governed components. Task and audit
cursor queries have index-aligned regression coverage, and signed 64-bit wire IDs
remain decimal strings.

Generated integration asserts the full backend descriptor contract rather than
module IDs alone. Frontend source selection independently fixes the supported
module/icon registry, and runtime resolution rejects duplicates, unknown modules,
missing permissions, or icon drift. Real records handlers prove authoritative
denial for list, create, update, and delete while preserving stored data; task and
audit routes retain equivalent direct denial coverage.

Fresh Governed generation is a required PostgreSQL test gate rather than a build
proxy. Its expected integration test must emit a non-skipped pass event. River's
46-byte queue-schema limit is validated during side-effect-free Module
construction, before schema creation or migration. Starter assigns distinct
role-prefixed namespaces to runtime and integration-test schemas, keeping every
accepted project ID outside PostgreSQL's reserved schemas. It independently
bounds names to the PostgreSQL and River limits and uses a stable SHA-256 fragment
when a readable name must be shortened. Copied-out maximum-length, `public`, and
`pg` Admins pass real PostgreSQL/River integration and reach runtime readiness
with generated defaults. Module Paths containing a Go `vendor` segment fail
before any destination is written, so every accepted path remains buildable by
the generated imports.

The document root is resolved to its physical canonical path before deciding
whether T041 digest enforcement applies. Make pins that root to the current
checkout, so caller-supplied alternate roots and equivalent relative or symlink
paths cannot weaken acceptance; direct script tests retain fixture injection
without changing the repository gate.

## UX And Accessibility

Pass. The generated Admin uses Chinese as its primary interface language while
preserving API names, task kinds, queue names, IDs, and audit source values.
Records, Tasks, and Audit provide stable loading, empty, populated, error/retry,
forbidden, filter, pagination, and session-expiry behavior. Shared primitives are
limited to concrete repeated work-surface needs. Desktop and mobile acceptance
show no overlap, horizontal overflow, accessibility violation, or browser log.
Mobile navigation remains a modal focus boundary with inert background, scroll
lock, focus cycling, Escape restoration, and selected-page heading focus.

## Release Verification

Pass. Multi-module quality gates, copied-out default and
operations Admin consumers, the copied-out Governed consumer, deterministic
assets, docs, neutrality, and generated state are current. The 8-minute repeat
budget covers curated concurrency and lifecycle contracts instead of multiplying
static packages. T041 records one digest over all candidate source outside its
own evidence directory; docs and release preflight reject unreviewed source
drift. The clean accepted commit, three immutable coordinated tags, hosted main
and tag CI, local and hosted remote consumers, and GitHub prerelease all passed.
The release remains pre-v1 and makes no stable compatibility claim.
