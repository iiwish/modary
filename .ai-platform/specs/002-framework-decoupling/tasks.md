# Framework Decoupling F0 Work Graph

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-07-31

## T010: Freeze Contract And Preserve Consumer Assets

Status: Completed
Priority: P0
Depends on: None
Blocks: T011, T012, T013, T014, T015, T016
Story / Requirement: FR-001, FR-002, FR-012, NFR-007
Parallel: No
Conflicts with: T011, T015
Goal: Establish the framework SSOT, canonical path, preservation inventory, and neutral package plan.
Deliverables: Confirmed spec, plan, checklist, analysis, work graph, execution packets, and checksum-verified prototype snapshot.
Allowed files: `.ai-platform/**`, `docs/**`, `/Users/iiwish/self/rulary/**`, `go.mod`.
Test targets: Delivery artifact structure and source-to-snapshot file checksums.
Acceptance criteria: Current framework decisions are canonical and every source file in the preserved prototype inventory has a byte-identical destination.
Definition of Done: current contract is canonical and every Rulary-owned source artifact has a verified destination before removal.
Validation commands: `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 002-framework-decoupling --task-id T010 --strict`; maintainer-local transfer checksum comparison; `git diff --check`.
TDD plan: RED incomplete artifact validation; GREEN complete artifacts and verified copy; REFACTOR canonical pointers.
Packet path: `packets/T010.yaml`
Evidence required: changed files, validation results, checksum result, review, and residual risk.

## T011: Build Public Kernel And Lifecycle

Status: Completed
Priority: P0
Depends on: T010
Blocks: T012, T013, T014
Story / Requirement: FR-001, FR-002, FR-003, FR-004, FR-005, FR-010, NFR-001, NFR-004, NFR-005, NFR-006
Parallel: No
Conflicts with: T012, T013, T014
Goal: Publish top-level Kernel packages, pure Module Definitions, typed services, safe lifecycle, Action binding ownership, and execution scope.
Deliverables: Canonical import path, pure Definition/ActionBinding contract, generic service keys, sealed installation Scope and read-only Handler Resolver, Host-owned migrations, safe database Access, Host state machine and cleanup, opaque execution scope, read-only Action catalog, and focused tests.
Allowed files: public Kernel directories, former `core/**`, Kernel tests, neutral Adapter schema migrations.
Test targets: Module graph/definition/service/lifecycle tests; Action registry/runtime/scope tests; concurrent callback and independent AuditFailure deadline tests; cleanup timeout overlap; non-cooperative Start and HandlerFactory rollback; explicit import-matrix fixtures; external custom-capability and retained-Resolver conformance; existing Runtime regressions.
Acceptance criteria: Definitions validate without Start; capability violations and recreated keys fail; provider and consumer share one package-level typed key; retained HandlerFactory Resolver use fails closed; Action owner cannot be forged; Catalog exposes no Handler; partial startup cleanup starts in reverse/LIFO order and timed-out callbacks may overlap provider cleanup; shutdown is exactly-once; Runtime dependencies document concurrency and context ownership; execution scope replaces public workspace fields.
Definition of Done: lifecycle and capability contracts pass with no raw handler escape through AppKit-facing APIs.
Validation commands: focused package tests; focused race tests; `go test ./...`; `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 002-framework-decoupling --task-id T011 --strict`; `git diff --check`.
TDD plan: RED lifecycle/capability/catalog/scope tests; GREEN minimal contracts; REFACTOR public names and remove transitional escape hatches.
Packet path: `packets/T011.yaml`
Evidence required: RED/GREEN results, changed files, lifecycle ordering evidence, public API review, and residual risk.

## T012: Build Public AppKit And Transports

