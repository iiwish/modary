# T034 External Acceptance, Documentation, And Review

- Status: Completed
- Date: 2026-08-02
- Release state: Technical_Accepted
- Distribution state: Not released

Modary v0.2 F0 is accepted as a lightweight componentized Go backend framework.
Core starts without a database or external service. Create-only API, Admin, and
Governed Profiles produce explicit consumer-owned source with distinct concrete
adapter, infrastructure, route, migration, process, and UI selections.

Fresh projects generated outside the repository passed `GOWORK=off` tests, vet,
and builds. API required no infrastructure. Admin passed real PostgreSQL scoped
CRUD/restart and a copied-out frozen Vue pipeline while River and governed
adapters remained absent. Governed passed required Preview, Execute, default
deny, idempotent replay, SQL Audit, restart, River enqueue and post-restart task
consumption while Admin source remained absent.

The public documentation defines Core, components, Profile selection, ordinary
versus governed persistence, component authoring, production replacement
points, deployment, security, versioning, Alpha 3 migration, and release
boundaries. English and Chinese onboarding cover API, Admin, and Governed paths.
The Rulary guide keeps Rulary a separate product and recommends an Admin-first,
selectively governed adoption sequence.

Final product and engineering reviews contain no unresolved P0, P1, or P2
finding. The source targets `v0.2.0-alpha.1` but is not represented as released.
The immutable `v0.1.0-alpha.3` tag still resolves to commit
`f39457a52c10ceecd8defb77e0def1b331c45dd2` and tree
`84722726fa592f04e31596b873445ce566a2f857`.

Accepted limitations remain explicit: PostgreSQL is the only official F0 data
adapter, local Identity is not complete public-internet IAM, River delivery is
at least once, Starter creation does not patch consumer source, and Core may
link small provider-neutral contract packages without installing their optional
services.
