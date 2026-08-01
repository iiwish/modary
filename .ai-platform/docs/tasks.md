# Current Delivery Work Graph

- Version: 5.0
- Status: Confirmed
- Last updated: 2026-08-01

The accepted framework F0 work graph is
`.ai-platform/specs/002-framework-decoupling/tasks.md`. The current Alpha
release-readiness work graph is
`.ai-platform/specs/003-release-readiness/tasks.md`. Onboarding and publication
are governed by `.ai-platform/specs/004-onboarding-release/tasks.md`.

| Task | State | Acceptance object |
|---|---|---|
| T010 | Completed | Contract and checksum-verified consumer asset preservation |
| T011 | Completed | Public Kernel and lifecycle |
| T012 | Completed | Public AppKit, commands, and transports |
| T013 | Completed | Pure consumer project tooling and independent review |
| T014 | Completed | Neutral official Adapters |
| T015 | Completed | Copied-out consumer conformance and active-tree neutrality |
| T016 | Completed | Full gates, two-pass review, and truthful F0 acceptance |
| T017 | Completed | Release governance, versioning, security, and contribution policy |
| T018 | Completed | Indexed adoption, reference, operations, and upgrade documentation |
| T019 | Completed | Candidate/tag preflight, remote consumer, and read-only tag CI |
| T020 | Completed | Full Release Readiness validation, review, and truthful handoff |
| T021 | Completed | Public onboarding candidate, Apache licensing, and local gates |
| T022 | Superseded | Rejected immutable v0.1.0-alpha.1 attempt |
| T023 | In_Progress | Supported v0.1.0-alpha.2 and remote consumer evidence |

Task state is complete only when its required implementation, validation,
independent review, and evidence are present. The framework F0 is accepted only
when T016 records no unresolved P0, P1, or P2 finding.

## T016: Current Framework F0 Acceptance

Status: Completed
Priority: P0
Depends on: T015
Blocks: None
Story / Requirement: FR-001 through FR-013; NFR-001 through NFR-010
Parallel: No
Conflicts with: All active implementation tasks

Goal: Complete full framework gates, two independent review passes, repairs,
and truthful F0 acceptance evidence.

Allowed files: Repository-wide reviewed fixes, current docs, task graph,
packets, and `.ai-platform/evidence/T016/**`. Completed T010-T015 evidence is
read-only historical input.

Test targets: Framework and external consumer tests, vet, race, count-20, fuzz,
cross-build, generated drift, neutrality, API documentation, docs, and artifact
validation.

Deliverables: Accepted implementation, complete evidence, current release and
acceptance reports, and a closed framework work graph.

Acceptance criteria: Every required gate passes and two independent review
passes report no unresolved P0, P1, or P2 finding.

Definition of Done: `make acceptance`, `make ci`, all T010-T016 strict artifact
validators, and `git diff --check` pass after the last code change. The archived
prototype checksum is a one-time maintainer-local preservation audit recorded by
T010, not a consumer, CI, or framework release gate.

Validation commands: `make acceptance`; `make ci`;
`for task in T010 T011 T012 T013 T014 T015 T016; do python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 002-framework-decoupling --task-id "$task" --strict; done`;
`git diff --check`. T016 consumes the completed T010 preservation record as
historical evidence and does not read or revalidate any external project path.

TDD plan: RED each gate or review finding; GREEN targeted fixes with focused
regressions; REFACTOR only after green; rerun full acceptance after the last fix.

Packet path: `.ai-platform/specs/002-framework-decoupling/packets/T016.yaml`

Evidence required: Changed files, test results, review findings and repairs,
acceptance matrix, release boundary, and residual risks.

## T020: Current Alpha Release Readiness

Status: Completed
Priority: P0
Depends on: T018, T019
Blocks: None
Story / Requirement: 003 FR-001 through FR-010; NFR-001 through NFR-006
Parallel: No
Conflicts with: All active release-readiness tasks

Goal: Complete full validation and review, report exact readiness state, and
reduce owner and external blockers to explicit publish decisions.

Allowed files: Repository-wide reviewed fixes, 003 release-readiness contracts,
current reports, and `.ai-platform/evidence/T020/**`.

Test targets: Complete framework, consumer, documentation, release, CI,
governance, race, repetition, fuzz, neutrality, and source-stability gates.

Deliverables: Engineering-ready Alpha candidate materials, complete evidence,
readiness matrix, review, and owner-decision handoff.

Acceptance criteria: No unresolved P0-P2 engineering finding. Distribution and
remote verification remain unclaimed until license, remote, security channel,
tag, and network evidence exist.

Definition of Done: `make acceptance`, `make ci`, strict T017-T020 artifact
validation, and `git diff --check` pass after the final implementation change.

Validation commands: `make acceptance`; `make ci`;
`for task in T017 T018 T019 T020; do python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 003-release-readiness --task-id "$task" --strict; done`;
`git diff --check`.

TDD plan: RED every review or gate finding; GREEN focused repair; REFACTOR only
after green; rerun full closure gates.

Packet path: `.ai-platform/specs/003-release-readiness/packets/T020.yaml`

