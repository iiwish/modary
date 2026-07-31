# Framework Decoupling Pre-Execution Analysis

- Version: 1.0
- Status: Completed
- Last updated: 2026-07-31

## Inputs

- User-approved framework boundary analysis.
- Current Kernel, Module Host, application bootstrap, transports, CLI, adapters,
  Console, CI, tests, delivery records, and Rulary assets.
- Three independent read-only reviews of Kernel contracts, consumer tooling, and
  domain neutrality.

## Findings Resolved By Plan

- Public application assembly replaces the inaccessible `internal/app` entry.
- Explicit consumer composition replaces same-repository scanning and generated
  Go registries.
- Pure Definitions replace database-backed catalog generation.
- Typed scoped services, automatic Action ownership, and lifecycle state enforce
  the Module boundary mechanically.
- Explicit transport wiring replaces concrete Module-ID switches.
- Configurable neutral adapters replace embedded users, policy, secrets, and grants.
- An independent consumer and neutrality gate replace Rulary-based framework CI.

## Worktree

The worktree contains the preceding F0 prototype and hardening work from the same
continuing delivery. No unrelated user-owned change was identified. Rulary-owned
assets must be transferred and checksum-verified before removal. The implementation
must build on the current Runtime improvements without resetting the worktree.

## Execution Decision

Critical: 0. High: 0 unresolved. Execution may proceed in T010 through T016 order.
