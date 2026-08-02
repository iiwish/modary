package databasecontrol

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	. "github.com/iiwish/modary/database"
)

func TestAccessExecContextAcceptsOnlySingleMutationStatements(t *testing.T) {
	executor := &recordingExecutor{result: fixedSQLResult(1)}
	access := newAccessForTest(t, fixedBackend{executor: executor})

	for _, statement := range []string{
		"INSERT INTO item(value) VALUES ($1)",
		"insert into item(value) values ($1) on conflict do nothing",
		"UPDATE item SET value = $1",
		"delete FROM item WHERE value = $1",
		"/* one public mutation */ UPDATE item SET value = 1",
		"UPDATE item SET value = 'ROLLBACK; COMMIT' -- inert SQL text",
	} {
		if _, err := access.ExecContext(context.Background(), statement, 1); err != nil {
			t.Fatalf("ExecContext(%q) error = %v", statement, err)
		}
	}
	if executor.execCalls != 6 {
		t.Fatalf("executor calls = %d, want 6", executor.execCalls)
	}

	for _, statement := range []string{
		"",
		"SELECT 1",
		"CREATE TABLE item(value INTEGER)",
		"DROP TABLE item",
		"COPY item FROM STDIN",
		"SET search_path = public",
		"BEGIN",
		"COMMIT",
		"END TRANSACTION",
		"ROLLBACK",
		"SAVEPOINT nested",
		"RELEASE nested",
		"WITH value AS (SELECT 1) UPDATE item SET value = 1",
		"TRUNCATE item",
		"MERGE INTO item USING source ON false WHEN NOT MATCHED THEN INSERT VALUES (1)",
		"UPDATE item SET value = 1; COMMIT",
		"UPDATE item SET value = 1; DELETE FROM item",
		"/* unterminated UPDATE item SET value = 1",
		"UPDATE item SET value = 'unterminated",
		"UPDATE\x00item SET value = 1",
		string([]byte{'U', 'P', 'D', 'A', 'T', 'E', ' ', 0xff}),
		strings.Repeat(" ", maxPublicStatementBytes) + "UPDATE item SET value = 1",
	} {
		before := executor.execCalls
		result, err := access.ExecContext(context.Background(), statement)
		if !errors.Is(err, ErrMutationStatementRequired) || result != nil {
			t.Fatalf("ExecContext(%q) = %#v, %v", statement, result, err)
		}
		if executor.execCalls != before {
			t.Fatalf("invalid statement %q reached executor", statement)
		}
	}
}

func TestAccessExecContextGuardsSQLResultBoundary(t *testing.T) {
	var typedNil *hostileSQLResult
	executor := &recordingExecutor{result: typedNil}
	access := newAccessForTest(t, fixedBackend{executor: executor})
	result, err := access.ExecContext(context.Background(), "UPDATE item SET value = 1")
	if !errors.Is(err, ErrAccessUnavailable) || result != nil {
		t.Fatalf("typed-nil result = %#v, %v", result, err)
	}

	executor.result = &hostileSQLResult{}
	result, err = access.ExecContext(context.Background(), "UPDATE item SET value = 1")
	if err != nil || result == nil {
		t.Fatalf("guarded result = %#v, %v", result, err)
	}
	if _, err := result.LastInsertId(); err == nil || !IsDependencyPanic(err) {
		t.Fatalf("LastInsertId panic boundary = %v", err)
	}
	if _, err := result.RowsAffected(); err == nil || !IsDependencyPanic(err) {
		t.Fatalf("RowsAffected panic boundary = %v", err)
	}
}

func TestAccessExecContextRejectsBeforeResolvingWriteExecutor(t *testing.T) {
	backend := &panickingWriteBackend{}
	access := newAccessForTest(t, backend)
	result, err := access.ExecContext(context.Background(), "CREATE TABLE item(value INTEGER)")
	if result != nil || !errors.Is(err, ErrMutationStatementRequired) {
		t.Fatalf("invalid mutation = %#v, %v", result, err)
	}
	if backend.writeCalls != 0 {
		t.Fatalf("invalid mutation resolved write executor %d times", backend.writeCalls)
	}
}

