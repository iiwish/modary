// Package sqlaudit provides a neutral SQLite-backed audit Hook.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package sqlaudit

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/module"
)

// ModuleID is the stable Module manifest and migration owner identifier.
const ModuleID = "sql-audit"

//go:embed migrations/sqlite/*.sql
var migrationFiles embed.FS

var sqliteMigrations = mustMigrationFS()

// Options is reserved for future backward-compatible storage policy. Its zero
// value is the complete F0 contract and provisions no events.
type Options struct{}

// Module returns a pure Registration for the SQL Audit Adapter.
func Module(_ Options) module.Registration {
	return module.Registration{
		Definition: module.Definition{
			Manifest: module.Manifest{
				SchemaVersion: module.SchemaVersion,
				ID:            ModuleID,
				Version:       "0.1.0",
				Type:          module.ModuleTypeAdapter,
				Requires:      []module.Capability{module.CapabilityDatabase},
				Provides:      []module.Capability{module.CapabilityAudit},
			},
			Migrations: []module.MigrationSource{{Driver: "sqlite", Files: sqliteMigrations}},
		},
		Start: start,
	}
}

func start(ctx context.Context, installation module.Scope) error {
	if ctx == nil {
		return fmt.Errorf("SQL Audit start context is required")
	}
	control, err := moduleassembly.ResolveDatabaseControl(installation)
	if err != nil {
		return fmt.Errorf("resolve database control: %w", err)
	}
	return module.Provide(installation, module.AuditHook(), audit.Hook(&hook{control: control}))
}

func mustMigrationFS() fs.FS {
	files, err := fs.Sub(migrationFiles, "migrations/sqlite")
	if err != nil {
		panic(err)
	}
	return files
}
