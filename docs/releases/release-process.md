# Release Process

This process publishes the Modary Go source module and documentation. It does
not publish a consumer application, container, UI service, or product database.

## Roles

- The owner approves repository visibility, license, canonical remote, version,
  and final publication.
- The maintainer prepares and validates the exact candidate.
- The reviewer checks compatibility, security claims, Profile conformance, and
  evidence against that commit.

One person may hold several roles, but owner publication approval is explicit.

## Prerequisites

1. The current F0 acceptance report is `Accepted`.
2. `CHANGELOG.md` describes the exact candidate and migration impact.
3. The Apache-2.0 license and third-party notices are present.
4. `origin` is the canonical repository.
5. The candidate worktree is clean and all intended changes are committed.
6. The proposed version is an unused semantic prerelease such as
   `v0.2.0-alpha.1`.
7. English and Chinese onboarding, examples, support matrix, security,
   limitations, and release notes describe the same candidate.
8. Every copied-out Profile and the Admin frontend pipeline pass from the exact
   commit.
9. The T041 source digest matches the complete candidate. Finalizing the
   changelog or changing any candidate file requires refreshing T041 evidence.

## Candidate Validation

Set but do not create the intended tag:

```bash
VERSION=v0.2.0-alpha.1
make bootstrap
make acceptance
make ci
make release-readiness VERSION="$VERSION"
```

Candidate mode must state that no release is claimed. Record the commit ID,
toolchain versions, PostgreSQL version, Profile results, frontend lockfile
result, and immutable Alpha 3 tag identity.

## Review And Approval

Review the complete candidate diff from the last published tag. Confirm that:

- Core remains database-free;
- each Profile contains only its selected components;
- ordinary and governed transaction authority remain separate;
- generated projects work outside the Modary checkout with `GOWORK=off`;
- no Rulary or other product domain has entered framework packages;
- production frontend dependencies and reachable Go vulnerabilities are clear;
- known limitations and breaking migration notes are complete;
- no P0, P1, or P2 review finding remains.

The owner then gives explicit approval to publish that exact commit and version.

## Tag And Publish

The root module and both published component modules use one version and one
commit. Go resolves nested modules through subdirectory tags, so all three
annotated tags are mandatory:

```bash
git tag -a "$VERSION" -m "Modary $VERSION"
git tag -a "components/postgres/$VERSION" -m "Modary PostgreSQL $VERSION"
git tag -a "components/governedpostgres/$VERSION" -m "Modary Governed PostgreSQL $VERSION"
git push origin \
  "$VERSION" \
  "components/postgres/$VERSION" \
  "components/governedpostgres/$VERSION"
```

Push the release train together, then wait for root-tag CI. The release
preflight rejects a missing, lightweight, mismatched, or differently targeted
component tag. Never move or reuse any published tag.

## Remote Verification

Verify normal Go module resolution without a source-checkout replacement:

```bash
make remote-consumer VERSION="$VERSION"
go list -m "github.com/iiwish/modary@$VERSION"
go list -m "github.com/iiwish/modary/components/postgres@$VERSION"
go list -m "github.com/iiwish/modary/components/governedpostgres@$VERSION"
```

The remote gate creates a fresh governed consumer outside the repository,
removes every root and component `replace`, verifies all three module versions,
and repeats its tests and builds. Copied-out API and Admin verification, including
the pinned frontend pipeline and deterministic assets, runs from the exact
candidate before the release train is tagged.

Create the repository-host release from the same tag. Release notes include
scope, breaking changes, supported Profiles and platforms, database and
frontend requirements, upgrade guide, known limitations, security contact, and
remote-consumer result.

## Post-Release Checks

- the module query resolves the expected version and commit;
- a new temporary consumer builds without `replace` or a Go work file;
- release links, changelog, license, and source tag are visible;
- tag CI and review evidence reference the same commit;
- a design-partner application pins and validates the exact version.

## Failure And Rollback

Before tag publication, repair the candidate and repeat all gates. After an
immutable tag is published, document defects and issue the next prerelease or
patch. For unsafe migrations, stop rollout and restore verified consumer data;
do not edit the published migration or tag.

## Release States

- `Technical_Accepted`: local implementation and copied-out Profiles pass.
- `Engineering_Ready`: docs, policy, automation, and reviews pass.
- `Distribution_Ready`: exact clean commit, owner approval, license, and remote
  are ready.
- `Released`: immutable tag is pushed and tag CI succeeds.
- `Remote_Verified`: normal module resolution and external consumers pass.

Reports use the earliest state actually proved by evidence.