func TestAccessRejectsTemporarySchemaBeforeResolvingExecutors(t *testing.T) {
	for _, test := range []struct {
		name      string
		qualifier string
	}{
		{name: "unquoted", qualifier: "temp"},
		{name: "double_quoted", qualifier: `"temp"`},
		{name: "single_quoted", qualifier: `'temp'`},
		{name: "backtick_quoted", qualifier: "`temp`"},
		{name: "bracket_quoted", qualifier: `[temp]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := &recordingExecutor{result: fixedSQLResult(1)}
			backend := &recordingResolverBackend{
				fixedBackend: fixedBackend{executor: executor},
			}
			access := &access{backend: backend}

			rows, err := access.QueryContext(context.Background(), "SELECT value FROM "+test.qualifier+".item")
			if rows != nil || !errors.Is(err, ErrReadQueryRequired) {
				t.Fatalf("temporary schema QueryContext = %#v, %v", rows, err)
			}
			if err := access.QueryRowContext(context.Background(), "SELECT value FROM "+test.qualifier+".item").Scan(new(int)); !errors.Is(err, ErrReadQueryRequired) {
				t.Fatalf("temporary schema QueryRowContext error = %v", err)
			}
			result, err := access.ExecContext(context.Background(), "UPDATE "+test.qualifier+".item SET value = 1")
			if result != nil || !errors.Is(err, ErrMutationStatementRequired) {
				t.Fatalf("temporary schema ExecContext = %#v, %v", result, err)
			}
			if backend.readCalls != 0 || backend.writeCalls != 0 {
				t.Fatalf("temporary schema resolved executors: read=%d write=%d", backend.readCalls, backend.writeCalls)
			}
			if executor.queryCalls != 0 || executor.queryRowCalls != 0 || executor.execCalls != 0 {
				t.Fatalf(
					"temporary schema reached executor: query=%d query-row=%d exec=%d",
					executor.queryCalls,
					executor.queryRowCalls,
					executor.execCalls,
				)
			}
		})
	}
}

func TestAccessQueryContextGuardsRowsLifecycle(t *testing.T) {
	executor := &recordingExecutor{}
	access := newAccessForTest(t, fixedBackend{executor: executor})

	var typedNil *callbackRows
	executor.rows = typedNil
	if rows, err := access.QueryContext(context.Background(), "SELECT value FROM item"); rows != nil || !errors.Is(err, ErrAccessUnavailable) {
		t.Fatalf("typed-nil rows = %#v, %v", rows, err)
	}

	columns := []string{"value"}
	closeCalls := 0
	executor.rows = &callbackRows{
		next: func() bool { return true },
		scan: func(destinations ...any) error {
			*destinations[0].(*int) = 7
			return nil
		},
		columns: func() ([]string, error) { return columns, nil },
		close:   func() error { closeCalls++; return nil },
	}
	rows, err := access.QueryContext(context.Background(), "SELECT value FROM item")
	if err != nil || rows == nil {
		t.Fatalf("guarded rows = %#v, %v", rows, err)
	}
	gotColumns, err := rows.Columns()
	if err != nil || len(gotColumns) != 1 || gotColumns[0] != "value" {
		t.Fatalf("Columns() = %#v, %v", gotColumns, err)
	}
	gotColumns[0] = "mutated"
	if columns[0] != "value" {
		t.Fatalf("Columns exposed dependency slice: %#v", columns)
	}
	if !rows.Next() {
		t.Fatal("Next() = false")
	}
	var value int
	if err := rows.Scan(&value); err != nil || value != 7 {
		t.Fatalf("Scan() value = %d, %v", value, err)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if err := rows.Close(); err != nil || closeCalls != 1 {
		t.Fatalf("repeated Close() = %v, calls=%d", err, closeCalls)
	}
	if rows.Next() {
		t.Fatal("closed rows advanced")
	}
	if err := rows.Scan(&value); !errors.Is(err, ErrRowsClosed) {
		t.Fatalf("closed Scan() error = %v", err)
	}
}

func TestAccessQueryContextContainsEveryRowsPanic(t *testing.T) {
	operations := []struct {
		name string
		rows *callbackRows
		call func(Rows) error
	}{
		{
			name: "next",
			rows: &callbackRows{next: func() bool { panic("next secret") }},
			call: func(rows Rows) error {
				if rows.Next() {
					return errors.New("panicking Next returned true")
				}
				return rows.Err()
			},
		},
		{
			name: "scan",
			rows: &callbackRows{scan: func(...any) error { panic("scan secret") }},
			call: func(rows Rows) error { return rows.Scan(new(int)) },
		},
		{
			name: "error",
			rows: &callbackRows{iterationErr: func() error { panic("error secret") }},
			call: func(rows Rows) error { return rows.Err() },
		},
		{
			name: "columns",
			rows: &callbackRows{columns: func() ([]string, error) { panic("columns secret") }},
			call: func(rows Rows) error { _, err := rows.Columns(); return err },
		},
		{
			name: "close",
			rows: &callbackRows{close: func() error { panic("close secret") }},
			call: func(rows Rows) error { return rows.Close() },
		},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			executor := &recordingExecutor{rows: operation.rows}
			access := newAccessForTest(t, fixedBackend{executor: executor})
			rows, err := access.QueryContext(context.Background(), "SELECT value FROM item")
			if err != nil {
				t.Fatal(err)
			}
			err = operation.call(rows)
			if err == nil || !IsDependencyPanic(err) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("contained %s panic = %v", operation.name, err)
			}
		})
	}
}

type fixedBackend struct{ executor Executor }

type panickingWriteBackend struct {
	fixedBackend
	writeCalls int
}

type recordingResolverBackend struct {
	fixedBackend
	readCalls  int
	writeCalls int
}

func newAccessForTest(t testing.TB, backend Backend) Access {
	t.Helper()
	control, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	return control.Access()
}

func (fixedBackend) Driver() string { return "access-test" }

func (fixedBackend) ValidateMigration(string) error { return nil }

func (backend fixedBackend) ReadExecutor(context.Context) (Executor, error) {
	return backend.executor, nil
}

func (backend fixedBackend) WriteExecutor(context.Context) (Executor, error) {
	return backend.executor, nil
}

func (backend fixedBackend) AdminExecutor(context.Context) (Executor, error) {
	return backend.executor, nil
}

func (fixedBackend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

func (backend *panickingWriteBackend) WriteExecutor(context.Context) (Executor, error) {
	backend.writeCalls++
	panic("invalid SQL crossed the access boundary")
}

func (backend *recordingResolverBackend) ReadExecutor(context.Context) (Executor, error) {
	backend.readCalls++
	return backend.executor, nil
}

func (backend *recordingResolverBackend) WriteExecutor(context.Context) (Executor, error) {
	backend.writeCalls++
	return backend.executor, nil
}

type recordingExecutor struct {
	result        sql.Result
	rows          Rows
	execCalls     int
	queryCalls    int
	queryRowCalls int
}

func (executor *recordingExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	executor.execCalls++
	return executor.result, nil
}

func (executor *recordingExecutor) QueryContext(context.Context, string, ...any) (Rows, error) {
	executor.queryCalls++
	return executor.rows, nil
}

func (executor *recordingExecutor) QueryRowContext(context.Context, string, ...any) Row {
	executor.queryRowCalls++
	return errorRow{err: errors.New("unexpected QueryRowContext call")}
}

type fixedSQLResult int64

func (result fixedSQLResult) LastInsertId() (int64, error) { return int64(result), nil }
func (result fixedSQLResult) RowsAffected() (int64, error) { return int64(result), nil }

type hostileSQLResult struct{}

func (*hostileSQLResult) LastInsertId() (int64, error) { panic("hostile LastInsertId") }
func (*hostileSQLResult) RowsAffected() (int64, error) { panic("hostile RowsAffected") }

type callbackRows struct {
	next         func() bool
	scan         func(...any) error
	iterationErr func() error
	columns      func() ([]string, error)
	close        func() error
}

func (rows *callbackRows) Next() bool {
	if rows.next == nil {
		return false
	}
	return rows.next()
}

func (rows *callbackRows) Scan(destinations ...any) error {
	if rows.scan == nil {
		return nil
	}
	return rows.scan(destinations...)
}

func (rows *callbackRows) Err() error {
	if rows.iterationErr == nil {
		return nil
	}
	return rows.iterationErr()
}

func (rows *callbackRows) Columns() ([]string, error) {
	if rows.columns == nil {
		return nil, nil
	}
	return rows.columns()
}

func (rows *callbackRows) Close() error {
	if rows.close == nil {
		return nil
	}
	return rows.close()
}
