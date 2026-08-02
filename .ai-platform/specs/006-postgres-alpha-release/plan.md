# PostgreSQL Alpha 3 Publication Plan

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-08-02

## Decisions

1. The next version is `v0.1.0-alpha.3` because Alpha 1 and Alpha 2 are already
   immutable and the PostgreSQL/River profile is consumer-visible and breaking.
2. The accepted T026 source is the implementation baseline. Publication changes
   only version, release, onboarding, governance, and evidence surfaces.
3. Every irreversible step follows a successful reversible check: prepare,
   validate, commit, clean candidate preflight, push main, hosted main CI,
   create local annotated tag, tag preflight, push tag, hosted tag CI, remote
   resolution, remote consumer, then GitHub prerelease.
4. A failed external gate stops publication. Existing public tags remain
   untouched, and a defective published Alpha 3 is superseded by a later tag.
5. Final evidence is committed to main after external verification; the release
   tag remains fixed at the candidate commit.
