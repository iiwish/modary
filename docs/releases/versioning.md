# Versioning And Compatibility

## Release Line

Modary uses semantic version tags. `v0.1.0-alpha.3` is the PostgreSQL and
durable-task release line. A prerelease tag means the framework is usable for
evaluation and design-partner development, but public APIs and generated
formats may still change before `v1.0.0`.

The Go module path remains `github.com/iiwish/modary` for every `v0` and `v1`
release. A future incompatible `v2` would require the standard `/v2` module-path
suffix and an explicit migration guide.

## Compatibility Contract

Within one published prerelease:

- the tag is immutable;
- checked-in documentation describes that exact source;
- the external-consumer gate defines the supported public composition path;
- generated files are outputs, not handwritten compatibility surfaces;
- database migrations are forward-only and are never silently rewritten after
publication.

The immutable `v0.1.0-alpha.1` tag failed its remote onboarding gate and has no
supported GitHub release. Consumers must not select it.

`v0.1.0-alpha.2` is the historical embedded-storage Alpha and does not contain
the PostgreSQL or River contracts. New development uses `v0.1.0-alpha.3`.

Across Alpha releases, maintainers prefer additive changes, but may change a
public Go API, manifest field, generated format, migration contract, or default
when framework correctness requires it. Every intentional consumer-visible
change must appear in `CHANGELOG.md` and the release's upgrade notes.

## Deprecation

Before v1, a deprecation period is preferred but not guaranteed. When a safe
transition is possible, maintainers should:

1. add the replacement before removing the old surface;
2. mark the Go symbol or document field as deprecated;
3. keep external-consumer coverage for both paths during the transition;
4. document the first deprecated and planned removal versions;
5. remove it only in a release with explicit upgrade instructions.

Security or correctness defects may require immediate removal. Release notes
must explain the affected boundary and migration.

## Go Toolchain

`go.mod` is authoritative for the minimum Go language and toolchain baseline.
A release may raise that baseline before v1. Such a change is consumer-visible
and requires changelog and upgrade entries. Project builds use
`GOTOOLCHAIN=local`; maintainers and consumers install a compatible toolchain
rather than relying on ambient automatic download.

## Generated Artifacts

Module graphs, Action catalogs, and TypeScript contracts are deterministic
consumer-owned outputs. Consumers regenerate them with the project tool pinned
to the same Modary version as the application. A release that changes an output
format must document whether regeneration alone is sufficient or source changes
are required. Cross-version merge of generated sets is unsupported.

## Database Migrations

Published framework and consumer migrations are forward-only. Never edit an
already distributed migration in place. A correction is a new migration. Back
back up the PostgreSQL control database and verify restore before upgrading. Downgrade
is application-level restore from a pre-upgrade backup, not reverse migration.

## Support

No release line is supported until its tag and release notes say so. Alpha
support is best-effort and optimized for current design partners. Stable v1
requires an explicit compatibility window, maintained security contact, tested
upgrade path, and evidence from more than one independent consumer.
