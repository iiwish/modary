package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/safeerr"
	"github.com/iiwish/modary/internal/transactionoutcome"
	moderncsqlite "modernc.org/sqlite"
)

type sqlRunner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type sqlExecutor struct {
	runner   sqlRunner
	boundary *transactionBoundary
}

// ExecContext delegates a statement to the wrapped SQL runner.
func (executor sqlExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if err := executor.boundaryError(); err != nil {
		return nil, err
	}
	result, err := executor.runner.ExecContext(ctx, query, args...)
	if boundaryErr := executor.boundaryError(); boundaryErr != nil {
		return nil, errors.Join(err, boundaryErr)
	}
	return result, err
}

// QueryContext delegates a query to the wrapped SQL runner.
func (executor sqlExecutor) QueryContext(ctx context.Context, query string, args ...any) (database.Rows, error) {
	if err := executor.boundaryError(); err != nil {
		return nil, err
	}
	rows, err := executor.runner.QueryContext(ctx, query, args...)
	if boundaryErr := executor.boundaryError(); boundaryErr != nil {
		if rows != nil {
			_ = rows.Close()
		}
		return nil, errors.Join(err, boundaryErr)
	}
	return rows, err
}

// QueryRowContext delegates a single-row query to the wrapped SQL runner.
func (executor sqlExecutor) QueryRowContext(ctx context.Context, query string, args ...any) database.Row {
	if err := executor.boundaryError(); err != nil {
		return transactionRow{boundaryErr: err}
	}
	return transactionRow{
		row:      executor.runner.QueryRowContext(ctx, query, args...),
		boundary: executor.boundary,
	}
}

func (executor sqlExecutor) boundaryError() error {
	if executor.boundary == nil {
		return nil
	}
	return executor.boundary.failure()
}

type backend struct{ db *sql.DB }

type transactionKey struct{}

type transactionBinding struct {
	backend  *backend
	executor database.Executor
	tx       *sql.Tx

	rollbackMu    sync.Mutex
	rollbackCause error
}

type transactionBoundary struct {
	frameworkEnd atomic.Bool

	mu               sync.Mutex
	cause            error
	rollbackObserved bool
}

type transactionRow struct {
	row         *sql.Row
	boundary    *transactionBoundary
	boundaryErr error
}

type transactionHookRegisterer interface {
	RegisterCommitHook(moderncsqlite.CommitHookFn)
	RegisterRollbackHook(moderncsqlite.RollbackHookFn)
}

var (
	errNestedTransactionRollback = errors.New("SQLite transaction was marked rollback-only")
	errNestedTransactionPanic    = errors.New("nested SQLite transaction operation panicked")
	errTransactionBoundaryEnded  = errors.New("SQLite transaction boundary was ended by SQL")
	errUnexpectedSQLCommit       = errors.New("SQL attempted to commit the framework-owned SQLite transaction")
	errUnexpectedSQLRollback     = errors.New("SQL rolled back the framework-owned SQLite transaction")
)

func (row transactionRow) Scan(destinations ...any) error {
	if row.boundaryErr != nil {
		return row.boundaryErr
	}
	if row.row == nil {
		return database.ErrAccessUnavailable
	}
	if row.boundary != nil {
		if err := row.boundary.failure(); err != nil {
			return err
		}
	}
	err := row.row.Scan(destinations...)
	if row.boundary != nil {
		if boundaryErr := row.boundary.failure(); boundaryErr != nil {
			return errors.Join(err, boundaryErr)
		}
	}
	return err
}

// Driver returns the canonical SQLite migration driver name.
func (*backend) Driver() string { return "sqlite" }

// ReadExecutor selects a transaction executor or the pooled read connection.
func (backend *backend) ReadExecutor(ctx context.Context) (database.Executor, error) {
	if ctx == nil {
		return nil, fmt.Errorf("SQLite context is required")
	}
	if backend == nil || backend.db == nil {
		return nil, fmt.Errorf("SQLite database is required")
	}
	if binding, present := ctx.Value(transactionKey{}).(*transactionBinding); present {
		if !validBinding(binding) || binding.backend != backend {
			return nil, fmt.Errorf("SQLite transaction belongs to a different database")
		}
		return binding.executor, nil
	}
	return sqlExecutor{runner: backend.db}, nil
}