Status: Completed
Priority: P0
Depends on: T011
Blocks: T015
Story / Requirement: FR-005, FR-006, FR-007, FR-010, FR-013, NFR-001, NFR-004, NFR-005, NFR-007
Parallel: No
Conflicts with: T013, T014, T015
Goal: Add public AppKit, app command helper, HTTP/MCP transport, explicit metadata/assets, and clean shutdown.
Deliverables: Opaque Application assembly, read-only Runtime/catalog/identity views, readiness, explicit Health/API/MCP/bounded-snapshot SPA handlers, and consumer-owned serve/token-authenticated Action command orchestration.
Allowed files: `appkit/**`, `appcmd/**`, `transport/**`, transport tests, former `internal/app/**` and `internal/transport/**`.
Test targets: AppKit preflight/assembly/shutdown tests; external public-surface tests; HTTP session/CSRF/body/Action/error tests; MCP auth/Actor/metadata/tool/schema tests; SPA and appcmd Actor/drain/cleanup tests; Linux/Darwin token-file policy and other-platform pre-filesystem rejection tests; opaque public error-chain tests against hostile extension and dependency causes.
Acceptance criteria: Consumers assemble without internal imports; Application exposes no Host/Registry/database/Handler; mounts are explicit and Module-ID independent; every Action path uses Runtime; malformed authenticator Actors fail before discovery. CLI `--token-file <path>` is Linux/Darwin-only and requires a regular file owned by the effective UID with exact mode `0400` or `0600`; on Darwin the retained open file descriptor rejects extended ACLs. Every other operating system rejects a token path before any filesystem access and supports only `--token-file -` through standard input. All extension and dependency causes cross one opaque public error-chain boundary; standard `errors.Is` and `errors.As` match a bounded chain without invoking caller-defined `Error`, `Is`, `As`, or `Unwrap`, and consumers import no internal helper. `appcmd.Options.Stdout` and `Stderr` remain trusted cooperative dependencies whose `Write` calls must return; context cancellation and shutdown timeouts do not claim to interrupt a blocked writer. Shutdown drains HTTP before Module cleanup.
Definition of Done: an external consumer can assemble and run without internal imports or concrete Module-ID checks.
Validation commands: affected tests/race/vet; Kernel race regression; neutrality scan; `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 002-framework-decoupling --task-id T012 --strict`; `git diff --check`.
TDD plan: RED AppKit/transport/appcmd boundary and failure tests; GREEN minimal public assembly and projections; REFACTOR delete old internal integration without compatibility escapes.
Packet path: `packets/T012.yaml`
Evidence required: changed files, RED/GREEN results, validation, public API review, protocol/security review, lifecycle review, and residual risk.

## T013: Build Pure Consumer Tooling

Status: Completed
Priority: P0
Depends on: T011
Blocks: T015, T016
Story / Requirement: FR-002, FR-007, FR-008, NFR-001, NFR-002, NFR-003, NFR-007, NFR-008
Parallel: No
Conflicts with: T015
Goal: Publish pure, cancelable, deterministic verify, generate, and check tooling plus a cancelable, root-verified consumer build driven by one consumer-supplied Definition.
Allowed files: `projecttool/**`, `action/typescript.go`, focused tests, T013 packet/task/evidence.
Test targets: strict manifest, pure inspection, deterministic identities, complete-batch rendering, sibling-rename replacement, rollback, verified-root mutation, load-captured filesystem-identity locking, retained canonical TMPDIR ancestry, symlinked TMPDIR alias and malicious GOTMPDIR override, Linux/Darwin parent and child staging policy, inherited Darwin ACL rejection at every level, unsupported-platform fail-closed build, isolated Go controls, command-directory replacement, waitid-backed compiler-process-group cleanup after zero and non-zero exits, cooperative-writer cancellation and inherited-pipe bounds, opaque writer-error chains, fuzz, and public API tests.
Deliverables: Context-aware project API, strict portable manifest, deterministic graph/catalog/TypeScript generation, read-only drift checking, Node-free Go build workflow, and consumer-owned command dispatcher.
Acceptance criteria: Inspection invokes no runtime callback or migration source; artifact mutation stays inside one verified Root; cancellation prevents pre-commit mutation; output is order-independent; unsafe numeric TypeScript literals fail; same-root aliases share the in-process gate by the filesystem identity captured when the Project is loaded, not by pathname spelling. Build invokes no frontend tool and gives Go only an outside-project operating system temporary output. It canonicalizes `TMPDIR`, rejects it inside or linked into the project, and retains and revalidates descriptors and identities for it and every ancestor through `/`. Every retained level is effective-UID- or root-owned; group- or other-writable levels are accepted only when root-owned and sticky. Darwin rejects extended ACLs at every level. The child staging directory is effective-UID-owned with exact mode `0700`, and Darwin rejects its extended ACL. Build removes every inherited case-variant `TMPDIR` and `GOTMPDIR` entry from the child environment, then sets both exactly once to the same canonical staging parent whose descriptor and ancestry are retained and revalidated. An ambient `GOTMPDIR` cannot override that parent. A fake Go regression receives a symlink alias through ambient `TMPDIR` and a malicious project-path `GOTMPDIR`, then observes that canonical parent in both child variables. Every other platform fails Build because F0 has no validated ACL policy; unsupported platforms are cross-compile-only where covered and have no native Build, ACL, or rename runtime validation claim. Build sets `GO111MODULE=on`, `GOTOOLCHAIN=local`, `GOENV=off`, `GOWORK=off`, and an empty `GOFLAGS`, and passes `-mod=readonly` and `-buildvcs=false`. Other inherited environment, the selected Go executable and toolchain, consumer source, `command.Dir` pathname, and same-UID replacement remain trusted inputs or deployment boundaries; Build is not a sandbox. On Linux and Darwin, `waitid(WEXITED|WNOWAIT)` observes but does not reap the started process-group leader; while its PID and PGID remain reserved, Build kills residual same-group descendants after zero and non-zero exits, then calls `Cmd.Wait`. When the leader and pre-reap group cleanup succeeded and the context remains active, `exec.ErrWaitDelay` from `Cmd.Wait` is only a residual inherited-pipe close backstop and does not fail Build. Writer, process-exit, cancellation, and cleanup errors still fail. Daemonized or re-grouped trusted descendants remain outside any strong process-sandbox claim. With cooperative output writers, compiler cancellation and inherited output pipes have a bounded wait. Caller-supplied `io.Writer.Write` must return because Go cannot interrupt a blocked call. Writer and other dependency causes cross the opaque public error-chain boundary without invoking caller-defined `Error`, `Is`, `As`, or `Unwrap`. Validated bytes are installed through the Root.
Definition of Done: Focused tests, race, vet, count-20, fuzz, cross-build, and independent review pass with no unresolved P0-P2 finding.
Validation commands: `go test ./projecttool ./action`; `go test -race ./projecttool`; `go vet ./projecttool`; `go test -count=20 ./projecttool`; manifest fuzz; Linux/Windows cross-build; `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 002-framework-decoupling --task-id T013 --strict`; `git diff --check`.
TDD plan: RED cancellation/path/root-swap/load-identity-lock/order/build/platform-ACL/TMPDIR-GOTMPDIR/environment/process-group/writer/pipe failures; GREEN bounded context-aware operations and fail-closed platform boundaries; REFACTOR shared verified-root and deterministic preflight logic.
Packet path: `packets/T013.yaml`
Evidence required: Changed files, RED/GREEN/REFACTOR results, purity and filesystem review, validation output, independent review, and residual risk.

