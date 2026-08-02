// Package postgres provides Modary's governed PostgreSQL profile and
// River-backed transactional task service.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/task"
)

// ModuleID is the stable Module manifest and migration owner identifier.
const ModuleID = "postgres"

const (
	schemaProfileTable    = "modary_schema_profile"
	schemaRoleApplication = "application"
	schemaRoleQueue       = "queue"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// Module returns a side-effect-free PostgreSQL registration. Network and
// database work starts only when the Host invokes Start.
func Module(options Options) (module.Registration, error) {
	normalized, config, err := normalizeOptions(options)
	if err != nil {
		return module.Registration{}, err
	}
	migrations, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return module.Registration{}, fmt.Errorf("prepare PostgreSQL migrations: %w", err)
	}
	return module.Registration{
		Definition: module.Definition{
			Manifest: module.Manifest{
				SchemaVersion: module.SchemaVersion,
				ID:            ModuleID,
				Version:       "0.1.0",
				Type:          module.ModuleTypeAdapter,
				Provides:      []module.Capability{module.CapabilityDatabase, module.CapabilityTasks},
			},
			Migrations: []module.MigrationSource{{Driver: "postgres", Files: migrations}},
		},
		Start: func(ctx context.Context, scope module.Scope) error {
			return start(ctx, scope, normalized, config.Copy())
		},
	}, nil
}

func start(ctx context.Context, scope module.Scope, options Options, config *pgx.ConnConfig) error {
	if ctx == nil {
		return fmt.Errorf("PostgreSQL start context is required")
	}
	db := stdlib.OpenDB(*config)
	configurePool(db, options.MaxOpenConnections, options.MaxIdleConnections, options.ConnectionMaxLifetime)
	resource := &databaseResource{db: db}
	if err := module.OnStop(scope, resource.close); err != nil {
		_ = db.Close()
		return err
	}
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect PostgreSQL database: %w", err)
	}
	migrationLockKey := schemaAdvisoryLockKey(options.ApplicationSchema)
	if err := prepareSchemasAndQueue(ctx, db, config, options); err != nil {
		return err
	}

	databaseBackend := &backend{db: db, migrationLockKey: migrationLockKey}
	control, err := databasecontrol.New(databaseBackend)
	if err != nil {
		return fmt.Errorf("create PostgreSQL database control: %w", err)
	}
	tasks, err := newTaskService(db, databaseBackend, options.QueueSchema)
	if err != nil {
		return err
	}
	if err := module.OnStop(scope, tasks.close); err != nil {
		return err
	}
	transactions := &transactionManager{control: control}
	plans := actionpersistence.PlanStore(&planStore{control: control})
	idempotency := actionpersistence.IdempotencyStore(&idempotencyStore{control: control})
	if err := moduleassembly.ProvideDatabase(scope, control); err != nil {
		return err
	}
	if err := moduleassembly.ProvideActionPersistence(scope, plans, idempotency, transactions); err != nil {
		return err
	}
	return module.Provide[task.Service](scope, module.Tasks(), tasks)
}

func configurePool(db *sql.DB, maxOpen, maxIdle int, lifetime time.Duration) {
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(lifetime)
}

func prepareSchemasAndQueue(
	ctx context.Context,
	db *sql.DB,
	config *pgx.ConnConfig,
	options Options,
) (err error) {
	lockConnection, err := pgx.ConnectConfig(ctx, config.Copy())
	if err != nil {
		return fmt.Errorf("connect PostgreSQL startup coordinator: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeErr := lockConnection.Close(closeCtx); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close PostgreSQL startup coordinator: %w", closeErr))
		}
	}()
	lockKeys := orderedSchemaLockKeys(options.ApplicationSchema, options.QueueSchema)
	for _, lockKey := range lockKeys {
		if _, err := lockConnection.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
			return fmt.Errorf("acquire PostgreSQL startup lock: %w", err)
		}
	}
	if err := createSchemas(ctx, lockConnection, options.ApplicationSchema, options.QueueSchema); err != nil {
		return err
	}
	if err := bindSchemaPair(ctx, lockConnection, options.ApplicationSchema, options.QueueSchema); err != nil {
		return err
	}
	riverDriver := riverdatabasesql.New(db)
	migrator, err := rivermigrate.New(riverDriver, &rivermigrate.Config{Schema: options.QueueSchema})
	if err != nil {
		return fmt.Errorf("create River migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("apply River migrations: %w", err)
	}
	for index := len(lockKeys) - 1; index >= 0; index-- {
		var unlocked bool
		if err := lockConnection.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, lockKeys[index]).Scan(&unlocked); err != nil {
			return fmt.Errorf("release PostgreSQL startup lock: %w", err)
		}
		if !unlocked {
			return fmt.Errorf("release PostgreSQL startup lock: lock was not held")
		}
	}
	return nil
}

