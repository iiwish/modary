package databasecontrol

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	. "github.com/iiwish/modary/database"
)

type typedNilDatabaseError struct{}

func (*typedNilDatabaseError) Error() string { panic("typed-nil Error must not be called") }

func databaseTypedNilError() error {
	var err *typedNilDatabaseError
	return err
}

func TestDatabaseDependencyTypedNilErrorsFailClosed(t *testing.T) {
	cause := databaseTypedNilError()
	result, err := invokeDependency("test typed-nil dependency", func() (string, error) {
		return "untrusted-result", cause
	})
	if result != "untrusted-result" || err == nil {
		t.Fatalf("invokeDependency() = %q, %v", result, err)
	}
	if got := err.Error(); got != "test typed-nil dependency failed" {
		t.Fatalf("stable wrapper error = %q", got)
	}
	var found *typedNilDatabaseError
	if !errors.Is(err, cause) || !errors.As(err, &found) || found != nil {
		t.Fatalf("typed-nil cause was not safely preserved: Is=%t As=%t value=%#v", errors.Is(err, cause), errors.As(err, &found), found)
	}

	backend := &typedNilBackend{readErr: databaseTypedNilError()}
	access := newAccessForTest(t, backend)
	if _, err := access.QueryContext(context.Background(), "SELECT 1"); err == nil || !strings.Contains(err.Error(), "resolve database read executor failed") {
		t.Fatalf("typed-nil ReadExecutor error = %v", err)
	}

	backend.readErr = nil
	backend.executor = typedNilExecutor{queryErr: databaseTypedNilError()}
	if _, err := access.QueryContext(context.Background(), "SELECT 1"); err == nil || !strings.Contains(err.Error(), "query database rows failed") {
		t.Fatalf("typed-nil QueryContext error = %v", err)
	}

	backend.executor = typedNilExecutor{execErr: databaseTypedNilError()}
	if _, err := access.ExecContext(context.Background(), "UPDATE record SET value = 1"); err == nil || !strings.Contains(err.Error(), "execute database statement failed") {
		t.Fatalf("typed-nil ExecContext error = %v", err)
	}

	backend.executor = typedNilExecutor{row: typedNilRow{}}
	if err := access.QueryRowContext(context.Background(), "SELECT 1").Scan(new(int)); err == nil || !strings.Contains(err.Error(), "scan database row failed") {
		t.Fatalf("typed-nil Scan error = %v", err)
	}
}

type typedNilBackend struct {
	fixedBackend
	executor Executor
	readErr  error
}

func (backend *typedNilBackend) ReadExecutor(context.Context) (Executor, error) {
	return backend.executor, backend.readErr
}

func (backend *typedNilBackend) WriteExecutor(context.Context) (Executor, error) {
	return backend.executor, nil
}

type typedNilExecutor struct {
	execErr  error
	queryErr error
	row      Row
}

func (executor typedNilExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, executor.execErr
}

func (executor typedNilExecutor) QueryContext(context.Context, string, ...any) (Rows, error) {
	return nil, executor.queryErr
}

func (executor typedNilExecutor) QueryRowContext(context.Context, string, ...any) Row {
	return executor.row
}

type typedNilRow struct{}

func (typedNilRow) Scan(...any) error { return databaseTypedNilError() }