// WriteExecutor requires the governed transaction bound to ctx.
func (backend *backend) WriteExecutor(ctx context.Context) (database.Executor, error) {
	if ctx == nil {
		return nil, fmt.Errorf("SQLite context is required")
	}
	if backend == nil || backend.db == nil {
		return nil, fmt.Errorf("SQLite database is required")
	}
	binding, present := ctx.Value(transactionKey{}).(*transactionBinding)
	if !present {
		return nil, database.ErrTransactionRequired
	}
	if !validBinding(binding) || binding.backend != backend {
		return nil, fmt.Errorf("SQLite transaction belongs to a different database: %w", database.ErrTransactionRequired)
	}
	return binding.executor, nil
}

// AdminExecutor selects privileged transaction or pooled execution.
func (backend *backend) AdminExecutor(ctx context.Context) (database.Executor, error) {
	if ctx == nil {
		return nil, fmt.Errorf("SQLite context is required")
	}
	if backend == nil || backend.db == nil {
		return nil, fmt.Errorf("SQLite database is required")
	}
	if binding, present := ctx.Value(transactionKey{}).(*transactionBinding); present {
		if !validBinding(binding) || binding.backend != backend {
			return nil, fmt.Errorf("SQLite transaction belongs to a different database")
		}
		return binding.executor, nil
	}
	return sqlExecutor{runner: backend.db}, nil
}

// WithinTransaction owns commit and rollback while supporting safe nesting.
func (backend *backend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if ctx == nil {
		return fmt.Errorf("transaction context is required")
	}
	if backend == nil || backend.db == nil {
		return fmt.Errorf("SQLite transaction database is required")
	}
	if operation == nil {
		return fmt.Errorf("transaction operation is required")
	}
	if existing, present := ctx.Value(transactionKey{}).(*transactionBinding); present {
		if !validBinding(existing) || existing.backend != backend {
			return fmt.Errorf("transaction belongs to a different SQLite database")
		}
		returned := false
		defer func() {
			if returned {
				return
			}
			recovered := recover()
			existing.markRollbackOnly(errNestedTransactionPanic)
			panic(recovered)
		}()
		err := operation(ctx)
		returned = true
		if err != nil {
			existing.markRollbackOnly(err)
			return transactionoutcome.RollbackPending(err)
		}
		return nil
	}

	connection, err := backend.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve SQLite transaction connection: %w", err)
	}
	defer connection.Close()
	boundary := &transactionBoundary{}
	if err := installTransactionHooks(connection, boundary); err != nil {
		return err
	}
	defer uninstallTransactionHooks(connection)

	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SQLite transaction: %w", err)
	}
	defer func() {
		boundary.frameworkEnd.Store(true)
		_ = transaction.Rollback()
	}()
	binding := &transactionBinding{
		backend: backend, executor: sqlExecutor{runner: transaction, boundary: boundary}, tx: transaction,
	}
	txContext := context.WithValue(ctx, transactionKey{}, binding)
	operationErr := operation(txContext)
	if boundaryErr := boundary.failure(); boundaryErr != nil {
		operationErr = errors.Join(operationErr, boundaryErr)
	}
	if operationErr != nil {
		rollbackErr := rollbackTransaction(transaction, boundary)
		operationErr = errors.Join(operationErr, binding.rollbackOnlyError())
		if rollbackErr != nil {
			return transactionoutcome.RollbackFailed(operationErr, rollbackErr)
		}
		return transactionoutcome.RolledBack(operationErr)
	}
	if rollbackOnlyErr := binding.rollbackOnlyError(); rollbackOnlyErr != nil {
		rollbackErr := rollbackTransaction(transaction, boundary)
		if rollbackErr != nil {
			return transactionoutcome.RollbackFailed(rollbackOnlyErr, rollbackErr)
		}
		return transactionoutcome.RolledBack(rollbackOnlyErr)
	}
	boundary.frameworkEnd.Store(true)
	if err := transaction.Commit(); err != nil {
		return transactionoutcome.CommitFailed(fmt.Errorf("commit SQLite transaction: %w", err))
	}
	return nil
}

