# T049 Test Results

- Result: Passed
- Date: 2026-08-27
- Go: 1.26.7 darwin/arm64

## Results

- Go 1.26.5 vulnerability baseline: failed as expected with reachable
  GO-2026-6218, GO-2026-6090, GO-2026-6089, GO-2026-5972, and GO-2026-5026.
- Go 1.26.7 vulnerability gate: passed with zero reachable vulnerabilities for
  the root and scanned modules.
- Release preflight Go-baseline RED: passed by rejecting the old expectation.
- Release preflight Go-baseline GREEN: passed with the 1.26.7 contract.
- Starter default-version RED: passed by rejecting Alpha 1.
- Starter default-version GREEN: passed with `v0.3.0-alpha.2`.
- `make acceptance`: passed with real PostgreSQL.
- `make race`: passed across the root and all four component modules.
- `make ci`: passed all acceptance, race, repeat, fuzz, copied-out Profile,
  generated-source, documentation, vulnerability, vet, and platform checks.
- `make provider-acceptance`: passed with disposable Dex and OpenTelemetry
  Collector infrastructure.
- `make container-acceptance VERSION=v0.3.0-alpha.2`: passed for API, Admin,
  and Governed Profiles, including migration-only execution, non-root runtime,
  probes, graceful shutdown, and runtime-content inspection.
- `go test ./scripts`, `go test ./starter`, `./scripts/check-docs.sh`, strict
  T049 artifact validation, and `git diff --check`: passed.
- Hosted candidate CI run `33039491126`: three jobs passed; the quality job
  failed when `TestServeEmitsStructuredHTTPLifecycle` canceled immediately
  after listener creation but before `Serve` crossed its documented startup
  boundary. No tag had been created, so publication remained stopped.
- Lifecycle synchronization fix: `go test ./appcmd
  -run '^TestServeEmitsStructuredHTTPLifecycle$' -count=1000
  -shuffle=1787805376018570912` and the same focused test with `-race -count=50
  -shuffle=on`: passed.
- Post-fix `make repeat`: passed across the full repeated package, Starter,
  PostgreSQL, Governed, integration, and copied-consumer matrix.

The first container-acceptance attempt exposed a host port collision with an
unrelated local service. Dynamic loopback port allocation removed the shared
host assumption, and the complete gate then passed without stopping that
service.
