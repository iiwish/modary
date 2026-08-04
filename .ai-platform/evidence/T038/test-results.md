# T038 Test Results

- Result: Passed
- Date: 2026-08-02

- `make tidy-check`: all root, published component, integration, and Counter
  modules are tidy under Go 1.26.5 with `GOWORK=off`.
- `make test-framework`: root, ordinary PostgreSQL, governed PostgreSQL, and
  integration modules passed against PostgreSQL.
- `make vet`: every shipped module and Counter passed.
- `make race`: root, both components, integration, and copied-out Counter passed.
- `make cross-build` passed every supported target for Core, both published
  component modules, integration, and the copied-out Counter consumer.
- `make vulncheck` reported no called vulnerability in Core, either component,
  integration, or Counter.
- `make repeat` passed shuffled repeated suites across every module and Counter.
- Starter dependency-graph tests prove API has no PostgreSQL/River, default
  Admin has no River, and selected operational/Governed consumers have River.
- RED: the release fixture accepted a root-only tag and the remote gate edited
  only the root replacement. GREEN: `go test ./scripts` rejects an incomplete
  tag train, version drift, non-annotated/misaligned tags, and verifies all
  three remote module identities without replacements.
- Actual candidate preflight validates the current acceptance report, module
  paths, exact version requirements, Go baseline, and release documents before
  stopping at the expected `clean committed worktree` boundary.
- `scripts/check-neutrality.sh`, import-direction tests, docs, generated-state,
  and `git diff --check` passed.