func createSchemas(ctx context.Context, connection *pgx.Conn, schemas ...string) error {
	for _, schema := range schemas {
		if _, err := connection.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+quoteIdentifier(schema)); err != nil {
			return fmt.Errorf("create PostgreSQL schema %q: %w", schema, err)
		}
		var owned bool
		if err := connection.QueryRow(ctx, `
			SELECT pg_get_userbyid(nspowner) = current_user
			FROM pg_namespace
			WHERE nspname = $1`, schema).Scan(&owned); err != nil {
			return fmt.Errorf("inspect PostgreSQL schema %q ownership: %w", schema, err)
		}
		if !owned {
			return fmt.Errorf("PostgreSQL schema %q must be owned by the configured database user", schema)
		}
	}
	return nil
}

func bindSchemaPair(ctx context.Context, connection *pgx.Conn, application, queue string) error {
	tx, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin PostgreSQL profile binding: %w", err)
	}
	defer tx.Rollback(ctx)
	bindings := []struct {
		schema, role, peer string
	}{
		{schema: application, role: schemaRoleApplication, peer: queue},
		{schema: queue, role: schemaRoleQueue, peer: application},
	}
	for _, binding := range bindings {
		table := quoteIdentifier(binding.schema) + `.` + quoteIdentifier(schemaProfileTable)
		if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (
			profile_id SMALLINT PRIMARY KEY CHECK (profile_id = 1),
			schema_role TEXT NOT NULL CHECK (schema_role IN ('application', 'queue')),
			peer_schema TEXT NOT NULL CHECK (octet_length(peer_schema) BETWEEN 1 AND 63)
		)`); err != nil {
			return fmt.Errorf("create PostgreSQL schema profile binding for %q: %w", binding.schema, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO `+table+` (profile_id, schema_role, peer_schema)
			VALUES (1, $1, $2) ON CONFLICT (profile_id) DO NOTHING`, binding.role, binding.peer); err != nil {
			return fmt.Errorf("bind PostgreSQL schema profile for %q: %w", binding.schema, err)
		}
		var boundRole, boundPeer sql.NullString
		if err := tx.QueryRow(ctx, `SELECT
			CASE WHEN octet_length(schema_role) <= 11 THEN schema_role END,
			CASE WHEN octet_length(peer_schema) <= 63 THEN peer_schema END
			FROM `+table+` WHERE profile_id = 1`).Scan(&boundRole, &boundPeer); err != nil {
			return fmt.Errorf("verify PostgreSQL schema profile binding for %q: %w", binding.schema, err)
		}
		if !boundRole.Valid || !boundPeer.Valid || boundRole.String != binding.role || boundPeer.String != binding.peer {
			return fmt.Errorf("PostgreSQL schema %q is already bound to a different profile", binding.schema)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL profile binding: %w", err)
	}
	return nil
}

func orderedSchemaLockKeys(schemas ...string) []int64 {
	keys := make([]int64, 0, len(schemas))
	seen := make(map[int64]struct{}, len(schemas))
	for _, schema := range schemas {
		key := schemaAdvisoryLockKey(schema)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	return keys
}

func schemaAdvisoryLockKey(schema string) int64 {
	digest := sha256.Sum256([]byte("modary:schema:" + schema))
	key := int64(binary.BigEndian.Uint64(digest[:8]))
	if key == 0 {
		return 1
	}
	return key
}

type databaseResource struct {
	db   *sql.DB
	once sync.Once
	err  error
}

func (resource *databaseResource) close(context.Context) error {
	if resource == nil || resource.db == nil {
		return nil
	}
	resource.once.Do(func() { resource.err = resource.db.Close() })
	return resource.err
}
