# Design Partner Validation Specification

- Version: 0.1
- Status: Ready_For_User_Review
- Date: 2026-08-10
- Source: owner request to close the current Modary delivery state and prepare the next milestone for review
- Prerequisite: `v0.3.0-alpha.1` is released and remote verified
- Execution authorization: Not granted

## Purpose

This milestone validates whether an independent product team can adopt Modary
to deliver product behavior without framework-checkout coupling or premature
framework expansion. It converts adoption experience into evidence for a
focused v0.3 hardening release or a separately approved v0.4 contract.

## Target Users

- A Go product team adopting Modary for a modular business backend or
  administrative system.
- A Modary maintainer deciding which adoption friction belongs in documentation,
  hardening, a reusable component, or consumer-owned product code.

## User Stories

- US-001: A product team can create or adapt an independent application using an
  exact Modary release without a local workspace or source replacement.
- US-002: The team can replace the generated example with one product-owned
  vertical slice while preserving explicit Module, capability, migration, route,
  authorization, and frontend ownership boundaries.
- US-003: An operator can migrate, start, observe, stop, restart, and diagnose the
  selected application using the documented process and deployment contracts.
- US-004: A Modary maintainer can reproduce and classify adoption friction before
  approving framework changes or a new release scope.

## Functional Requirements

- FR-001: Acceptance uses an application in an independent repository with
  `GOWORK=off`, exact Modary module versions, and no local `replace` directive.
- FR-002: The application selects the smallest Profile that satisfies its actual
  product job. OIDC, OpenTelemetry, tasks, audit, and governed operations remain
  absent unless the product scenario requires them.
- FR-003: The application replaces the structural example with at least one
  product-owned Module covering persistence or state, service behavior, an
  external interface, authorization where required, and restart verification.
- FR-004: An Admin consumer owns its frontend module, routes, navigation,
  terminology, API integration, loading, empty, error, permission, and responsive
  states. A headless consumer proves the equivalent complete API workflow.
- FR-005: Product domain models, workflows, policies, UI, deployment decisions,
  and release cadence remain outside the Modary repository.
- FR-006: The validation records each material friction item with reproduction,
  severity, affected workflow, workaround, and one classification: documentation
  gap, defect, missing extension point, or feature hypothesis.
- FR-007: A friction item does not authorize a Modary source change. Defects and
  feature hypotheses require separately approved scope and validation evidence.
- FR-008: The selected consumer runs its relevant tests, production build,
  migration path, liveness/readiness checks, graceful shutdown, and restart path.
- FR-009: The milestone ends with an evidence-based recommendation to keep the
  current release, prepare a bounded v0.3 hardening release, or clarify a v0.4
  product contract.

## Non-Functional Requirements

- NFR-001 Security: validation uses synthetic or approved non-production data,
  stores no credentials in evidence, and preserves the documented local Identity,
  OIDC, CSRF, RBAC, token, and deployment boundaries.
- NFR-002 Reliability: the product workflow remains correct across process restart
  and dependency failure relevant to the selected Profile.
- NFR-003 Independence: acceptance commands run outside the Modary checkout and
  do not depend on uncommitted framework source, a Go work file, or private local
  build artifacts.
- NFR-004 Replaceability: unselected components remain absent from source,
  configuration, routes, migrations, lifecycle, and dependency graphs.
- NFR-005 Quality: evidence includes commands, results, changed consumer files,
  unresolved findings, and explicit residual risk. No unresolved P0 or P1
  adoption blocker may remain at milestone acceptance.
- NFR-006 Privacy: logs, screenshots, traces, metrics, database fixtures, and
  evidence contain no production personal data or secrets.

## Scope

- One independent design-partner application and one complete product workflow.
- Bootstrap, module replacement, selected authentication and authorization,
  persistence, frontend or API behavior, build, deployment, restart, and
  operational validation relevant to that application.
- A structured adoption-friction report and a release-scope recommendation.
- Narrow documentation or defect proposals derived from observed evidence.

## Non-Goals

- Implementing a new Modary feature, adapter, Profile, component marketplace,
  low-code surface, database backend, hosted platform, or Kubernetes operator.
- Treating every F0 known limitation as backlog.
- Copying design-partner product behavior into Modary packages or examples.
- Automatically patching generated consumer source.
- Claiming Beta, stable-v1 compatibility, or broader platform support.
- Publishing a release as part of this requirements-review artifact.

## Edge Cases

- If no independent design-partner repository and owner are available, the
  milestone remains blocked; another copied-out fixture cannot satisfy FR-001.
- If the selected product does not need Admin, OIDC, OpenTelemetry, tasks, audit,
  or governed Actions, validation must not add them merely to increase coverage.
- If adoption exposes a security, data-loss, authentication, migration, or release
  risk, normal validation stops and a separate governed remediation contract is
  required.
- If friction belongs to consumer domain design, the report records the boundary
  rather than adding a generic framework abstraction.
- If an exact release cannot support the workflow without a local replacement,
  the failure is recorded as an adoption blocker rather than hidden by source-mode
  validation.

## Constraints And Assumptions

- `v0.3.0-alpha.1` is the immutable baseline for the first acceptance pass.
- The design-partner repository controls its own branch, data, deployment,
  reviewers, and acceptance decision.
- Modary may be imported by the consumer; Modary never imports the consumer.
- Generated source is consumer-owned and is changed through reviewed application
  edits, not regeneration over handwritten code.
- Planning, execution, cross-repository access, and any release action require
  explicit owner approval after this specification is confirmed.

## Data And Integration Needs

- An owner-approved independent repository and a named product workflow.
- A disposable or approved PostgreSQL 17 environment when the selected Profile
  requires persistence.
- An approved identity provider and OTLP endpoint only when those components are
  selected.
- A reproducible command log, sanitized evidence, and a friction register owned
  by the validation milestone.

## Success Criteria

- SC-001: the independent application builds and tests with `GOWORK=off`, exact
  released modules, and no local replacement.
- SC-002: one product-owned workflow completes from its external interface through
  persistence or state, authorization where applicable, and restart.
- SC-003: the selected deployment executes migration, liveness, readiness,
  graceful shutdown, and restart checks without framework-checkout dependencies.
- SC-004: every material friction item has reproduction, severity, workaround,
  classification, and an owner decision; no unresolved P0 or P1 blocker remains.
- SC-005: unselected component absence is demonstrated for the design-partner
  composition.
- SC-006: the final recommendation names one bounded next action: no framework
  change, v0.3 hardening, or a new product contract requiring clarification.

## Acceptance Criteria

- The user approves the selected repository, workflow, timebox, and evidence
  boundary before planning begins.
- Reviewer-visible evidence satisfies SC-001 through SC-006.
- Consumer maintainers accept the product workflow and operational result.
- Modary review confirms the consumer boundary and records all residual risks.
- Any proposed framework change remains outside this milestone until separately
  approved.

## Clarifications

- The 2026-08-10 status-closeout request authorizes this review artifact, not a
  design-partner implementation, task graph, framework change, or release.
- The current F0 known limitations remain contract boundaries rather than an
  automatic feature backlog.
- Rulary is a suitable candidate described by the existing adoption guide, but
  the owner may select another independent application before planning.

## Planning Inputs For Owner Review

- Confirm the independent design-partner repository and its maintainer.
- Confirm the product workflow used for the vertical slice.
- Confirm the timebox and whether cross-repository edits are authorized.
- Confirm whether Rulary is the first design partner.

## User Review Gate

- Approval: Pending
- Planning: Not started
- Work graph: Not created
- Execution: Not authorized
