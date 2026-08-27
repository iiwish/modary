# Security Maintenance Release Plan

- Version: 1.0
- Status: Confirmed
- Date: 2026-08-27
- Target: `v0.3.0-alpha.2`
- Approval source: explicit owner request to complete the release

## Decisions

1. Use Go 1.26.7, the current patch in the supported 1.26 line. Go 1.26.6 fixed
   the five reachable findings observed from Alpha 1, and Go 1.26.7 contains the
   subsequent upstream `net/http` fix.
2. Keep the framework, public API, database migrations, Profiles, and generated
   product behavior unchanged.
3. Publish the root and four component modules as one immutable five-tag train
   from one clean commit.
4. Treat the existing Design Partner Validation specification as review-only;
   Alpha 2 changes its future validation baseline but does not start that work.
5. If hosted candidate validation exposes a test-only synchronization defect,
   return to T049, fix the test without changing runtime behavior, and repeat
   candidate acceptance before any tag is created.

## Implementation

1. Change release-fixture and Starter assertions first so the old baseline and
   default version fail focused tests.
2. Update Go directives, Docker build arguments, module requirements, Starter
   defaults, current documentation, release automation, and changelog.
3. Run format, tidy, docs, focused tests, vulnerability, acceptance, race,
   copied-out Profile, container, and release-readiness gates with the exact Go
   1.26.7 binary.
4. Commit the complete candidate, confirm the worktree is clean, then run
   candidate preflight again against the commit.
5. Create and push all five annotated tags together, wait for hosted tag CI,
   verify normal remote module consumption and released containers, and publish
   the GitHub prerelease.
6. Record final evidence in a post-tag commit, push `main`, wait for hosted main
   CI, and close the goal only after local and remote state agree.

## Validation

- `make acceptance GO=<go1.26.7>`
- `make race GO=<go1.26.7>`
- `make ci GO=<go1.26.7>`
- `make release-readiness VERSION=v0.3.0-alpha.2 GO=<go1.26.7>`
- `make release-preflight VERSION=v0.3.0-alpha.2 RELEASE_MODE=tag GO=<go1.26.7>`
- `make remote-consumer VERSION=v0.3.0-alpha.2`
- `make released-container-acceptance VERSION=v0.3.0-alpha.2`
- strict T049 and T050 artifact validation
- hosted tag and final `main` CI

## Rollback

Before tag publication, fix the candidate and repeat every affected gate. After
publication, never move a tag; document any new defect and issue a later
prerelease.
