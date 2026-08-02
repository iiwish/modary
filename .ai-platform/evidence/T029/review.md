# T029 Core Optionality Review

- Stage: Final
- Date: 2026-08-02
- Verdict: Pass
- P0: 0
- P1: 0
- P2: 0

## Spec Compliance

An external package starts a feature-only application and serves readiness plus
a consumer-owned route without selecting infrastructure or governance
components. The Application exposes an empty Action catalog, nil Runtime, nil
task facade, and explicit unavailable identity result. Shutdown revokes
readiness and runs cleanup exactly once.

## Optionality And Boundaries

The runtime decision derives from the validated static Action declarations
already owned by the Host. It does not add a second configuration flag, global
registry, service-locator escape, fake provider, or implicit route. Optional
facades are still validated when present. Package graph inspection confirms
that database-free AppKit and HTTP consumers do not acquire a PostgreSQL driver,
PostgreSQL adapter, or River dependency.

## Governed Regression

Every application that declares an Action still assembles through the original
authorization, audit, plan, idempotency, and transaction requirements. Missing
governance tests were made semantically explicit by declaring an Action, and
they continue to prove rollback and exactly-once cleanup.

## Lifecycle And Engineering Quality

Concurrent assembly, assembly-versus-shutdown, facade revocation, readiness,
and cleanup tests pass under the race detector. The change is confined to the
assembly decision and public contract descriptions. Full package tests, focused
vet, documentation integrity, strict artifact validation, and whitespace gates
pass. No current P0 through P2 finding remains.
