// Package sqlaudit provides a neutral PostgreSQL-backed audit Hook.
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

//go:embed migrations/postgres/*.sql
var migrationFiles embed.FS

var postgresMigrations = mustMigrationFS()

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
				Provides: []module.Capability{
					module.CapabilityAudit, module.CapabilityAuditInspection,
				},
			},
			Migrations: []module.MigrationSource{{Driver: "postgres", Files: postgresMigrations}},
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
	store := &hook{control: control}
	if err := module.Provide(installation, module.AuditHook(), audit.Hook(store)); err != nil {
		return err
	}
	return module.Provide(installation, module.AuditReader(), audit.Reader(store))
}

func mustMigrationFS() fs.FS {
	files, err := fs.Sub(migrationFiles, "migrations/postgres")
	if err != nil {
		panic(err)
	}
	return files
}
