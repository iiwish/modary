# Current Product Contract

- Version: 4.0
- Status: Confirmed
- Last updated: 2026-08-01
- Source: explicit owner request to complete onboarding and publish v0.1.0-alpha.1

Modary is a Go-first modular application kernel, SDK, and build tool. Independent
applications compose public Module packages and execute typed Actions through a
single governed Runtime across human and Agent channels. Consumer applications
own their domain, branding, migrations, policy, bootstrap data, UI, and release
artifacts.

The accepted F0 product and implementation contract is
`.ai-platform/specs/002-framework-decoupling/spec.md`. The current Alpha
distribution-readiness contract is
`.ai-platform/specs/003-release-readiness/spec.md`. The public onboarding and
Alpha publication contract is
`.ai-platform/specs/004-onboarding-release/spec.md`.

Technical F0 acceptance proves local source consumption. Release Readiness adds
versioning, security, operations, adoption, upgrade, and release documentation;
fail-closed candidate/tag preflight; and a normal Go Module remote-consumer
gate. The target is `v0.1.0-alpha.1`, not a stable-v1 compatibility claim.

Modary-owned source and documentation are licensed under Apache-2.0. Embedded
third-party licenses and attributions remain preserved and are aggregated by
the repository notice. A published version additionally requires the canonical
public remote, private security channel, immutable annotated tag, successful
tag CI, and normal Go Module remote-consumer evidence.
