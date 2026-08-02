# T026 Test Results

- Result: Passed
- Completed at: 2026-08-02T01:58:49Z

- `make acceptance`
- `make ci`
- Framework and copied-out consumer `go test -race ./...`
- Counter copied-out and `GOWORK=off` conformance, including Action enqueue,
  producer shutdown, application restart, and public Runner consumption
- PostgreSQL schema role-reuse and swapped-profile concurrency repetitions
- Active-tree storage/dependency/import deletion audits
- `python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 005-postgres-task-runtime --task-id T026 --strict`
- `git diff --check`
