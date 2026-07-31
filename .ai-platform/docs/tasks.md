# Current Delivery Work Graph

- Version: 2.0
- Status: Confirmed
- Last updated: 2026-07-31

The authoritative framework F0 work graph is
`.ai-platform/specs/002-framework-decoupling/tasks.md`.

| Task | State | Acceptance object |
|---|---|---|
| T010 | Completed | Contract and checksum-verified consumer asset preservation |
| T011 | Completed | Public Kernel and lifecycle |
| T012 | Completed | Public AppKit, commands, and transports |
| T013 | Completed | Pure consumer project tooling and independent review |
| T014 | Completed | Neutral official Adapters |
| T015 | Completed | Copied-out consumer conformance and active-tree neutrality |
| T016 | Completed | Full gates, two-pass review, and truthful F0 acceptance |

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
