# T023 Alpha 2 Publication Evidence

- Status: Completed
- Date: 2026-08-01
- Packet: `.ai-platform/specs/004-onboarding-release/packets/T023.yaml`

## Release Identity

- Canonical remote: `https://github.com/iiwish/modary`
- Candidate and tag commit: `a4700f1c7ef53fe058a50fd43d65b906c3be89c4`
- Annotated tag object: `43ce902582c8f89238d1a973d95b4be7efeede26`
- Supported tag: `v0.1.0-alpha.2`
- Rejected immutable tag: `v0.1.0-alpha.1` at
  `f57e3adda9a5f0e7335e821ef0b69eaf75c3548b`
- Supported release: `https://github.com/iiwish/modary/releases/tag/v0.1.0-alpha.2`
- Rejected release notice: `https://github.com/iiwish/modary/releases/tag/v0.1.0-alpha.1`

## Repair

The remote gate identifies its temporary application through the existing
copied-out environment contract. Only recursive copy and development-replacement
assertions skip. Project verify, generated checks, all application behavior
tests, build, version, Node-family shims, exact module identity, and absence of a
replacement remain mandatory.

## External Proof

- Main candidate CI: `https://github.com/iiwish/modary/actions/runs/30688748867`
- Tag CI: `https://github.com/iiwish/modary/actions/runs/30688981095`
- Public Go proxy resolved Alpha 2 to the exact candidate commit.
- Local and hosted copied-out remote consumers passed without a replacement.
- Private vulnerability reporting remains enabled at the documented URL.

## Residual Risk

The release is pre-v1 Alpha. Its supported functional, platform, deployment,
security, and trust boundaries remain those in the release notes, support
matrix, security operations guide, and F0 known limitations. The rejected Alpha
1 tag remains publicly resolvable and is explicitly unsupported.
