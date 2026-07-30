# Modary F0 Acceptance Report

- Acceptance object: Rulary address-label vertical slice
- Contract: `docs/modary-f0-rulary-v0.1.md`
- Date: 2026-07-30
- Status: Passed

## Result

All AC-001 through AC-015 pass. The release runs as one statically linked Go
process with one SQLite database, embedded React assets, and no runtime Node.js
or external service dependency.

| Criterion | Result | Primary evidence |
|---|---|---|
| AC-001 module composition | Pass | Deterministic registry generation, generated Module UI entries, and route ownership tests |
| AC-002 missing dependency | Pass | Project verification rejects a selected module whose database capability is absent |
| AC-003 cycle reporting | Pass | Contract test asserts the full `a -> b -> a` path |
| AC-004 shared Action | Pass | Real HTTP, CLI, and MCP adapters use the same registry/runtime; Playwright covers the UI flow |
| AC-005 authorization | Pass | Author publish denial and MCP agent allowlist denial are covered |
| AC-006 structured denial | Pass | HTTP and MCP assertions cover all required error context fields |
| AC-007 plan consistency | Pass | Changed input, expired plan, changed source snapshot, and actor/workspace binding tests |
| AC-008 Agent limit | Pass | A 51-row plan is rejected at Execute for a grant with `max_rows=50` |
| AC-009 address result | Pass | Standalone golden dataset asserts exact output and source-backed evidence offsets |
| AC-010 immutable version | Pass | SQLite update/delete triggers and integration coverage |
| AC-011 idempotency | Pass | Replayed execution returns the original run without another side effect |
| AC-012 unified audit | Pass | Preview, allow, replay, deny, and internal failure paths record correlated events |
| AC-013 single binary | Pass | Linux scratch container completes login, create, validate, preview, publish, run, result, and audit |
| AC-014 resource budget | Pass | Executable gates enforce process readiness, idle RSS, and 1000-row Preview budgets |
| AC-015 kernel boundary | Pass | AST import checks and a core-domain term scan run in the contract suite |

## Release Verification

Release binaries are produced with:

```bash
go run ./cmd/modary build --output dist/modary-rulary
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o dist/modary-rulary-linux-amd64 ./cmd/modary
```

The measured binaries are approximately 15 MiB for Darwin arm64 and Linux
amd64, and 14 MiB for Linux arm64. The Linux artifacts are statically linked.

The clean Linux smoke run used a scratch image restricted to 2 CPUs, 2 GiB of
memory, and 128 PIDs. It returned:

```json
{
  "registered_address": "平顶山市卫东区建设路东段南4号院",
  "business_address": "平顶山市黄河路与高新大道交叉口尼龙织造产业园内办公楼50号",
  "matched_rows": 20,
  "result_rows": 20,
  "audit_events": 8
}
```

## Resource Budget

Measurements use Docker/OrbStack on an arm64 host with explicit `--cpus 2
--memory 2g`. The amd64 row runs through platform emulation and is therefore a
conservative compatibility measurement.

| Linux release | Process readiness | Idle RSS | Budget |
|---|---:|---:|---|
| arm64 native | 240 ms | 23.30 MiB | Pass |
| amd64 emulated | 265 ms | 62.91 MiB | Pass |

The Linux amd64 1000-row Preview benchmark runs a compiled integration test in a
2C2G container. Its 20-sample P95 is 29.48 ms. The target is at most 2 seconds
to readiness, 128 MiB idle RSS, and 3 seconds Preview P95.
`startup_ms` measures process initialization through successful listener bind;
this is the NFR readiness gate. The external Docker health probe measured 602
ms on native arm64 and 605 ms under amd64 emulation. That diagnostic also
includes container creation, port publication, polling, and CPU translation.

Reproduction commands:

```bash
make release-acceptance
```

## Delivery Artifacts

- Module graph: `internal/generated/module_graph.json`
- Module registry: `internal/generated/modules_gen.go`
- Action catalog: `internal/generated/action_catalog.json`
- Action schemas: `internal/generated/action_schemas.json`
- Golden dataset: `modules/rulary-core/testdata/address_golden.json`
- Permission matrix: `docs/f0-permissions.md`
- Audit sample: `docs/examples/audit-event.json`
- Architecture decision: `docs/adr/ADR-007-f0-runtime-implementation.md`
- Known limitations: `docs/f0-known-limitations.md`
- Next-stage proposal: `docs/f1-proposal.md`

## Test Inventory

- Go contract and unit tests: `go test ./...`
- Static analysis and race detector: `go vet ./...`, `go test -race ./...`
- 1000-row performance test: `TestPreviewPerformance1000Rows`
- React unit tests: `pnpm test`
- Type checks: `pnpm typecheck`
- Desktop and mobile E2E: `pnpm test:e2e`
- Clean release smoke: `scripts/f0-smoke.sh`

## Hardening Evidence

- Every pooled SQLite connection enables foreign keys and a five-second busy
  timeout through the connection DSN.
- Bootstrap password changes rotate bootstrap credentials and invalidate local
  sessions; deactivating a delegator also revokes the delegated Agent identity.
- Public non-loopback listeners reject the built-in demo password or Agent token.
- Write actions declare idempotency requirements, and Action descriptors fail
  closed when schemas, channels, preview policy, or audit policy are invalid.
- Module manifests own UI route entries; excluding the console module removes the
  SPA route rather than leaving an independently hardcoded shell.
