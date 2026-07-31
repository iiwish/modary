package sqlite

import (
	"fmt"

	"github.com/iiwish/modary/internal/sqlpolicy"
)

const maxMigrationScriptBytes = 1 << 20

// ValidateMigration accepts the strict durable SQLite DDL/DML script profile.
// Transaction control, temporary schema, and rollback conflict policy remain
// owned by the framework transaction boundary.
func (*backend) ValidateMigration(script string) error {
	if err := sqlpolicy.ValidateMigrationScript(script, maxMigrationScriptBytes); err != nil {
		return fmt.Errorf("SQLite migration is outside the supported profile: %w", err)
	}
	return nil
}
