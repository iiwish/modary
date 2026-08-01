# T022 Alpha 1 Release Review

- Reviewer: Codex primary direct reviewer
- Date: 2026-08-01
- Verdict: Rejected and superseded
- P0: 0
- P1: 1 release-blocking onboarding validation defect
- P2: 0

## Finding

The copied-out remote application correctly removed the local framework
replacement, resolved the public tag, and passed project verification and
generated drift. Its complete test command then invoked two conformance tests
whose checkout-only assertions were not guarded by the existing copied-out
environment signal.

## Decision

The public Go proxy cached the annotated tag before the defect was observed.
The tag is not moved, deleted, or reused. No GitHub release is created for it.
The T022 packet's failure policy authorizes a subsequent version, which T023
must validate from a new clean candidate.

## Claim Review

Alpha 1 is not called Released, Remote_Verified, or supported. The successful
quality, native-platform, tag preflight, and module-resolution facts remain
valid, but they do not override the failed release gate.
