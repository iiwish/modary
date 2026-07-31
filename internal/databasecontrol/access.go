package databasecontrol

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/sqlpolicy"
)

const maxPublicStatementBytes = 1 << 20

type access struct {
	backend Backend
}

type errorRow struct{ err error }

type guardedRow struct{ row database.Row }

type guardedResult struct{ result sql.Result }

type guardedRows struct {
	rows database.Rows

	mu       sync.Mutex
	terminal error
	closed   bool
	closeErr error
}

func (row errorRow) Scan(...any) error { return row.err }

func (row guardedRow) Scan(destinations ...any) error {
	return invokeDependencyError("scan database row", func() error {
		return row.row.Scan(destinations...)
	})
}

func (rows *guardedRows) Next() bool {
	if rows == nil {
		return false
	}
	rows.mu.Lock()
	defer rows.mu.Unlock()
	if rows.closed || rows.terminal != nil || isNil(rows.rows) {
		return false
	}
	next, err := invokeDependency("advance database rows", func() (bool, error) {
		return rows.rows.Next(), nil
	})
	if err != nil {
		rows.terminal = err
		return false
	}
	return next
}

func (rows *guardedRows) Scan(destinations ...any) error {
	if rows == nil {
		return database.ErrAccessUnavailable
	}
	rows.mu.Lock()
	defer rows.mu.Unlock()
	if rows.terminal != nil {
		return rows.terminal
	}
	if rows.closed {
		return database.ErrRowsClosed
	}
	err := invokeDependencyError("scan database rows", func() error {
		return rows.rows.Scan(destinations...)
	})
	if database.IsDependencyPanic(err) {
		rows.terminal = err
	}
	return err
}

func (rows *guardedRows) Err() error {
	if rows == nil {
		return database.ErrAccessUnavailable
	}
	rows.mu.Lock()
	defer rows.mu.Unlock()
	if rows.terminal != nil {
		return rows.terminal
	}
	if rows.closeErr != nil {
		return rows.closeErr
	}
	err := invokeDependencyError("read database rows error", rows.rows.Err)
	if err != nil {
		rows.terminal = err
	}
	return err
}

func (rows *guardedRows) Columns() ([]string, error) {
	if rows == nil {
		return nil, database.ErrAccessUnavailable
	}
	rows.mu.Lock()
	defer rows.mu.Unlock()
	if rows.terminal != nil {
		return nil, rows.terminal
	}
	if rows.closed {
		return nil, database.ErrRowsClosed
	}
	columns, err := invokeDependency("read database row columns", rows.rows.Columns)
	if err != nil {
		if database.IsDependencyPanic(err) {
			rows.terminal = err
		}
		return nil, err
	}
	return append([]string(nil), columns...), nil
}

func (rows *guardedRows) Close() error {
	if rows == nil {
		return database.ErrAccessUnavailable
	}
	rows.mu.Lock()
	defer rows.mu.Unlock()
	if rows.closed {
		return rows.closeErr
	}
	rows.closed = true
	rows.closeErr = invokeDependencyError("close database rows", rows.rows.Close)
	return rows.closeErr
}

func (result guardedResult) LastInsertId() (int64, error) {
	return invokeDependency("read database last insert id", result.result.LastInsertId)
}

func (result guardedResult) RowsAffected() (int64, error) {
	return invokeDependency("read database affected row count", result.result.RowsAffected)
}

func (access *access) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if err := validateMutationStatement(query); err != nil {
		return nil, err
	}
	executor, err := access.writeExecutor(ctx)
	if err != nil {
		return nil, err
	}
	result, err := invokeDependency("execute database statement", func() (sql.Result, error) {
		return executor.ExecContext(ctx, query, args...)
	})
	if err != nil {
		return nil, err
	}
	if isNil(result) {
		return nil, fmt.Errorf("%w: database executor returned no result", database.ErrAccessUnavailable)
	}
	return guardedResult{result: result}, nil
}

func (access *access) QueryContext(ctx context.Context, query string, args ...any) (database.Rows, error) {
	if err := validateReadQuery(query); err != nil {
		return nil, err
	}
	executor, err := access.readExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := invokeDependency("query database rows", func() (database.Rows, error) {
		return executor.QueryContext(ctx, query, args...)
	})
	if err != nil {
		return nil, err
	}
	if isNil(rows) {
		return nil, database.ErrAccessUnavailable
	}
	return &guardedRows{rows: rows}, nil
}

func (access *access) QueryRowContext(ctx context.Context, query string, args ...any) database.Row {
	if err := validateReadQuery(query); err != nil {
		return errorRow{err: err}
	}
	executor, err := access.readExecutor(ctx)
	if err != nil {
		return errorRow{err: err}
	}
	row, err := invokeDependency("query database row", func() (database.Row, error) {
		return executor.QueryRowContext(ctx, query, args...), nil
	})
	if err != nil {
		return errorRow{err: err}
	}
	if isNil(row) {
		return errorRow{err: database.ErrAccessUnavailable}
	}
	return guardedRow{row: row}
}

func validateReadQuery(query string) error {
	return validatePublicStatement(query, []string{"SELECT"}, "", database.ErrReadQueryRequired)
}

func validateMutationStatement(query string) error {
	return validatePublicStatement(
		query,
		[]string{"INSERT", "UPDATE", "DELETE"},
		"ROLLBACK",
		database.ErrMutationStatementRequired,
	)
}

func validatePublicStatement(query string, allowed []string, forbiddenWord string, invalid error) error {
	tokens, err := sqlpolicy.Tokenize(query, maxPublicStatementBytes)
	if err != nil {
		return invalid
	}
	if sqlpolicy.HasTemporarySchemaReference(tokens) {
		return invalid
	}
	statements := 0
	inStatement := false
	for _, token := range tokens {
		if token.Separator {
			if !inStatement {
				return invalid
			}
			statements++
			inStatement = false
			continue
		}
		if !inStatement {
			if statements != 0 || token.Word == "" || !containsFold(allowed, token.Word) {
				return invalid
			}
			inStatement = true
		}
		if forbiddenWord != "" && strings.EqualFold(token.Word, forbiddenWord) {
			return invalid
		}
	}
	if inStatement {
		statements++
	}
	if statements != 1 {
		return invalid
	}
	return nil
}

func containsFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

func (access *access) readExecutor(ctx context.Context) (database.Executor, error) {
	if ctx == nil {
		return nil, fmt.Errorf("database context is required")
	}
	if access == nil || isNil(access.backend) {
		return nil, database.ErrAccessUnavailable
	}
	executor, err := invokeDependency("resolve database read executor", func() (database.Executor, error) {
		return access.backend.ReadExecutor(ctx)
	})
	if err != nil {
		return nil, err
	}
	if isNil(executor) {
		return nil, database.ErrAccessUnavailable
	}
	return executor, nil
}

func (access *access) writeExecutor(ctx context.Context) (database.Executor, error) {
	if ctx == nil {
		return nil, fmt.Errorf("database context is required")
	}
	if access == nil || isNil(access.backend) {
		return nil, database.ErrAccessUnavailable
	}
	executor, err := invokeDependency("resolve database write executor", func() (database.Executor, error) {
		return access.backend.WriteExecutor(ctx)
	})
	if err != nil {
		return nil, err
	}
	if isNil(executor) {
		return nil, database.ErrAccessUnavailable
	}
	return executor, nil
}
