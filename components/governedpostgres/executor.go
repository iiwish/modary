package governedpostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/transactionoutcome"
)

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
	backend  *backend
	executor database.Executor
	tx       *sql.Tx

	rollbackMu    sync.Mutex
	rollbackCause error
}

var (
	errNestedTransactionRollback = errors.New("PostgreSQL transaction was marked rollback-only")
	errNestedTransactionPanic    = errors.New("nested PostgreSQL transaction operation panicked")
)

func (*backend) Driver() string { return "postgres" }

func (backend *backend) ReadExecutor(ctx context.Context) (database.Executor, error) {
	if ctx == nil {
		return nil, fmt.Errorf("PostgreSQL context is required")
	}
	if backend == nil || backend.db == nil {
		return nil, fmt.Errorf("PostgreSQL database is required")
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
	if ctx == nil {
		return nil, fmt.Errorf("PostgreSQL context is required")
	}
	if backend == nil || backend.db == nil {
		return nil, fmt.Errorf("PostgreSQL database is required")
	}
	binding, present := ctx.Value(transactionKey{}).(*transactionBinding)
	if !present {
		return nil, database.ErrTransactionRequired
	}
	if !validBinding(binding) || binding.backend != backend {
		return nil, fmt.Errorf("PostgreSQL transaction belongs to a different database: %w", database.ErrTransactionRequired)
	}
	return binding.executor, nil
}

func (backend *backend) AdminExecutor(ctx context.Context) (database.Executor, error) {
	return backend.ReadExecutor(ctx)
}

func (backend *backend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if ctx == nil {
		return fmt.Errorf("transaction context is required")
	}
	if backend == nil || backend.db == nil {
		return fmt.Errorf("PostgreSQL transaction database is required")
	}
	if operation == nil {
		return fmt.Errorf("transaction operation is required")
	}
	if existing, present := ctx.Value(transactionKey{}).(*transactionBinding); present {
		if !validBinding(existing) || existing.backend != backend {
			return fmt.Errorf("transaction belongs to a different PostgreSQL database")
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

	transaction, err := backend.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin PostgreSQL transaction: %w", err)
	}
	defer transaction.Rollback()
	binding := &transactionBinding{
		backend: backend, executor: sqlExecutor{runner: transaction}, tx: transaction,
	}
	txContext := context.WithValue(ctx, transactionKey{}, binding)
	operationErr := operation(txContext)
	if operationErr != nil {
		operationErr = errors.Join(operationErr, binding.rollbackOnlyError())
		if rollbackErr := transaction.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return transactionoutcome.RollbackFailed(operationErr, rollbackErr)
		}
		return transactionoutcome.RolledBack(operationErr)
	}
	if rollbackOnlyErr := binding.rollbackOnlyError(); rollbackOnlyErr != nil {
		if rollbackErr := transaction.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return transactionoutcome.RollbackFailed(rollbackOnlyErr, rollbackErr)
		}
		return transactionoutcome.RolledBack(rollbackOnlyErr)
	}
	if err := transaction.Commit(); err != nil {
		return transactionoutcome.CommitFailed(fmt.Errorf("commit PostgreSQL transaction: %w", err))
	}
	return nil
}

func (backend *backend) transaction(ctx context.Context) (*sql.Tx, error) {
	if backend == nil || backend.db == nil || ctx == nil {
		return nil, fmt.Errorf("%w", database.ErrTransactionRequired)
	}
	binding, present := ctx.Value(transactionKey{}).(*transactionBinding)
	if !present || !validBinding(binding) || binding.backend != backend {
		return nil, database.ErrTransactionRequired
	}
	if binding.rollbackOnlyError() != nil {
		return nil, errNestedTransactionRollback
	}
	return binding.tx, nil
}

func (binding *transactionBinding) markRollbackOnly(cause error) {
	binding.rollbackMu.Lock()
	if binding.rollbackCause == nil {
		binding.rollbackCause = errors.Join(errNestedTransactionRollback, cause)
	}
	binding.rollbackMu.Unlock()
}

func (binding *transactionBinding) rollbackOnlyError() error {
	if binding == nil {
		return errNestedTransactionRollback
	}
	binding.rollbackMu.Lock()
	defer binding.rollbackMu.Unlock()
	return binding.rollbackCause
}

func validBinding(binding *transactionBinding) bool {
	return binding != nil && binding.backend != nil && binding.executor != nil && binding.tx != nil
}
