// Package sqlitetest provides privileged SQLite control for adapter tests.
// It is internal to Modary's adapter tree and is not a consumer API.
package sqlitetest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/sqlpolicy"
	"github.com/iiwish/modary/internal/transactionoutcome"
)

type runner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type executor struct{ runner runner }

// ExecContext delegates to the test SQL runner.
func (executor executor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return executor.runner.ExecContext(ctx, query, args...)
}

// QueryContext delegates to the test SQL runner.
func (executor executor) QueryContext(ctx context.Context, query string, args ...any) (database.Rows, error) {
	return executor.runner.QueryContext(ctx, query, args...)
}

// QueryRowContext delegates to the test SQL runner.
func (executor executor) QueryRowContext(ctx context.Context, query string, args ...any) database.Row {
	return executor.runner.QueryRowContext(ctx, query, args...)
}

type backend struct{ db *sql.DB }
type transactionKey struct{}
type transaction struct {
	backend *backend
	tx      *sql.Tx
}

// Driver returns the SQLite test backend name.
func (*backend) Driver() string { return "sqlite" }

func (*backend) ValidateMigration(script string) error {
	return sqlpolicy.ValidateMigrationScript(script, 1<<20)
}

// ReadExecutor selects the test read executor.
func (backend *backend) ReadExecutor(ctx context.Context) (database.Executor, error) {
	if tx, present := ctx.Value(transactionKey{}).(*transaction); present {
		if tx == nil || tx.backend != backend || tx.tx == nil {
			return nil, fmt.Errorf("transaction belongs to another database")
		}
		return executor{runner: tx.tx}, nil
	}
	return executor{runner: backend.db}, nil
}

// WriteExecutor requires a test transaction binding.
func (backend *backend) WriteExecutor(ctx context.Context) (database.Executor, error) {
	tx, present := ctx.Value(transactionKey{}).(*transaction)
	if !present {
		return nil, database.ErrTransactionRequired
	}
	if tx == nil || tx.backend != backend || tx.tx == nil {
		return nil, fmt.Errorf("transaction belongs to another database: %w", database.ErrTransactionRequired)
	}
	return executor{runner: tx.tx}, nil
}

// AdminExecutor selects privileged test execution.
func (backend *backend) AdminExecutor(ctx context.Context) (database.Executor, error) {
	if tx, present := ctx.Value(transactionKey{}).(*transaction); present {
		if tx == nil || tx.backend != backend || tx.tx == nil {
			return nil, fmt.Errorf("transaction belongs to another database")
		}
		return executor{runner: tx.tx}, nil
	}
	return executor{runner: backend.db}, nil
}

// WithinTransaction owns the test transaction boundary.
func (backend *backend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if current, present := ctx.Value(transactionKey{}).(*transaction); present {
		if current == nil || current.backend != backend || current.tx == nil {
			return fmt.Errorf("transaction belongs to another database")
		}
		operationErr := operation(ctx)
		if operationErr != nil {
			return transactionoutcome.RollbackPending(operationErr)
		}
		return nil
	}
	tx, err := backend.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	txCtx := context.WithValue(ctx, transactionKey{}, &transaction{backend: backend, tx: tx})
	if err := operation(txCtx); err != nil {
		rollbackErr := tx.Rollback()
		if errors.Is(rollbackErr, sql.ErrTxDone) {
			rollbackErr = nil
		}
		if rollbackErr != nil {
			return transactionoutcome.RollbackFailed(err, rollbackErr)
		}
		return transactionoutcome.RolledBack(err)
	}
	if err := tx.Commit(); err != nil {
		return transactionoutcome.CommitFailed(err)
	}
	return nil
}

// NewControl wraps db with the same authority boundary used by official
// adapters. The caller retains ownership of db.
func NewControl(db *sql.DB) (databasecontrol.Control, error) {
	if db == nil {
		return nil, fmt.Errorf("SQLite test database is required")
	}
	return databasecontrol.New(&backend{db: db})
}
