package module

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/databasecontrol"
)

var startupReferenceDatabaseKey = MustKey[databasecontrol.Control](
	databasecontrol.ServiceName,
	CapabilityDatabase,
)

func TestHostReleasesStartupReferencesAfterTerminalStartAttempts(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var migrationValidations atomic.Int32
		control, err := databasecontrol.New(startupReferenceBackend{validations: &migrationValidations})
		if err != nil {
			t.Fatal(err)
		}
		source := fstest.MapFS{
			"0001_test.sql": {Data: []byte("CREATE TABLE startup_reference (id INTEGER)")},
		}
		registration := startupReferenceRegistration(
			"startup-success",
			source,
			func(_ context.Context, installation Scope) error {
				return Provide(installation, startupReferenceDatabaseKey, control)
			},
		)
		host := NewHost()
		if err := host.Register(registration); err != nil {
			t.Fatal(err)
		}
		if err := host.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		if calls := migrationValidations.Load(); calls != 1 {
			t.Fatalf("migration source validation calls = %d, want 1", calls)
		}
		assertHostStartupReferencesReleased(t, host, "startup-success")
		assertCallerStartupReferencesRetained(t, registration)
		if catalog, err := host.Catalog(); err != nil || len(catalog) != 1 {
			t.Fatalf("Catalog() after reference release = %#v, %v", catalog, err)
		}
		if err := host.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		startFailure := errors.New("startup failed")
		registration := startupReferenceRegistration(
			"startup-failure",
			fstest.MapFS{"0001_test.sql": {Data: []byte("SELECT 1")}},
			func(context.Context, Scope) error { return startFailure },
		)
		host := NewHost()
		if err := host.Register(registration); err != nil {
			t.Fatal(err)
		}
		if err := host.Start(context.Background()); !errors.Is(err, startFailure) {
			t.Fatalf("Start() error = %v", err)
		}
		assertHostStartupReferencesReleased(t, host, "startup-failure")
		assertCallerStartupReferencesRetained(t, registration)
	})

	t.Run("cancellation", func(t *testing.T) {
		entered := make(chan struct{})
		registration := startupReferenceRegistration(
			"startup-cancel",
			fstest.MapFS{"0001_test.sql": {Data: []byte("SELECT 1")}},
			func(ctx context.Context, _ Scope) error {
				close(entered)
				<-ctx.Done()
				return ctx.Err()
			},
		)
		host := NewHost()
		if err := host.Register(registration); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() { result <- host.Start(ctx) }()
		<-entered
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("Start() cancellation error = %v", err)
		}
		assertHostStartupReferencesReleased(t, host, "startup-cancel")
		assertCallerStartupReferencesRetained(t, registration)
	})
}

func TestNilAndZeroValueHostPublicMethodsFailClosed(t *testing.T) {
	initialized := NewHost()
	hosts := []struct {
		name string
		host *Host
	}{
		{name: "nil", host: nil},
		{name: "zero", host: &Host{}},
		{name: "partially forged", host: &Host{state: StateNew}},
		{
			name: "foreign initialization",
			host: &Host{initialization: initialized.initialization, state: StateNew},
		},
	}
	for _, fixture := range hosts {
		t.Run(fixture.name, func(t *testing.T) {
			if state := fixture.host.State(); state != StateUnavailable {
				t.Fatalf("State() = %q, want %q", state, StateUnavailable)
			}
			if manifests := fixture.host.Manifests(); manifests != nil {
				t.Fatalf("Manifests() = %#v, want nil", manifests)
			}
			if started := fixture.host.StartedModules(); started != nil {
				t.Fatalf("StartedModules() = %#v, want nil", started)
			}

			operations := []struct {
				name string
				call func() error
			}{
				{name: "Register", call: func() error { return fixture.host.Register() }},
				{name: "Start", call: func() error { return fixture.host.Start(context.Background()) }},
				{name: "Shutdown", call: func() error { return fixture.host.Shutdown(context.Background()) }},
				{name: "Catalog", call: func() error {
					_, err := fixture.host.Catalog()
					return err
				}},
				{name: "Assemble", call: func() error {
					_, err := fixture.host.Assemble()
					return err
				}},
			}
			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					defer func() {
						if recovered := recover(); recovered != nil {
							t.Fatalf("%s panicked: %v", operation.name, recovered)
						}
					}()
					if err := operation.call(); !errors.Is(err, ErrHostUnavailable) {
						t.Fatalf("%s error = %v", operation.name, err)
					}
				})
			}

			if err := fixture.host.Start(nil); !errors.Is(err, ErrContextRequired) {
				t.Fatalf("Start(nil) error = %v", err)
			}
			if err := fixture.host.Shutdown(nil); !errors.Is(err, ErrContextRequired) {
				t.Fatalf("Shutdown(nil) error = %v", err)
			}
		})
	}
}