Evidence required: Changed files, full validation, spec and engineering review,
readiness matrix, owner blockers, and residual risk.

## T021: Current Onboarding Readiness Candidate

Status: Completed
Priority: P0
Depends on: T020
Blocks: T022
Story / Requirement: 004 FR-001 through FR-005; NFR-001 through NFR-006
Parallel: No
Conflicts with: T022 and all active release tasks

Goal: Produce one Apache-licensed, publicly navigable, executable, and fully
validated onboarding candidate without changing the F0 runtime contract.

Allowed files: License and attribution; public example promotion; current
README, user docs, scripts, Make, CI, governance, release metadata, tests, and
T021 evidence. Historical evidence is read-only.

Test targets: Documentation and stale-path checks, public example copied-out
conformance, acceptance, CI, neutrality, release scripts, and governance.

Deliverables: Apache-2.0 licensing, root attribution, public Counter example,
golden path, troubleshooting, deterministic checks, and T021 evidence.

Acceptance criteria: Public docs contain no retired example path; the public
example remains the independent conformance module; no P0-P2 finding remains;
all local gates pass.

Definition of Done: `make acceptance`, `make ci`, strict T021 artifact
validation, and `git diff --check` pass after the final implementation change.

Validation commands: `make acceptance`; `make ci`;
`python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 004-onboarding-release --task-id T021 --strict`;
`git diff --check`.

TDD plan: RED stale path, public example, and license checks; GREEN onboarding
implementation and focused tests; REFACTOR only after complete gates remain green.

Packet path: `.ai-platform/specs/004-onboarding-release/packets/T021.yaml`

Evidence required: RED/GREEN results, changed files, onboarding and attribution
review, complete validation, and residual risk.

## T022: Current v0.1.0-alpha.1 Publication

Status: Superseded
Priority: P0
Depends on: T021
Blocks: None
Story / Requirement: 004 FR-006, FR-007; NFR-002, NFR-004, NFR-005
Parallel: No
Conflicts with: All active release tasks

Goal: Publish the exact accepted candidate through the canonical public GitHub
repository and prove normal tagged Go module consumption.

Allowed files: Current release metadata and T022 evidence before the release
commit; Git remote configuration, commits, annotated tag, repository security
settings, tag push, and GitHub prerelease after validation.

Test targets: Clean candidate/tag preflight, tag CI, Go module resolution, and
copied-out remote consumer conformance.

Deliverables: Canonical public origin, private reporting channel, release
commit, annotated tag, successful tag CI, normal remote consumer result, GitHub
prerelease, and truthful final evidence.

Acceptance criteria: Every public release object identifies the same version
and candidate commit, and consumers resolve Modary without a local replacement.

Definition of Done: Candidate and tag preflight pass, tag CI succeeds,
`make remote-consumer VERSION=v0.1.0-alpha.1` passes, the GitHub prerelease
exists, and strict T022 artifact validation passes.

Validation commands: `make release-readiness VERSION=v0.1.0-alpha.1`;
`make release-preflight VERSION=v0.1.0-alpha.1 RELEASE_MODE=tag`;
`gh run list --workflow ci.yml --limit 10`;
`go list -m github.com/iiwish/modary@v0.1.0-alpha.1`;
`make remote-consumer VERSION=v0.1.0-alpha.1`;
strict T022 artifact validation.

TDD plan: Release workflow exception; validate each external state before the
next irreversible transition and never move a published tag.

Packet path: `.ai-platform/specs/004-onboarding-release/packets/T022.yaml`

Evidence required: Candidate commit, canonical remote, tag object, tag CI,
module resolution, remote consumer, release URL, claim review, residual risk.

## T023: Current v0.1.0-alpha.2 Publication

Status: In_Progress
Priority: P0
Depends on: T022
Blocks: None
Story / Requirement: 004 FR-006, FR-007; NFR-002, NFR-004, NFR-005
Parallel: No
Conflicts with: All active release tasks

Goal: Preserve the rejected immutable Alpha 1 tag, repair copied-out remote
validation, and publish one supported Alpha 2 release with complete evidence.

Allowed files: Remote-consumer script and tests; current public version,
security, changelog, release, onboarding, governance, and CI metadata; T022 and
T023 evidence; Git and GitHub release objects.

Test targets: Remote-consumer regression, complete gates, preflight, tag CI,
normal Go proxy resolution, copied-out consumer, and strict evidence validation.

Deliverables: Immutable rejection record, repaired candidate, successful Alpha
2 tag CI, remote consumer result, GitHub prerelease, and truthful final report.

Acceptance criteria: The Alpha 1 tag is not moved; the supported Alpha 2 tag,
CI, module resolution, release, and evidence identify one exact commit.

Definition of Done: `make remote-consumer VERSION=v0.1.0-alpha.2` and all local,
tag, hosted, release, and strict T023 gates pass.

Packet path: `.ai-platform/specs/004-onboarding-release/packets/T023.yaml`

Evidence required: Rejection record, candidate and tag identity, CI, normal
module resolution, copied-out consumption, release URLs, review, residual risk.
