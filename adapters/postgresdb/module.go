package postgresdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/module"
)

// ModuleID is the stable identifier of the general PostgreSQL component.
const ModuleID = "postgres-database"

// Module returns a side-effect-free registration. Network and schema work
// begins only when the Host starts it.
func Module(options Options) (module.Registration, error) {
	normalized, config, err := normalizeOptions(options)
	if err != nil {
		return module.Registration{}, err
	}
	return module.Registration{Definition: module.Definition{Manifest: module.Manifest{
		SchemaVersion: module.SchemaVersion, ID: ModuleID, Version: "0.2.0", Type: module.ModuleTypeAdapter,
		Provides: []module.Capability{module.CapabilityDatabase},
	}}, Start: func(ctx context.Context, installation module.Scope) error {
		return start(ctx, installation, normalized, config.Copy())
	}}, nil
}

func start(ctx context.Context, installation module.Scope, options Options, config *pgx.ConnConfig) error {
	if ctx == nil {
		return fmt.Errorf("PostgreSQL start context is required")
	}
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(options.MaxOpenConnections)
	db.SetMaxIdleConns(options.MaxIdleConnections)
	db.SetConnMaxLifetime(options.ConnectionMaxLifetime)
	resource := &databaseResource{db: db}
	if err := module.OnStop(installation, resource.close); err != nil {
		_ = db.Close()
		return err
	}
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect PostgreSQL database: %w", err)
	}
	lockKey := advisoryLockKey(options.Schema)
	if err := prepareSchema(ctx, config, options.Schema, lockKey); err != nil {
		return err
	}
	control, err := databasecontrol.New(&backend{db: db, migrationLockKey: lockKey})
	if err != nil {
		return fmt.Errorf("create PostgreSQL database control: %w", err)
	}
	return moduleassembly.ProvideDatabase(installation, control)
}

func prepareSchema(ctx context.Context, config *pgx.ConnConfig, schema string, lockKey int64) (err error) {
	connection, err := pgx.ConnectConfig(ctx, config.Copy())
	if err != nil {
		return fmt.Errorf("connect PostgreSQL startup coordinator: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeErr := connection.Close(closeCtx); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close PostgreSQL startup coordinator: %w", closeErr))
		}
	}()
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return fmt.Errorf("acquire PostgreSQL startup lock: %w", err)
	}
	if _, err := connection.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+quoteIdentifier(schema)); err != nil {
		return fmt.Errorf("create PostgreSQL schema %q: %w", schema, err)
	}
	var owned bool
	if err := connection.QueryRow(ctx, `SELECT pg_get_userbyid(nspowner) = current_user FROM pg_namespace WHERE nspname=$1`, schema).Scan(&owned); err != nil {
		return fmt.Errorf("inspect PostgreSQL schema %q ownership: %w", schema, err)
	}
	if !owned {
		return fmt.Errorf("PostgreSQL schema %q must be owned by the configured database user", schema)
	}
	var unlocked bool
	if err := connection.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, lockKey).Scan(&unlocked); err != nil || !unlocked {
		return fmt.Errorf("release PostgreSQL startup lock")
	}
	return nil
}

type databaseResource struct{ db *sql.DB }

func (resource *databaseResource) close(context.Context) error {
	if resource == nil || resource.db == nil {
		return nil
	}
	return resource.db.Close()
}
