package databasecontrol

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	. "github.com/iiwish/modary/database"
)

func TestAccessValidatesStatementsAndRequiresTransactionForWrites(t *testing.T) {
	backend := &unitBackend{executor: unitExecutor{row: unitRow{value: 1}}}
	control, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	access := control.Access()
	if _, exposed := access.(Control); exposed {
		t.Fatal("safe Access exposes privileged Control")
	}
	if got := reflect.TypeOf(access).String(); got == "*sql.DB" || got == "*sql.Tx" {
		t.Fatalf("safe Access exposes raw SQL type %s", got)
	}
	if _, err := access.ExecContext(context.Background(), `CREATE TABLE forbidden (value INTEGER)`); !errors.Is(err, ErrMutationStatementRequired) {
		t.Fatalf("public DDL error = %v", err)
	}
	if _, err := access.ExecContext(context.Background(), `INSERT INTO item(value) VALUES (1)`); !errors.Is(err, ErrTransactionRequired) {
		t.Fatalf("write outside transaction error = %v", err)
	}
	if rows, err := access.QueryContext(context.Background(), `DELETE FROM item RETURNING value`); !errors.Is(err, ErrReadQueryRequired) || rows != nil {
		t.Fatalf("mutation through QueryContext = %#v, %v", rows, err)
	}
	if err := control.WithinTransaction(context.Background(), func(ctx context.Context) error {
		_, err := access.ExecContext(ctx, `INSERT INTO item(value) VALUES (1)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var value int
	if err := access.QueryRowContext(context.Background(), `SELECT value FROM item`).Scan(&value); err != nil || value != 1 {
		t.Fatalf("read value = %d, %v", value, err)
	}
}

func TestControlRejectsInvalidDependenciesAndInputs(t *testing.T) {
	if control, err := New((*unitBackend)(nil)); err == nil || control != nil {
		t.Fatalf("New with typed nil backend = %#v, %v", control, err)
	}
	control, err := New(&unitBackend{executor: unitExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
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

func TestDatabaseDependencyErrorsRemainOpaqueAndPanicsAreClassified(t *testing.T) {
	hostile := &hostileDatabaseError{}
	control, err := New(&unitBackend{executor: unitExecutor{}, readErr: hostile})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.Access().QueryContext(context.Background(), `SELECT 1`); err == nil || err.Error() != "resolve database read executor failed" {
		t.Fatalf("read dependency error = %v", err)
	}

	backend := &unitBackend{executor: unitExecutor{}, panicTransaction: true}
	control, err = New(backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := control.WithinTransaction(context.Background(), func(context.Context) error {
		t.Fatal("operation ran after backend panic")
		return nil
	}); err == nil || !IsDependencyPanic(err) || err.Error() != "run database transaction failed" {
		t.Fatalf("transaction panic = %v", err)
	}
}

type unitBackend struct {
	executor         Executor
	readErr          error
	panicTransaction bool
}

type unitTransactionKey struct{}

func (*unitBackend) Driver() string                 { return "test" }
func (*unitBackend) ValidateMigration(string) error { return nil }
func (backend *unitBackend) ReadExecutor(context.Context) (Executor, error) {
	return backend.executor, backend.readErr
}
func (backend *unitBackend) WriteExecutor(ctx context.Context) (Executor, error) {
	if ctx.Value(unitTransactionKey{}) == nil {
		return nil, ErrTransactionRequired
	}
	return backend.executor, nil
}
func (backend *unitBackend) AdminExecutor(context.Context) (Executor, error) {
	return backend.executor, nil
}
func (backend *unitBackend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if backend.panicTransaction {
		panic("database transaction panic")
	}
	return operation(context.WithValue(ctx, unitTransactionKey{}, true))
}

type unitExecutor struct {
	row unitRow
}

func (unitExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return unitResult(1), nil
}
func (unitExecutor) QueryContext(context.Context, string, ...any) (Rows, error) {
	return &unitRows{}, nil
}
func (executor unitExecutor) QueryRowContext(context.Context, string, ...any) Row {
	return executor.row
}

type unitResult int64

func (result unitResult) LastInsertId() (int64, error) { return int64(result), nil }
func (result unitResult) RowsAffected() (int64, error) { return int64(result), nil }

type unitRow struct{ value int }

func (row unitRow) Scan(destinations ...any) error {
	if len(destinations) == 1 {
		if target, ok := destinations[0].(*int); ok {
			*target = row.value
			return nil
		}
	}
	return errors.New("unexpected scan")
}

type unitRows struct{}

func (*unitRows) Next() bool                 { return false }
func (*unitRows) Scan(...any) error          { return nil }
func (*unitRows) Err() error                 { return nil }
func (*unitRows) Columns() ([]string, error) { return []string{"value"}, nil }
func (*unitRows) Close() error               { return nil }

type hostileDatabaseError struct{}

func (*hostileDatabaseError) Error() string { panic("hostile database Error invoked") }
func (*hostileDatabaseError) Unwrap() error { panic("hostile database Unwrap invoked") }
