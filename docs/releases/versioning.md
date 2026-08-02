# Versioning And Compatibility

## Release Lines

Modary uses semantic version tags and the Go module path
`github.com/iiwish/modary`.

| Line | State | Contract |
|---|---|---|
| `v0.1.0-alpha.3` | Published and immutable | Historical Governed-first PostgreSQL/River release |
| `v0.2.0-alpha.1` | Current source target, not released | Componentized Core with API/Admin/Governed Profiles |

A prerelease is suitable for evaluation and design-partner development, but
public APIs, generated structure, and component boundaries may change before
v1. A future incompatible v2 requires the standard `/v2` module-path suffix.

## Immutability

Every published tag is immutable. A defect in a published prerelease is fixed
in a new version, never by moving the tag or rewriting a migration. Documentation
at a tag describes that exact source.

The immutable `v0.1.0-alpha.1` tag failed its remote onboarding gate and has no
supported GitHub release. `v0.1.0-alpha.2` is the historical embedded-storage
Alpha. `v0.1.0-alpha.3` is the supported published baseline until a later tag is
actually released.

## Pre-v1 Compatibility

Maintainers prefer additive changes, but a prerelease may change public Go APIs,
Starter structure, Profile selection, manifest fields, generated formats,
migrations, or defaults when correctness or product focus requires it.

Every intentional consumer-visible change requires:

1. a `CHANGELOG.md` entry;
2. upgrade instructions identifying source and data work;
3. copied-out consumer acceptance for affected Profiles;
4. explicit release notes and known limitations.

Exact-version pinning is required. Generated application source is not a hidden
compatibility surface: consumers own it after creation and upgrade it
deliberately.

## Deprecation

Before v1, a deprecation period is preferred but not guaranteed. When a safe
transition exists, add the replacement, mark the old API deprecated, cover both
paths externally, document removal timing, and remove only with explicit upgrade
instructions. Security or correctness defects may require immediate removal.

## Go Toolchain

`go.mod` defines the minimum Go language and toolchain baseline. Builds use
`GOTOOLCHAIN=local`; maintainers and consumers install a compatible toolchain
rather than relying on ambient automatic download. Raising the baseline is a
consumer-visible release change.

## Generated Artifacts

Starter source is created once. Admin frontend assets are deterministic outputs
of consumer-owned React source and must pass the pinned asset parity check.

Optional `projecttool` graph, Action catalog, and TypeScript files are
deterministic consumer-owned outputs. Regenerate them with the same pinned
Modary version and review all diffs. Cross-version merging of generated sets is
unsupported.

## Database Migrations

Published migrations are forward-only. A correction is a new migration.
Downgrade is an application-level restore from a verified pre-upgrade backup,
not a reverse migration. Profile changes that add or remove infrastructure need
an explicit data and deployment plan.

## Support

No source line is released until its immutable tag and release notes exist.
Alpha support is best-effort and optimized for current design partners. Stable
v1 requires an explicit compatibility window, maintained security contact,
tested upgrades, and evidence from multiple independent consumers.