## T014: Neutralize Official Adapters

Status: Completed
Priority: P0
Depends on: T011
Blocks: T015, T016
Story / Requirement: FR-003, FR-004, FR-009, FR-010, NFR-001, NFR-004, NFR-005, NFR-006
Parallel: No
Conflicts with: T011
Goal: Publish explicit SQLite, local Identity, RBAC, and SQL Audit Adapters with no implicit provisioning or consumer policy.
Allowed files: `adapters/**`, `database/**`, focused contract tests, T014 packet/task/evidence.
Test targets: empty and explicit provisioning, migration history, transaction opacity, revocation, authorization, audit corruption, filesystem ownership, effective-UID/root-owned SQLite ancestry, root-owned sticky writable-ancestor exceptions, restart, and concurrency.
Deliverables: Neutral Adapter registrations, schema-only zero options, typed provisioning/revocation options, secure SQLite profile, transaction-aware stores, and bounded credential verification.
Acceptance criteria: Empty options create no principal, credential, role, binding, event, secret, or domain row; write invariants share one opaque transaction; policy and identity revocation fail closed. File-backed SQLite requires every directory ancestor to be owned by the effective UID or root and accepts a group- or other-writable ancestor only when root-owned and sticky; the final database directory remains effective-UID-owned and non-writable by group or other users. Existing ownership and mode are validated without changing either.
Definition of Done: Focused tests, race, vet, count-20, POSIX/Windows compile checks, neutrality scan, and independent review pass with no unresolved P0-P2 finding.
Validation commands: `go test ./action ./adapters/... ./database`; race, vet, and count-20 equivalents; Linux/Windows compile checks; Adapter neutrality scan; `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 002-framework-decoupling --task-id T014 --strict`; `git diff --check`.
TDD plan: RED provisioning/transaction/revocation/corruption/filesystem/concurrency failures; GREEN explicit durable Adapters; REFACTOR common executor, validation, and bounded-security contracts.
Packet path: `packets/T014.yaml`
Evidence required: Changed files, RED/GREEN/REFACTOR results, validation output, provisioning/durability/security reviews, independent review, and residual risk.

