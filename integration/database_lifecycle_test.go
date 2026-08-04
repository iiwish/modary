package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	postgres "github.com/iiwish/modary/components/governedpostgres"
	"github.com/iiwish/modary/integration/internal/testpostgres"
	"github.com/iiwish/modary/module"
)

func TestHostAppliesDeclaredMigrationsInDependencyOrderBeforeStart(t *testing.T) {
	databaseConfig := testpostgres.New(t)
	postgresModule, err := postgres.Module(postgres.Options{
		URL: databaseConfig.URL, ApplicationSchema: databaseConfig.ApplicationSchema, QueueSchema: databaseConfig.QueueSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	var orderMu sync.Mutex
	first := migrationModule("migration-first", []module.Capability{module.CapabilityDatabase}, []module.Capability{"migration-first"}, fstest.MapFS{
		"0001_first.sql": {Data: []byte(`
			CREATE TABLE migration_order (position INTEGER PRIMARY KEY, name TEXT NOT NULL);
			INSERT INTO migration_order (position, name) VALUES (1, 'first')`)},
	}, func(ctx context.Context, scope module.Scope) error {
		access, err := module.Resolve(scope, module.Database())
		if err != nil {
			return err
		}
		if _, exposed := any(access).(*sql.DB); exposed {
			return errors.New("public database capability exposes raw sql.DB")
		}
		if err := access.WithinTransaction(ctx, func(context.Context) error { return nil }); err != nil {
			return fmt.Errorf("ordinary transaction facade: %w", err)
		}
		var count int
		if err := access.QueryRowContext(ctx, `SELECT COUNT(*) FROM migration_order`).Scan(&count); err != nil || count != 1 {
			return errors.New("first migration was not applied before Start")
		}
		orderMu.Lock()
		order = append(order, "first")
		orderMu.Unlock()
		return nil
	})
	second := migrationModule("migration-second", []module.Capability{module.CapabilityDatabase, "migration-first"}, nil, fstest.MapFS{
		"0001_second.sql": {Data: []byte(`INSERT INTO migration_order (position, name) VALUES (2, 'second')`)},
	}, func(ctx context.Context, scope module.Scope) error {
		access, err := module.Resolve(scope, module.Database())
		if err != nil {
			return err
		}
		var count int
		if err := access.QueryRowContext(ctx, `SELECT COUNT(*) FROM migration_order`).Scan(&count); err != nil || count != 2 {
			return errors.New("second migration was not applied after its dependency")
		}
		orderMu.Lock()
		order = append(order, "second")
		orderMu.Unlock()
		return nil
	})

	host := module.NewHost()
	if _, exposed := any(host).(module.Resolver); exposed {
		t.Fatal("public Host implements Resolver")
	}
	if err := host.Register(second, postgresModule, first); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, ","); got != "first,second" {
		t.Fatalf("start order = %q", got)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHostContainsPanickingMigrationSourceWithoutFormattingValue(t *testing.T) {
	databaseConfig := testpostgres.New(t)
	postgresModule, err := postgres.Module(postgres.Options{
		URL: databaseConfig.URL, ApplicationSchema: databaseConfig.ApplicationSchema, QueueSchema: databaseConfig.QueueSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	consumer := migrationModule("panic-migration", []module.Capability{module.CapabilityDatabase}, nil, panicMigrationSource{}, nil)
	host := module.NewHost()
	if err := host.Register(postgresModule, consumer); err != nil {
		t.Fatal(err)
	}
	err = host.Start(context.Background())
	if !errors.Is(err, module.ErrCallbackPanic) || !strings.Contains(err.Error(), "migration callback panicked") {
		t.Fatalf("Start() error = %v", err)
	}
	if strings.Contains(err.Error(), "migration-secret") || host.State() != module.StateFailed {
		t.Fatalf("migration panic leaked or escaped cleanup: state=%s error=%v", host.State(), err)
	}
}

func migrationModule(id string, requires, provides []module.Capability, migrations fs.FS, start module.StartFunc) module.Registration {
	return module.Registration{
		Definition: module.Definition{
			Manifest: module.Manifest{
				SchemaVersion: module.SchemaVersion,
				ID:            id,
				Version:       "0.1.0",
				Type:          module.ModuleTypeFeature,
				Requires:      requires,
				Provides:      provides,
			},
			Migrations: []module.MigrationSource{{Driver: "postgres", Files: migrations}},
		},
		Start: start,
	}
}

type panicMigrationSource struct{}

func (panicMigrationSource) Open(string) (fs.File, error) { panic(hostileMigrationPanic{}) }

type hostileMigrationPanic struct{}

func (hostileMigrationPanic) Error() string  { panic("migration-secret-error") }
func (hostileMigrationPanic) String() string { panic("migration-secret-string") }
