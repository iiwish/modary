# Modary Alpha Release Readiness Plan

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-07-31

## Decisions

1. Target `v0.1.0-alpha.1`; do not claim stable compatibility.
2. Treat a signed or annotated source tag as the primary Go-library release
   artifact. Consumer applications own binaries, containers, and UI assets.
3. Keep the accepted F0 contract and ADRs authoritative. New documentation
   provides learning paths and operational guidance without duplicating the
   entire contract.
4. Use a documentation index with audience and task navigation. Keep README as
   a concise orientation and point detailed readers to `docs/index.md`.
5. Keep local checkout conformance and remote version conformance separate.
   Normal acceptance remains network-independent; a release ref adds the
   networked remote-consumer gate.
6. Implement release preflight as fail-closed POSIX shell with Go tests that
   construct isolated Git repositories and fake tool boundaries.
7. Require a clean candidate, canonical module path, selected license, origin,
   exact release tag, tidy modules, accepted F0, and complete gates. A dry
   readiness inspection may report missing owner decisions without claiming a
   release.
8. Preserve the external consumer's local `replace` only as a source-checkout
   fixture. The remote gate copies it out and removes the replacement before
   normal module resolution.
9. Add tag-triggered CI release preflight. Do not add GitHub release publication
   permissions or automatic tag creation.
10. Record license and repository visibility as owner decisions. Apache-2.0 is
    a candidate for an open ecosystem, but no license text is installed until
    the owner explicitly selects it.

## Documentation Architecture

- `docs/index.md`: audience map and canonical navigation.
- `docs/getting-started/`: installation, quickstart, project layout.
- `docs/concepts/`: framework boundary, modules/capabilities, governed Actions.
- `docs/how-to/`: add modules and Actions, expose surfaces, test consumers.
- `docs/reference/`: package map, support matrix, manifest and generated files.
- `docs/operations/`: deployment, security, SQLite operations.
- `docs/releases/`: versioning, release process, upgrade policy.
- Root governance: `CHANGELOG.md`, `CONTRIBUTING.md`, `SECURITY.md`.
- Existing `framework-f0.md`, ADRs, limitations, and acceptance report remain
  deep canonical references.

## Validation Strategy

- Documentation: required-file, local-link, stale-claim, and representative
  command checks.
- Automation: RED/GREEN fixture tests for invalid versions, missing license,
  incorrect module path, missing origin, dirty state, mismatched tags, and
  remote-consumer replacement removal.
- Framework: `make acceptance`, then `make ci` for closure.
- Governance: strict artifact validation for T017 through T020.

## Constitution Check

- Consumer ownership and framework neutrality: satisfied.
- Evidence over assertion: release claims are gated by commands and reports.
- Fail-closed security: preflight rejects missing or ambiguous metadata.
- Node-free headless workflow: all new checks use Go, Git, Make, and POSIX shell.
- Preserve user work: no remote, tag, visibility, or license choice is created
  implicitly.
