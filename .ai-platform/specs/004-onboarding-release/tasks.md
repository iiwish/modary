# Modary Onboarding And Alpha Publication Work Graph

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-08-01
- Approval source: owner request to complete onboarding and publish v0.1.0-alpha.1

## T021: Onboarding Readiness Candidate

Status: Completed
Priority: P0
Depends on: T020
Blocks: T022
Story / Requirement: FR-001 through FR-005; NFR-001 through NFR-006
Parallel: No
Conflicts with: T022 and all active release tasks

Goal: Produce one legally attributable, publicly navigable, executable, and
fully validated onboarding candidate without changing the F0 runtime contract.

Allowed files: Apache license and notices; `README.md`; `docs/**`; promoted
`examples/counter/**`; current `scripts/**`, `Makefile`, `.gitignore`, CI and
release metadata; 004 contracts; T021 evidence. Historical evidence is read-only.

Test targets: Documentation checks, path and Make fixtures, public example
verify/generate/test/build/copy-out, framework acceptance, CI, governance, and
candidate release checks available before a clean commit.

Deliverables: Apache-2.0 licensing, attribution, public Counter example, golden
path, troubleshooting, updated navigation, deterministic validation, and T021
evidence.

Acceptance criteria: Public docs contain no retired example path; the public
example remains the independent conformance module; no P0-P2 review finding
remains; complete local gates pass.

Definition of Done: `make acceptance`, `make ci`, focused onboarding tests,
strict T021 artifact validation, and `git diff --check` pass after the final
implementation change.

Validation commands:
- `make acceptance`
- `make ci`
- `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 004-onboarding-release --task-id T021 --strict`
- `git diff --check`

TDD plan: RED stale-path and missing-artifact fixtures; GREEN path promotion,
docs, license, and checks; REFACTOR only after the public example gates pass.

Packet path: `.ai-platform/specs/004-onboarding-release/packets/T021.yaml`

Evidence required: RED/GREEN results, changed files, onboarding journey review,
license and attribution review, full validation, residual risk.

## T022: Publish v0.1.0-alpha.1

Status: In_Progress
Priority: P0
Depends on: T021
Blocks: None
Story / Requirement: FR-006, FR-007; NFR-002, NFR-004, NFR-005
Parallel: No
Conflicts with: All active release tasks

Goal: Publish the exact accepted candidate through the canonical public GitHub
repository and prove normal tagged Go module consumption.

Allowed files: Release metadata and T022 evidence before the release commit;
Git remote configuration, commits, annotated tag, GitHub repository settings,
tag push, and GitHub prerelease after validation.

Test targets: Clean candidate/tag preflight, GitHub tag CI, Go module
resolution, and copied-out remote consumer conformance.

Deliverables: Canonical public origin, security channel, release commit,
annotated tag, successful tag CI, remote consumer result, GitHub prerelease,
and truthful final report/evidence.

Acceptance criteria: Every public release object identifies the same version
and commit, and normal consumers resolve the module without local replacement.

Definition of Done: Candidate and tag preflight pass, tag CI succeeds,
`make remote-consumer VERSION=v0.1.0-alpha.1` passes, GitHub prerelease exists,
and strict T022 evidence validation passes.

Validation commands:
- `make release-readiness VERSION=v0.1.0-alpha.1`
- `make release-preflight VERSION=v0.1.0-alpha.1 RELEASE_MODE=tag`
- `gh run list --workflow ci.yml --limit 10`
- `go list -m github.com/iiwish/modary@v0.1.0-alpha.1`
- `make remote-consumer VERSION=v0.1.0-alpha.1`
- `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 004-onboarding-release --task-id T022 --strict`

TDD plan: Release workflow exception; every external transition is validated
before the next irreversible action and no failed gate is bypassed.

Packet path: `.ai-platform/specs/004-onboarding-release/packets/T022.yaml`

Evidence required: Candidate commit, remote, tag object, CI run, module
resolution, remote consumer, GitHub release URL, claim review, residual risk.
