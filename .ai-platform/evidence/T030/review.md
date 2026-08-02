# T030 Starter And API Profile Review

- Stage: Final
- Date: 2026-08-02
- Verdict: Pass
- P0: 0
- P1: 0
- P2: 0

## Spec Compliance

The public API and actual command accept the required destination-first syntax,
support flags after the destination, and create the database-free API Profile.
The generated application starts and tests outside the framework workspace,
answers health and a feature route, and exposes nil optional Action and task
surfaces. Profile selection is visible in imports and composition source rather
than a runtime switch.

## Filesystem And Ownership

The generator owns files only during initial creation. Input and templates are
validated before writes, destinations must be absent or empty real directories,
files use exclusive creation, rollback removes only recorded paths, and a final
walk plus stable directory identity check detects external interference. Repeat
creation cannot merge, patch, or overwrite handwritten source.

## HTTP And Lifecycle

`httpkit` accepts standard `net/http` handlers, rejects nil handlers, invalid
methods and patterns, exact duplicates, and ServeMux pattern conflicts before
publishing a handler. It retains no global mutable router. The generated server
uses a synchronous listener, bounded header and idle behavior, SIGINT/SIGTERM
shutdown, and the AppKit lifecycle.

## Architecture And Engineering Quality

The global Starter command was decoupled from AppKit runtime dependencies and
its package dependency count fell from 235 to 94 in the checkout inspection.
Architecture inventory, API documentation, panic-boundary policy, copied-out
tests, race, vet, full suite, cross-build, documentation, strict artifact, and
diff gates pass. No current P0 through P2 finding remains.
