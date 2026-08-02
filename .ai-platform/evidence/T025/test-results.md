# T025 Test Results

- Result: Passed
- Completed at: 2026-08-01T14:39:05Z

- `go test -count=1 ./internal/databasecontrol ./adapters/postgres ./adapters/localidentity ./adapters/rbac ./adapters/sqlaudit`
- `go test -race -count=1 ./adapters/postgres ./adapters/localidentity ./adapters/rbac ./adapters/sqlaudit`
- PostgreSQL 17 migration drift, corruption, restart, authorization,
  credentialless principal, credential-target, audit, and transaction tests
- `git diff --check`
