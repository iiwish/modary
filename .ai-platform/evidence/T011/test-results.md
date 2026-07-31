# T011 Test Results

```text
go test -race -count=1 ./action ./audit ./authz ./database ./identity ./module ./scope
PASS

go vet ./action ./audit ./authz ./database ./identity ./module ./scope
PASS

go test -race -count=30 ./action -run 'TestRuntimePreviewExecuteAndIdempotency|TestRuntimeOptionalPreviewBindsExplicitPlanHash|TestConcurrentInlineIdempotencyDoesNotBindGeneratedPlanAsClientConstraint|TestRuntimeEnforcesPreviewPolicyBeforeIdempotencyLookup|TestRuntimeRejectsIdempotencyReplayAcrossActorChannelOrPlan|TestRuntimeRejectsIdempotencyReplayFromDifferentActionContract|TestRuntimeReauthorizesStoredImpactBeforeIdempotencyReplay|TestRuntimeContainsHandlerPanicsAndReleasesReservation|TestCleanupExpiredPlansParticipatesInRuntimeDrain|TestUnknownActionReleasesRuntimeLease'
PASS

go test -race -count=30 ./module -run 'TestNewRuntimeAndShutdownAreRaceSafeAndLinearizable|TestShutdownDuringStartCancelsWaitsAndRollsBack|TestShutdownCancelsAndDrainsInFlightRuntimeBeforeCleanup|TestShutdownDeadlineLeavesBackgroundDrainSafeAndObservable|TestStartPanicRollsBackResourcesAndFailsDeterministically'
PASS

git diff --check -- action audit authz database identity module scope internal/authority go.mod go.sum
PASS
```

Focused coverage includes Definition purity, capability and service-key rejection,
public catalog shape, prepared-contract ownership, strict Descriptor/SemVer/schema
validation, external `internal` import rejection, fail-closed Runtime dependencies,
plan integrity and contract drift, exact numeric canonicalization, idempotency
state/identity/channel/plan/contract provenance, concurrent inline reservations,
preview-policy ordering, replay authorization and audit provenance, handler and
lifecycle panic containment, startup/shutdown races, execution drain, reverse/LIFO
cleanup, migration atomicity/history integrity, and scope/identity value ownership.