## T015: Prove Independent Consumer And Remove Domain Coupling

Status: Completed
Priority: P0
Depends on: T012, T013, T014
Blocks: T016
Story / Requirement: FR-001, FR-002, FR-005, FR-006, FR-007, FR-008, FR-009, FR-011, FR-012, FR-013, NFR-001, NFR-002, NFR-003, NFR-007, NFR-008
Parallel: No
Conflicts with: T013, active application-tree cleanup
Goal: Prove copied-out independent consumption and leave the active Modary repository as a domain-neutral Go framework.
Allowed files: `testdata/external-consumer/**`, `scripts/**`, `Makefile`, `.github/**`, `.gitignore`, active framework docs, application-owned path removals, T015 task/packet/evidence.
Test targets: copied-out source, fake Node-family shims, pure inspection, generated drift, Runtime/HTTP/command/MCP conformance, supported token source, public `errors.Is`/`errors.As` without an internal helper, restart, scope isolation, empty provisioning, neutrality, build, and docs gates.
Deliverables: Independent Counter conformance module, checksum-protected external archives, application-tree removal, Node-free Go framework gates, active neutrality and documentation checks, and canonical framework docs.
Acceptance criteria: A copied module imports only public Modary packages, including no internal error helper, runs with `GOWORK=off`, invokes no Node-family tool, exercises one governed durable write across every channel and restart, uses standard `errors.Is` and `errors.As` across opaque public chains without triggering caller-defined `Error`, `Is`, `As`, or `Unwrap`, proves the supported CLI token source on its host, and leaves the active framework free of consumer domain or application release paths.
Definition of Done: Archive manifests reverify; root and copied consumer tests/vet/build/generated/neutrality/docs gates pass; evidence is complete; independent boundary review has no unresolved P0-P2 finding.
Validation commands: maintainer-local `sh .ai-platform/evidence/T010/check-archives.sh` preservation audit; `make acceptance`; copied-out consumer gate; `scripts/check-neutrality.sh`; `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 002-framework-decoupling --task-id T015 --strict`; `git diff --check`.
TDD plan: RED copied-out/import/Node/drift/channel/restart/empty-provisioning and active-domain scans; GREEN external composition and neutral repository; REFACTOR canonical Node-free Go framework gates and docs.
Packet path: `packets/T015.yaml`
Evidence required: Changed and removed files, archive verification, consumer and root test results, neutrality/build/CI review, independent boundary review, and residual risk.

## T016: Full Review And F0 Acceptance

Status: Completed
Priority: P0
Depends on: T015
Blocks: None
Story / Requirement: FR-001 through FR-013; NFR-001 through NFR-010
Parallel: No
Conflicts with: All active implementation tasks
Goal: Complete two independent architecture/code review passes, repair every P0-P2 finding, and publish truthful framework F0 acceptance evidence.
Allowed files: Repository-wide reviewed fixes, canonical docs, `.ai-platform/evidence/T016/**`, T016 task and packet. Completed T010-T015 evidence is read-only historical input.
Test targets: Full framework and copied consumer acceptance, race, count-20, fuzz, cross-build, public API documentation, neutrality, generated drift, source cleanliness, and governance artifact validation.
Deliverables: Reviewed fixes, complete evidence, canonical acceptance and release reports, final known limitations, and closed work graph.
Acceptance criteria: Every required automated gate passes from the active tree; two independent passes report no unresolved P0-P2; evidence matches actual commands; reports distinguish local source proof from a future tagged release.
Definition of Done: `make acceptance`, `make ci`, all T010-T016 strict artifact validators, `git diff --check`, and final independent reviews pass with no required work remaining. The T010 external archive checksum is a maintainer-local preservation audit, not a consumer, CI, or framework release gate.
Validation commands: `make acceptance`; `make ci`; `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 002-framework-decoupling --task-id T016 --strict`; rerun the corresponding exact strict artifact validator for T010 through T015; `git diff --check`. T016 consumes the completed T010 preservation evidence as historical input and does not read or revalidate any external project path.
TDD plan: RED every review or gate finding; GREEN targeted repair with focused regression; REFACTOR only where it reduces verified complexity; repeat full gates after the last change.
Packet path: `packets/T016.yaml`
Evidence required: Changed files, complete command results, acceptance matrix, residual risks, accurate release boundary, and two different fresh reviewers whose reports record start time, scope, commands, and the same frozen-tree SHA-256 digest with zero P0-P2 findings.
