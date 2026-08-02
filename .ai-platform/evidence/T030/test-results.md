# T030 Test Results

- Result: Passed
- Date: 2026-08-02

Validation results:

- RED: `go test ./starter ./httpkit ./cmd/modary` failed because all three
  production packages were absent.
- GREEN: `go test ./starter ./httpkit ./cmd/modary -count=1` passed.
- Copied-out API: two independent deterministic renderings matched; the first
  ran `go mod tidy`, `go list -deps`, `go test ./...`, and
  `go build ./cmd/sample-api` with `GOWORK=off`.
- Runtime: the copied-out binary bound a real loopback listener, answered health,
  received an interrupt, shut down, and exited successfully.
- Absence: generated source and package dependencies contain no PostgreSQL,
  River, local Identity, RBAC, SQL Audit, Action, Identity, Audit, Authz, task,
  MCP command, or database configuration import/surface.
- Ownership: existing files, non-empty destinations, symlink destinations,
  invalid paths, invalid modules, unavailable Profiles, canceled contexts, and
  repeat creation were rejected without changing consumer content.
- Race: `go test -race ./starter ./httpkit` passed.
- Vet: `go vet ./starter ./httpkit ./cmd/modary` passed.
- Full repository: `go test ./... -count=1` passed.
- Cross build: the global command built for Linux, macOS, and Windows on amd64
  and arm64 with `CGO_ENABLED=0`.
- Documentation, strict T030 delivery artifacts, tidy-diff, formatting, and
  `git diff --check` gates passed.
