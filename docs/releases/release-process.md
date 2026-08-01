# Release Process

This process publishes the Modary Go source module and documentation. It does
not publish a consumer application, binary, container, UI, or domain starter.

## Roles

- The owner selects repository visibility, redistribution license, canonical
  remote, release version, and final publish approval.
- The maintainer prepares the candidate, runs validation, writes release notes,
  and records evidence.
- The reviewer checks compatibility, security claims, consumer conformance, and
  the exact candidate commit.

One person may hold several roles, but owner decisions must remain explicit.

## Prerequisites

Before release:

1. The F0 technical acceptance report is `Accepted`.
2. `CHANGELOG.md` contains the intended release entry and no unresolved item is
   silently omitted.
3. An owner-selected redistribution license is present as `LICENSE` or
   `LICENSE.txt`.
4. `origin` is the canonical repository and matches the module ownership plan.
5. The candidate is on a clean worktree and all intended changes are committed.
6. The version is a valid semantic prerelease such as `v0.1.0-alpha.2`.
7. Documentation and examples describe the same candidate.

## Candidate Validation

Run from the repository root:

```bash
make bootstrap
make acceptance
make ci
make release-readiness VERSION=v0.1.0-alpha.2
```

The final command must verify the module path, version, license, origin, clean
source, tag relationship, dependency metadata, canonical docs, and complete
gates. On an untagged candidate it may be run in candidate mode only when the
script documents that no release is claimed; tag CI requires an exact tag.

Record the commit ID and produce release notes from the matching changelog
entry. Do not move or reuse a published tag.

## Tag And Publish

After explicit owner approval:

```bash
git tag -a v0.1.0-alpha.2 -m "Modary v0.1.0-alpha.2"
git push origin v0.1.0-alpha.2
```

Wait for tag CI. Then verify normal remote module resolution:

```bash
make remote-consumer VERSION=v0.1.0-alpha.2
```

The remote gate copies the conformance consumer outside the repository, removes
the source-checkout replacement, downloads the requested version with
`GOWORK=off`, and runs project verification, generated drift, tests, build, and
the version command.

Create the repository-host release from the same tag. Release notes must include
scope, compatibility status, supported platform profile, upgrade requirements,
known limitations, security contact, and remote-consumer result.

## Post-Release Checks

- `go list -m github.com/iiwish/modary@v0.1.0-alpha.2` resolves the expected
  module and version.
- A new temporary consumer builds with no `replace` directive or Go work file.
- Release links, changelog, license, and source tag are visible.
- The tag commit matches successful CI and review evidence.
- The first design-partner application pins the exact version before implementation.

## Failure And Rollback

Never overwrite a published tag. If publication fails before the tag is pushed,
repair the candidate and create the tag only after validation. If an immutable
published tag is defective, document it, publish the next prerelease or patch,
and advise consumers to upgrade. If a migration is unsafe, stop rollout and
restore consumer data from the verified pre-upgrade backup; do not rewrite the
published migration.

## Release States

- `Technical_Accepted`: local framework behavior and conformance are accepted.
- `Engineering_Ready`: docs, policy, automation, and review are complete.
- `Distribution_Ready`: owner license and canonical remote are present.
- `Released`: the approved tag is pushed and tag CI succeeds.
- `Remote_Verified`: normal Go module resolution and copied-out consumer pass.

Reports must use the earliest state actually proved by evidence.
