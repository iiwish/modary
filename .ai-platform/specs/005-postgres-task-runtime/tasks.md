# PostgreSQL Control Store And Task Runtime Work Graph

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-08-02
- Approval source: owner request for autonomous PostgreSQL-first rewrite

## T024: PostgreSQL And Task Contracts

Status: Completed
Priority: P0
Depends on: T023
Blocks: T025
Parallel: No

Goal: Implement the canonical PostgreSQL backend and public transactional task
contract without retaining SQLite compatibility.

Allowed files: `task/**`, `module/**`, `appkit/**`, `internal/databasecontrol/**`,
`internal/moduleassembly/**`, `adapters/postgres/**`, root Go module files, 005
governance artifacts, and focused tests.

Validation commands:
- `go test ./task ./module ./appkit ./internal/databasecontrol ./adapters/postgres`
- `go test -race ./task ./adapters/postgres`
- `git diff --check`

Definition of Done: PostgreSQL connects securely, schemas and River migrations
initialize, governed transactions preserve their contract, task enqueue is
atomic with domain writes, runners retry and recover, and focused review has no
P0 through P2 finding.

Packet path: `.ai-platform/specs/005-postgres-task-runtime/packets/T024.yaml`

## T025: PostgreSQL Standard Persistence

Status: Completed
Priority: P0
Depends on: T024
Blocks: T026
Parallel: No

Goal: Port Module migration registry, Action persistence, Identity, RBAC, and
SQL Audit to PostgreSQL-native schemas and queries.

Allowed files: `internal/databasecontrol/**`, `adapters/postgres/**`,
`adapters/localidentity/**`, `adapters/rbac/**`, `adapters/sqlaudit/**`, focused
tests, 005 governance, and T025 evidence.

Validation commands:
- `go test ./internal/databasecontrol ./adapters/postgres ./adapters/localidentity ./adapters/rbac ./adapters/sqlaudit`
- `go test -race ./adapters/postgres ./adapters/localidentity ./adapters/rbac ./adapters/sqlaudit`
- `git diff --check`

Definition of Done: all standard durable services use PostgreSQL and preserve
validation, authorization, transaction, corruption, and secret-safety contracts.

Packet path: `.ai-platform/specs/005-postgres-task-runtime/packets/T025.yaml`

## T026: Consumer, Deletion, And Acceptance

Status: Completed
Priority: P0
Depends on: T025
Blocks: Modary F0 acceptance
Story / Requirement: FR-009, FR-010, SC-001, SC-002, SC-003, SC-004, and SC-005
Parallel: No
Conflicts with: None

Goal: Port Counter and repository gates, delete SQLite completely, update
canonical documentation, and complete Modary acceptance.

Allowed files: all active Modary files except immutable historical evidence;
005 governance and T026 evidence.

Test targets: framework packages, PostgreSQL adapters, copied-out Counter with
`GOWORK=off`, task restart consumption, documentation, neutrality, builds,
race detection, repetition, fuzzing, and source stability.

Deliverables: PostgreSQL-only framework and Counter consumer, public task
runtime acceptance coverage, canonical documentation, release report, T026
packet, test results, diff manifest, and two independent final review records.

Acceptance criteria: the validation commands pass against real PostgreSQL and
the current final reviews contain no unresolved P0, P1, or P2 finding.

Validation commands:
- `make acceptance`
- `make ci`
- `go test -race ./...`
- strict T026 artifact validation
- `git diff --check`

Definition of Done: no active SQLite surface remains, Counter is independently
consumable on PostgreSQL and proves transactionally enqueued work survives an
application restart, all checks pass against real PostgreSQL, and two current
review passes contain no unresolved P0 through P2 finding.

TDD plan: RED records the active SQLite and consumer-boundary failures; GREEN
requires PostgreSQL-only Counter and framework acceptance to pass; REFACTOR
requires the complete CI, race, stability, and strict artifact gates to pass.

Packet path: `.ai-platform/specs/005-postgres-task-runtime/packets/T026.yaml`

Evidence required: `.ai-platform/evidence/T026/summary.md`, `diff.patch`,
`test-results.md`, and the current final review records.
