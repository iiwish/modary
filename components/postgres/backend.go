package postgresdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/sqlpolicy"
	"github.com/iiwish/modary/internal/transactionoutcome"
)

const maxMigrationScriptBytes = 1 << 20

type sqlRunner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type sqlExecutor struct{ runner sqlRunner }

func (executor sqlExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return executor.runner.ExecContext(ctx, query, args...)
}
func (executor sqlExecutor) QueryContext(ctx context.Context, query string, args ...any) (database.Rows, error) {
	return executor.runner.QueryContext(ctx, query, args...)
}
func (executor sqlExecutor) QueryRowContext(ctx context.Context, query string, args ...any) database.Row {
	return executor.runner.QueryRowContext(ctx, query, args...)
}

type backend struct {
	db               *sql.DB
	migrationLockKey int64
}

type transactionKey struct{}
type transactionBinding struct {
	backend       *backend
	executor      database.Executor
	tx            *sql.Tx
	rollbackMu    sync.Mutex
	rollbackCause error
}

var errNestedRollback = errors.New("PostgreSQL transaction was marked rollback-only")

func (*backend) Driver() string { return "postgres" }

func (backend *backend) ReadExecutor(ctx context.Context) (database.Executor, error) {
	if ctx == nil || backend == nil || backend.db == nil {
		return nil, fmt.Errorf("PostgreSQL database context is unavailable")
	}
	if binding, present := ctx.Value(transactionKey{}).(*transactionBinding); present {
		if !validBinding(binding) || binding.backend != backend {
			return nil, fmt.Errorf("PostgreSQL transaction belongs to a different database")
		}
		return binding.executor, nil
	}
	return sqlExecutor{runner: backend.db}, nil
}

func (backend *backend) WriteExecutor(ctx context.Context) (database.Executor, error) {
	if ctx == nil || backend == nil || backend.db == nil {
		return nil, database.ErrTransactionRequired
	}
	binding, present := ctx.Value(transactionKey{}).(*transactionBinding)
	if !present || !validBinding(binding) || binding.backend != backend {
		return nil, database.ErrTransactionRequired
	}
	return binding.executor, nil
}

func (backend *backend) AdminExecutor(ctx context.Context) (database.Executor, error) {
	return backend.ReadExecutor(ctx)
}

func (backend *backend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if ctx == nil || backend == nil || backend.db == nil || operation == nil {
		return fmt.Errorf("PostgreSQL transaction inputs are unavailable")
	}
	if existing, present := ctx.Value(transactionKey{}).(*transactionBinding); present {
		if !validBinding(existing) || existing.backend != backend {
			return fmt.Errorf("transaction belongs to a different PostgreSQL database")
		}
		returned := false
		defer func() {
			if !returned {
				existing.markRollbackOnly(errors.New("nested PostgreSQL transaction operation panicked"))
			}
		}()
		err := operation(ctx)
		returned = true
		if err != nil {
			existing.markRollbackOnly(err)
			return transactionoutcome.RollbackPending(err)
		}
		return nil
	}
	tx, err := backend.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin PostgreSQL transaction: %w", err)
	}
	defer tx.Rollback()
	binding := &transactionBinding{backend: backend, executor: sqlExecutor{runner: tx}, tx: tx}
	operationErr := operation(context.WithValue(ctx, transactionKey{}, binding))
	if operationErr != nil || binding.rollbackOnlyError() != nil {
		cause := errors.Join(operationErr, binding.rollbackOnlyError())
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return transactionoutcome.RollbackFailed(cause, rollbackErr)
		}
		return transactionoutcome.RolledBack(cause)
	}
	if err := tx.Commit(); err != nil {
		return transactionoutcome.CommitFailed(fmt.Errorf("commit PostgreSQL transaction: %w", err))
	}
	return nil
}

func (binding *transactionBinding) markRollbackOnly(err error) {
	binding.rollbackMu.Lock()
	if binding.rollbackCause == nil {
		binding.rollbackCause = errors.Join(errNestedRollback, err)
	}
	binding.rollbackMu.Unlock()
}
func (binding *transactionBinding) rollbackOnlyError() error {
	if binding == nil {
		return errNestedRollback
	}
	binding.rollbackMu.Lock()
	defer binding.rollbackMu.Unlock()
	return binding.rollbackCause
}
func validBinding(binding *transactionBinding) bool {
	return binding != nil && binding.backend != nil && binding.executor != nil && binding.tx != nil
}

func (*backend) ValidateMigration(script string) error {
	if err := sqlpolicy.ValidateMigrationScript(script, maxMigrationScriptBytes); err != nil {
		return fmt.Errorf("PostgreSQL migration is outside the supported profile: %w", err)
	}
	return nil
}
func (backend *backend) LockMigrations(ctx context.Context, executor database.Executor) error {
	if backend == nil || backend.migrationLockKey == 0 {
		return fmt.Errorf("PostgreSQL migration lock is unavailable")
	}
	_, err := executor.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, backend.migrationLockKey)
	return err
}

func advisoryLockKey(schema string) int64 {
	return databasecontrol.SchemaAdvisoryLockKey("postgres", schema)
}
