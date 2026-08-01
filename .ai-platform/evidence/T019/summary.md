# T019 Evidence Summary

- Status: Completed
- Date: 2026-07-31
- Packet: `.ai-platform/specs/003-release-readiness/packets/T019.yaml`

## Changed Files

- `scripts/release-preflight.sh` validates semantic prerelease syntax, canonical
  module path and origin, owner-selected license, private security channel,
  changelog state, accepted F0 evidence, clean committed state, and candidate or
  annotated exact-tag relationships.
- `scripts/remote-consumer.sh` copies the independent consumer to `/tmp`, removes
  the local replacement, pins the requested module version, confirms normal
  resolution, and runs verify, generate/check, test, build, and version.
- `Makefile` separates metadata preflight, full release readiness, and remote
  consumer targets.
- `.github/workflows/ci.yml` adds a read-only tag job after Ubuntu and native
  Darwin gates, with full tag history and remote consumer verification.
- Focused release/remote/Make/CI fixture tests cover success and failure paths.
- External consumer and release documentation describe local and remote
  conformance as separate claims.

## Security Review

- Scripts reject symlinked or missing canonical inputs and ambiguous license
  files.
- Preflight refuses dirty or uncommitted release state, a noncanonical origin,
  a moved tag, a lightweight tag in tag mode, and an unfinalized changelog.
- CI retains `contents: read`; no script creates, moves, pushes, publishes, or
  deletes a tag or release.
- Remote conformance disables Go work files, clears ambient `GOFLAGS`, uses the
  installed local toolchain, and rejects any resolved replacement.

## Current Real State

Running the real candidate preflight stops with:

```text
release requires one non-empty owner-selected redistribution license
```

This is the expected owner gate. The test fixture proves the same preflight
passes with a concrete license, security channel, origin, clean commit, and
correct candidate/tag state.

## Residual Risk

- A real remote resolution cannot pass before the tag is published.
- A private repository would require an explicitly configured authenticated Go
  module environment; the current tag job is immediately suitable for a public
  GitHub module.
- Repository host visibility and final license remain owner decisions.
