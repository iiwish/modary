package databasecontrol

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	. "github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/transactionoutcome"
)

func TestAccessAllowsReadsAndRequiresGovernedTransactionForWrites(t *testing.T) {
	db := openExecutorTestDB(t)
	control := newTestControl(t, db)
	access := control.Access()
	if _, exposed := access.(Control); exposed {
		t.Fatal("safe Access exposes privileged Control")
	}
	if got := reflect.TypeOf(access).String(); got == "*sql.DB" || got == "*sql.Tx" {
		t.Fatalf("safe Access exposes raw SQL type %s", got)
	}
	admin, err := control.Executor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(context.Background(), `CREATE TABLE item (value INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := access.ExecContext(context.Background(), `CREATE TABLE forbidden (value INTEGER NOT NULL)`); !errors.Is(err, ErrMutationStatementRequired) {
		t.Fatalf("public DDL error = %v", err)
	}
	if _, err := access.ExecContext(context.Background(), `INSERT INTO item (value) VALUES (9)`); !errors.Is(err, ErrTransactionRequired) {
		t.Fatalf("write outside transaction error = %v", err)
	}
	if rows, err := access.QueryContext(context.Background(), `INSERT INTO item (value) VALUES (9) RETURNING value`); !errors.Is(err, ErrReadQueryRequired) || rows != nil {
		t.Fatalf("mutation through QueryContext = %#v, %v", rows, err)
	}
	if err := control.WithinTransaction(context.Background(), func(ctx context.Context) error {
		if _, err := access.ExecContext(ctx, `CREATE TABLE forbidden (value INTEGER NOT NULL)`); !errors.Is(err, ErrMutationStatementRequired) {
			return errors.New("public Access accepted DDL")
		}
		_, err := access.ExecContext(ctx, `INSERT INTO item (value) VALUES (1)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := access.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM item`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("read count = %d, %v", count, err)
	}
}

func TestControlCommitsRollsBackNestsAndHidesTransactionControl(t *testing.T) {
	db := openExecutorTestDB(t)
	control := newTestControl(t, db)
	admin, err := control.Executor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(context.Background(), `CREATE TABLE item (value INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := control.WithinTransaction(context.Background(), func(ctx context.Context) error {
		executor, err := control.Executor(ctx)
		if err != nil {
			return err
		}
		if _, exposed := any(executor).(*sql.Tx); exposed {
			t.Fatal("executor exposes commit and rollback control")
		}
		if _, err := executor.ExecContext(ctx, `INSERT INTO item (value) VALUES (1)`); err != nil {
			return err
		}
		return control.WithinTransaction(ctx, func(nested context.Context) error {
			_, err := executor.ExecContext(nested, `INSERT INTO item (value) VALUES (2)`)
			return err
		})
	}); err != nil {
		t.Fatal(err)
	}
	assertExecutorCount(t, db, 2)

	want := errors.New("operation failed")
	if err := control.WithinTransaction(context.Background(), func(ctx context.Context) error {
		executor, err := control.Executor(ctx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `INSERT INTO item (value) VALUES (3)`); err != nil {
			return err
		}
		return want
	}); !errors.Is(err, want) {
		t.Fatalf("rollback error = %v", err)
	}
	assertExecutorCount(t, db, 2)
}

func TestNewRejectsInvalidDependencies(t *testing.T) {
	if control, err := New((*testBackend)(nil)); err == nil || control != nil {
		t.Fatalf("New with typed nil backend = %#v, %v", control, err)
	}
}

func TestControlRejectsInvalidInputs(t *testing.T) {
	control := newTestControl(t, openExecutorTestDB(t))
	if _, err := control.Executor(nil); err == nil {
		t.Fatal("Executor accepted nil context")
	}
	if err := control.WithinTransaction(nil, func(context.Context) error { return nil }); err == nil {
		t.Fatal("WithinTransaction accepted nil context")
	}
	if err := control.WithinTransaction(context.Background(), nil); err == nil {
		t.Fatal("WithinTransaction accepted nil operation")
	}
}

func TestDatabaseDependencyErrorsAreStableAndTypedNilFailsClosed(t *testing.T) {
	db := openExecutorTestDB(t)
	executor := testExecutor{runner: db}
	hostile := &hostileDatabaseError{secret: "database-secret"}

	readControl, err := New(&boundaryBackend{executor: executor, readErr: hostile})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readControl.Access().QueryContext(context.Background(), `SELECT 1`); err == nil {
		t.Fatal("hostile backend read error was ignored")
	} else {
		assertStableDatabaseError(t, err, hostile, "resolve database read executor failed")
	}

	execControl, err := New(&boundaryBackend{
		executor: boundaryExecutor{err: hostile},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execControl.Access().ExecContext(context.Background(), `UPDATE item SET value = 1`); err == nil {
		t.Fatal("hostile executor error was ignored")
	} else {
		assertStableDatabaseError(t, err, hostile, "execute database statement failed")
	}

	transactionControl, err := New(&boundaryBackend{executor: executor})
	if err != nil {
		t.Fatal(err)
	}
	if err := transactionControl.WithinTransaction(context.Background(), func(context.Context) error { return hostile }); err == nil {
		t.Fatal("hostile transaction callback error was ignored")
	} else {
		assertStableDatabaseError(t, err, hostile, "transaction operation was rolled back")
	}
	var typedNil *hostileDatabaseError
	if _, err := db.Exec(`CREATE TABLE item (value INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	rollbackControl, err := New(&testBackend{db: db})
	if err != nil {
		t.Fatal(err)
	}
	typedNilOperationErr := rollbackControl.WithinTransaction(context.Background(), func(ctx context.Context) error {
		if _, err := rollbackControl.Access().ExecContext(ctx, `INSERT INTO item (value) VALUES (1)`); err != nil {
			return err
		}
		return typedNil
	})
	if typedNilOperationErr == nil {
		t.Fatal("typed-nil transaction callback error was ignored")
	}
	assertStableDatabaseError(t, typedNilOperationErr, typedNil, "transaction operation was rolled back")
	assertExecutorCount(t, db, 0)

	backendFailureControl, err := New(&boundaryBackend{executor: executor, transactionErr: hostile})
	if err != nil {
		t.Fatal(err)
	}
	if err := backendFailureControl.WithinTransaction(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("hostile transaction backend error was ignored")
	} else {
		assertStableDatabaseError(t, err, hostile, "run database transaction failed")
	}
	typedNilBackendControl, err := New(&boundaryBackend{executor: executor, transactionErr: typedNil})
	if err != nil {
		t.Fatal(err)
	}
	if err := typedNilBackendControl.WithinTransaction(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("typed-nil transaction backend error was ignored")
	} else {
		assertStableDatabaseError(t, err, typedNil, "run database transaction failed")
	}

	typedNilReadControl, err := New(&boundaryBackend{executor: executor, readErr: typedNil})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := typedNilReadControl.Access().QueryContext(context.Background(), `SELECT 1`); err == nil {
		t.Fatal("typed-nil read backend error was ignored")
	} else {
		assertStableDatabaseError(t, err, typedNil, "resolve database read executor failed")
	}
}

func TestControlContainsBackendPanicsAndUsesValidatedDriver(t *testing.T) {
	db := openExecutorTestDB(t)
	backend := &boundaryBackend{executor: testExecutor{runner: db}}
	control, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}

	backend.panicDriver = true
	if got := control.Driver(); got != "postgres" {
		t.Fatalf("Driver() = %q after backend mutation, want cached validated value", got)
	}

	backend.panicTransaction = true
	err = control.WithinTransaction(context.Background(), func(context.Context) error {
		t.Fatal("transaction callback ran after backend panic")
		return nil
	})
	if err == nil || !IsDependencyPanic(err) {
		t.Fatalf("WithinTransaction backend panic = %v, want ErrDependencyPanic classification", err)
	}
	if got := err.Error(); got != "run database transaction failed" {
		t.Fatalf("WithinTransaction backend panic diagnostic = %q", got)
	}
}

func openExecutorTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return openPostgresTestDB(t)
}

func assertExecutorCount(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM item`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("item count = %d, want %d", got, want)
	}
}

type hostileDatabaseError struct{ secret string }

func (*hostileDatabaseError) Error() string { panic("hostile database Error invoked") }
func (*hostileDatabaseError) Unwrap() error { panic("hostile database Unwrap invoked") }

type boundaryExecutor struct{ err error }

type boundaryErrorRow struct{ err error }

func (row boundaryErrorRow) Scan(...any) error { return row.err }

func (executor boundaryExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, executor.err
}

func (executor boundaryExecutor) QueryContext(context.Context, string, ...any) (Rows, error) {
	return nil, executor.err
}

func (executor boundaryExecutor) QueryRowContext(context.Context, string, ...any) Row {
	return boundaryErrorRow{err: executor.err}
}

type boundaryBackend struct {
	executor         Executor
	readErr          error
	transactionErr   error
	panicDriver      bool
	panicTransaction bool
}

func (backend *boundaryBackend) Driver() string {
	if backend.panicDriver {
		panic("database Driver panic must be contained")
	}
	return "postgres"
}

func (*boundaryBackend) ValidateMigration(string) error { return nil }

func (backend *boundaryBackend) ReadExecutor(context.Context) (Executor, error) {
	return backend.executor, backend.readErr
}

func (backend *boundaryBackend) WriteExecutor(context.Context) (Executor, error) {
	return backend.executor, nil
}

func (backend *boundaryBackend) AdminExecutor(context.Context) (Executor, error) {
	return backend.executor, nil
}

func (backend *boundaryBackend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if backend.panicTransaction {
		panic("database transaction panic must be contained")
	}
	if backend.transactionErr != nil {
		return backend.transactionErr
	}
	if err := operation(ctx); err != nil {
		return transactionoutcome.RolledBack(err)
	}
	return nil
}

func assertStableDatabaseError(t *testing.T, err error, cause error, want string) {
	t.Helper()
	if got := err.Error(); !strings.Contains(got, want) || strings.Contains(got, "secret") {
		t.Fatalf("database boundary error = %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("database dependency cause was not preserved")
	}
}