func installTransactionHooks(connection *sql.Conn, boundary *transactionBoundary) error {
	if connection == nil || boundary == nil {
		return fmt.Errorf("SQLite transaction hook boundary is required")
	}
	if err := connection.Raw(func(driverConnection any) error {
		hooks, ok := driverConnection.(transactionHookRegisterer)
		if !ok {
			return fmt.Errorf("SQLite driver does not expose transaction hooks")
		}
		hooks.RegisterCommitHook(boundary.commitHook)
		hooks.RegisterRollbackHook(boundary.rollbackHook)
		return nil
	}); err != nil {
		return fmt.Errorf("install SQLite transaction hooks: %w", err)
	}
	return nil
}

func uninstallTransactionHooks(connection *sql.Conn) {
	if connection == nil {
		return
	}
	_ = connection.Raw(func(driverConnection any) error {
		if hooks, ok := driverConnection.(transactionHookRegisterer); ok {
			hooks.RegisterCommitHook(nil)
			hooks.RegisterRollbackHook(nil)
		}
		return nil
	})
}

func rollbackTransaction(transaction *sql.Tx, boundary *transactionBoundary) error {
	boundary.frameworkEnd.Store(true)
	err := transaction.Rollback()
	if errors.Is(err, sql.ErrTxDone) || boundary.rollbackWasObserved() {
		return nil
	}
	return err
}

func (boundary *transactionBoundary) commitHook() int32 {
	if boundary == nil || boundary.frameworkEnd.Load() {
		return 0
	}
	boundary.mark(errUnexpectedSQLCommit, false)
	return 1
}

func (boundary *transactionBoundary) rollbackHook() {
	if boundary == nil || boundary.frameworkEnd.Load() {
		return
	}
	boundary.mark(errUnexpectedSQLRollback, true)
}

func (boundary *transactionBoundary) mark(cause error, rollbackObserved bool) {
	boundary.mu.Lock()
	if boundary.cause == nil {
		boundary.cause = cause
	}
	boundary.rollbackObserved = boundary.rollbackObserved || rollbackObserved
	boundary.mu.Unlock()
}

func (boundary *transactionBoundary) failure() error {
	if boundary == nil {
		return nil
	}
	boundary.mu.Lock()
	cause := boundary.cause
	boundary.mu.Unlock()
	if cause == nil {
		return nil
	}
	return errors.Join(errTransactionBoundaryEnded, cause)
}

func (boundary *transactionBoundary) rollbackWasObserved() bool {
	if boundary == nil {
		return false
	}
	boundary.mu.Lock()
	observed := boundary.rollbackObserved
	boundary.mu.Unlock()
	return observed
}

func validBinding(binding *transactionBinding) bool {
	return binding != nil && binding.backend != nil && binding.executor != nil && binding.tx != nil
}

func (binding *transactionBinding) markRollbackOnly(cause error) {
	if binding == nil || cause == nil {
		return
	}
	binding.rollbackMu.Lock()
	if binding.rollbackCause == nil {
		binding.rollbackCause = cause
	}
	binding.rollbackMu.Unlock()
}

func (binding *transactionBinding) rollbackOnlyError() error {
	if binding == nil {
		return nil
	}
	binding.rollbackMu.Lock()
	cause := binding.rollbackCause
	binding.rollbackMu.Unlock()
	if cause == nil {
		return nil
	}
	return errors.Join(errNestedTransactionRollback, safeerr.Opaque(cause))
}

func executorFor(ctx context.Context, control databasecontrol.Control) (database.Executor, error) {
	if control == nil {
		return nil, fmt.Errorf("SQLite database control is required")
	}
	executor, err := control.Executor(ctx)
	if err != nil {
		return nil, fmt.Errorf("SQLite database executor is unavailable: %w", err)
	}
	return executor, nil
}