func startupReferenceRegistration(id string, files fs.FS, start StartFunc) Registration {
	return Registration{
		Definition: Definition{
			Manifest: Manifest{
				SchemaVersion: SchemaVersion,
				ID:            id,
				Version:       "0.1.0",
				Type:          ModuleTypeAdapter,
				Provides:      []Capability{CapabilityDatabase},
			},
			Actions: []ActionBinding{{
				Descriptor: testActionDescriptor(id + ".run"),
				NewHandler: func(context.Context, Resolver) (action.Handler, error) {
					return inertActionHandler{}, nil
				},
			}},
			Migrations: []MigrationSource{{Driver: "memory", Files: files}},
		},
		Start: start,
	}
}

func assertHostStartupReferencesReleased(t *testing.T, host *Host, id string) {
	t.Helper()
	host.mu.RLock()
	registration, exists := host.registrations[id]
	host.mu.RUnlock()
	if !exists {
		t.Fatalf("Host registration %q is missing", id)
	}
	if registration.Start != nil {
		t.Fatal("Host retained Start callback")
	}
	if len(registration.Definition.Actions) != 1 ||
		registration.Definition.Actions[0].NewHandler != nil {
		t.Fatalf("Host retained handler factory: %#v", registration.Definition.Actions)
	}
	if len(registration.Definition.Migrations) != 1 ||
		registration.Definition.Migrations[0].Files != nil {
		t.Fatalf("Host retained migration source: %#v", registration.Definition.Migrations)
	}
}

func assertCallerStartupReferencesRetained(t *testing.T, registration Registration) {
	t.Helper()
	if registration.Start == nil ||
		len(registration.Definition.Actions) != 1 ||
		registration.Definition.Actions[0].NewHandler == nil ||
		len(registration.Definition.Migrations) != 1 ||
		registration.Definition.Migrations[0].Files == nil {
		t.Fatal("Host mutated the caller-owned Registration")
	}
}

type startupReferenceBackend struct {
	validations *atomic.Int32
}

func (startupReferenceBackend) Driver() string { return "memory" }

func (backend startupReferenceBackend) ValidateMigration(string) error {
	if backend.validations != nil {
		backend.validations.Add(1)
	}
	return nil
}

func (startupReferenceBackend) ReadExecutor(context.Context) (database.Executor, error) {
	return startupReferenceExecutor{}, nil
}

func (startupReferenceBackend) WriteExecutor(context.Context) (database.Executor, error) {
	return startupReferenceExecutor{}, nil
}

func (startupReferenceBackend) AdminExecutor(context.Context) (database.Executor, error) {
	return startupReferenceExecutor{}, nil
}

func (startupReferenceBackend) WithinTransaction(
	ctx context.Context,
	operation func(context.Context) error,
) error {
	return operation(ctx)
}

type startupReferenceExecutor struct{}

func (startupReferenceExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return startupReferenceResult(0), nil
}

func (startupReferenceExecutor) QueryContext(context.Context, string, ...any) (database.Rows, error) {
	return startupReferenceRows{}, nil
}

func (startupReferenceExecutor) QueryRowContext(context.Context, string, ...any) database.Row {
	return startupReferenceRow{}
}

type startupReferenceResult int64

func (result startupReferenceResult) LastInsertId() (int64, error) { return int64(result), nil }
func (result startupReferenceResult) RowsAffected() (int64, error) { return int64(result), nil }

type startupReferenceRows struct{}

func (startupReferenceRows) Next() bool                 { return false }
func (startupReferenceRows) Scan(...any) error          { return sql.ErrNoRows }
func (startupReferenceRows) Err() error                 { return nil }
func (startupReferenceRows) Columns() ([]string, error) { return nil, nil }
func (startupReferenceRows) Close() error               { return nil }

type startupReferenceRow struct{}

func (startupReferenceRow) Scan(...any) error { return sql.ErrNoRows }
