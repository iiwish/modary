package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/iiwish/modary/internal/safeerr"
)

var (
	// ErrTransactionRequired reports a write attempted without the transaction
	// binding installed by the governed Runtime.
	ErrTransactionRequired = errors.New("database write requires a governed transaction")
	// ErrReadQueryRequired reports a non-read statement submitted through a
	// Query method. Mutations must use ExecContext and a governed transaction.
	ErrReadQueryRequired = errors.New("database query must be a single SELECT statement")
	// ErrMutationStatementRequired reports SQL outside the public mutation
	// surface. ExecContext accepts one INSERT, UPDATE, or DELETE statement.
	ErrMutationStatementRequired = errors.New("database mutation must be a single INSERT, UPDATE, or DELETE statement")
	// ErrAccessUnavailable reports an invalid or unavailable database capability.
	ErrAccessUnavailable = errors.New("database access is unavailable")
	// ErrRowsClosed reports an operation that requires an open Rows iterator.
	ErrRowsClosed = errors.New("database rows are closed")
	// ErrDependencyPanic identifies a recovered panic from an official database
	// dependency boundary.
	ErrDependencyPanic = errors.New("database dependency panicked")
)

// Row is the deferred result of one query. It intentionally exposes only Scan.
// A Row is owned by one caller and is not a concurrent-use surface.
type Row interface {
	Scan(...any) error
}

// Rows is the multi-row read surface exposed to consumer Modules. It
// intentionally omits driver and connection access while retaining the ordinary
// iteration contract. The caller owns the cursor, must close it, and must not
// call its methods concurrently.
type Rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Columns() ([]string, error)
	Close() error
}

// Executor is the narrow SQL method set shared by the consumer Access contract
// and framework-internal database control. It exposes no commit, rollback,
// driver, or raw-connection operation. The same Executor may be called
// concurrently. Implementations must be safe for concurrent use and honor each
// method context's cancellation and deadline.
type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (Rows, error)
	QueryRowContext(context.Context, string, ...any) Row
}

// Access is the database capability exposed to consumer Modules. The
// framework-owned implementation accepts one SELECT for reads and permits one
// INSERT, UPDATE, or DELETE only through the transaction-bound context supplied
// by the governed Runtime.
//
// Consumers may implement Access in isolated tests. Implementing this method
// set does not make a value installable as the Host's canonical database
// service and grants no migration, administration, or transaction ownership.
type Access interface {
	Executor
}

// IsDependencyPanic reports whether err contains ErrDependencyPanic without
// invoking custom Is, As, or unsafe Unwrap methods on dependency errors.
func IsDependencyPanic(err error) bool {
	return safeerr.Is(err, ErrDependencyPanic)
}
