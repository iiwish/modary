package governedpostgres

import (
	"context"
	"fmt"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/sqlpolicy"
)

const maxMigrationScriptBytes = 1 << 20

func (*backend) ValidateMigration(script string) error {
	if err := sqlpolicy.ValidateMigrationScript(script, maxMigrationScriptBytes); err != nil {
		return fmt.Errorf("PostgreSQL migration is outside the supported profile: %w", err)
	}
	return nil
}

func (backend *backend) LockMigrations(ctx context.Context, executor database.Executor) error {
	if backend == nil || backend.migrationLockKey == 0 {
		return fmt.Errorf("PostgreSQL migration lock is unavailable")
	}
	if _, err := executor.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, backend.migrationLockKey); err != nil {
		return fmt.Errorf("acquire PostgreSQL migration lock: %w", err)
	}
	return nil
}
