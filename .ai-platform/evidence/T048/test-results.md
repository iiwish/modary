# T048 Test Results

- Result: Passed
- Date: 2026-08-04

## Candidate

- `make release-readiness VERSION=v0.3.0-alpha.1`: passed from a clean committed
  worktree with full CI, real Dex and OpenTelemetry Collector acceptance, and
  source-mode API, Admin, and Governed container acceptance.
- Hosted main CI run `30893905267`: passed all quality, copied-profile,
  Darwin ARM64, and operational-provider jobs.

## Published Source

- Hosted tag CI run `30895918247`: passed all quality jobs and the release job.
- Tag-mode preflight: passed for one candidate commit and five aligned annotated
  module tags.
- Hosted replacement-free remote consumer: passed for all five modules.
- Local replacement-free remote consumer: passed for all five modules. The
  public proxy served module source; `GONOSUMDB=github.com/iiwish/*` was used
  only because the selected proxy's public checksum-database endpoint returned
  HTTP 504 for the newly published component version.
- Hosted and local released-source API, Admin, and Governed container
  acceptance: passed, including non-root execution, migration-only startup,
  readiness/liveness, graceful termination, and runtime-content inspection.
- Final record validation: full `go test ./scripts`, canonical documentation and
  link checks, candidate-tag-aware acceptance digest, strict T048 artifact
  validation, and `git diff --check` passed.
- Final source worktree before the release record: clean.
