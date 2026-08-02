# Current Delivery Work Graph

- Version: 8.0
- Status: Confirmed
- Last updated: 2026-08-02

The active work graph is
`.ai-platform/specs/008-react-admin-starter/tasks.md`.

| Task | State | Acceptance object |
|---|---|---|
| T035 | Completed | React-only Admin platform, typed application architecture, tests, and deterministic assets |
| T036 | Completed | React Admin behavior, accessibility, responsive quality, and browser acceptance |
| T037 | Completed | Copied-out React consumer, canonical docs, and final v0.2 release readiness |
| T028 | Completed | Lightweight product contract, competitor research, Profiles, and acceptance boundary |
| T029 | Completed | Database-free Core and optional component surfaces |
| T030 | Completed | Create-only CLI and API Profile |
| T031 | Completed | Optional Admin backend Profile |
| T032 | Completed | Superseded Admin UI F0 implementation and frontend module registry |
| T033 | Completed | Optional Governed Profile with retained Alpha 3 guarantees |
| T034 | Completed | Copied-out Profile acceptance, docs, compatibility, and final reviews |
| T024 | Completed | PostgreSQL control adapter and public task contract |
| T025 | Completed | PostgreSQL-native standard persistence |
| T026 | Completed | Alpha 3 framework and copied-out consumer acceptance |
| T027 | Completed | Immutable Alpha 3 release and remote verification |

## T028: Product Contract And Research

Status: Completed
Priority: P0
Dependencies: T027
Blocks: T029
Story / Requirement: PR-001, PR-002, and PR-003
Parallel: No
Conflicts with: every implementation task under feature 007

Goal: establish the canonical lightweight componentized Go framework contract
from owner input, current competitor evidence, and relevant Gin-Vue-Admin Issue
themes.

Allowed files: canonical governance documents, feature 007 artifacts, T028
evidence, and documentation-checker expectations for those artifacts.

Test targets: product-scope consistency, research source integrity, Profile and
component boundary completeness, documentation integrity, link validity, and
strict delivery-artifact validation.

Deliverables: confirmed constitution and product contract, competitor research,
feature spec, technical plan, work graph, checklist, analysis, execution packet,
and T028 review evidence.

Acceptance criteria: the artifacts agree that Core is database-free, Admin and
Governed capabilities are optional, generation is create-only, omitted
components are absent, and Alpha 3 is immutable. No Critical or High ambiguity
remains.

Definition of Done: documentation and strict artifact checks pass, the research
does not present historical Issues as current defects, and review finds no
unresolved P0 through P2 issue.

Validation commands:
- `./scripts/check-docs.sh`
- `./scripts/check-doc-links.sh`
- `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 007-component-framework-refoundation --task-id T028 --strict`
- `git diff --check`

TDD plan: documentation-only exception; strict artifact, documentation, and
link validation replace behavior TDD.

Packet path: `.ai-platform/specs/007-component-framework-refoundation/packets/T028.yaml`

Evidence required: `.ai-platform/evidence/T028/summary.md`, `diff.patch`,
`test-results.md`, and `review.md`.

Completed component-framework, PostgreSQL, task, release, and onboarding work
remains available as historical governance under features 002 through 007. The immutable
`v0.1.0-alpha.3` release remains accepted and remotely consumable while feature
008 defines the active React-only Admin delivery contract.
