# T022 Alpha 1 Publication Evidence

- Status: Superseded
- Date: 2026-08-01
- Packet: `.ai-platform/specs/004-onboarding-release/packets/T022.yaml`

## External Objects

- Canonical remote: `https://github.com/iiwish/modary`
- Candidate and tag commit: `f57e3adda9a5f0e7335e821ef0b69eaf75c3548b`
- Annotated tag object: `150d237ad95c563c0c5acd47ce55ba7190954fcf`
- Tag: `v0.1.0-alpha.1`
- Go proxy: resolved the tag to the candidate commit
- GitHub Actions: `https://github.com/iiwish/modary/actions/runs/30688318337`
- GitHub release URL: none; publication stopped before release creation

## Result

Candidate preflight, tag preflight, Linux quality, Darwin arm64 native checks,
and Go module resolution passed. The release job and direct remote-consumer gate
failed because two example tests asserted the source-checkout replacement after
the gate intentionally removed it from the copied application.

The tag had already been cached by the public Go proxy. It remains immutable and
is recorded as rejected. T023 owns the narrow repair and subsequent version.

## Residual Risk

The rejected tag is publicly resolvable even though no GitHub release was
created. Current public documentation and release notes must direct consumers
to the supported subsequent prerelease.
