// Package postgrestest provides isolated real-PostgreSQL control for adapter
// integration tests. It is internal to Modary's adapter tree.
package postgrestest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/sqlpolicy"
	"github.com/iiwish/modary/internal/transactionoutcome"
)

const defaultURL = "postgres://modary:modary-test-password@127.0.0.1:55432/modary_test?sslmode=disable"

var (
	sequence      atomic.Uint64
	testIDPattern = regexp.MustCompile(`[^a-z0-9_]+`)
)

type runner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type executor struct{ runner runner }

func (executor executor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return executor.runner.ExecContext(ctx, query, args...)
}
func (executor executor) QueryContext(ctx context.Context, query string, args ...any) (database.Rows, error) {
	return executor.runner.QueryContext(ctx, query, args...)
}
func (executor executor) QueryRowContext(ctx context.Context, query string, args ...any) database.Row {
	return executor.runner.QueryRowContext(ctx, query, args...)
}

type backend struct{ db *sql.DB }
type transactionKey struct{}
type transaction struct {
	backend *backend
	tx      *sql.Tx
}

func (*backend) Driver() string { return "postgres" }
func (*backend) ValidateMigration(script string) error {
	return sqlpolicy.ValidateMigrationScript(script, 1<<20)
}
func (backend *backend) ReadExecutor(ctx context.Context) (database.Executor, error) {
	if tx, present := ctx.Value(transactionKey{}).(*transaction); present {
		if tx == nil || tx.backend != backend || tx.tx == nil {
			return nil, fmt.Errorf("transaction belongs to another database")
		}
		return executor{runner: tx.tx}, nil
	}
	return executor{runner: backend.db}, nil
}
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
func (backend *backend) AdminExecutor(ctx context.Context) (database.Executor, error) {
	return backend.ReadExecutor(ctx)
}
func (backend *backend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if current, present := ctx.Value(transactionKey{}).(*transaction); present {
		if current == nil || current.backend != backend || current.tx == nil {
			return fmt.Errorf("transaction belongs to another database")
		}
		if err := operation(ctx); err != nil {
			return transactionoutcome.RollbackPending(err)
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

// Open creates an isolated schema and database/sql pool and removes the schema
// after the test. Tests are skipped only when the local integration service is
// unreachable.
func Open(t testing.TB) (*sql.DB, databasecontrol.Control) {
	t.Helper()
	url := os.Getenv("MODARY_TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("MODARY_DATABASE_URL")
	}
	if url == "" {
		url = defaultURL
	}
	name := strings.ToLower(testIDPattern.ReplaceAllString(t.Name(), "_"))
	if len(name) > 30 {
		name = name[len(name)-30:]
	}
	schema := fmt.Sprintf("adapter_%s_%d", name, sequence.Add(1))
	admin, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.PingContext(context.Background()); err != nil {
		_ = admin.Close()
		if strings.Contains(err.Error(), "connection refused") {
			t.Skipf("PostgreSQL integration service unavailable: %v", err)
		}
		t.Fatal(err)
	}
	quoted := `"` + schema + `"`
	if _, err := admin.Exec(`CREATE SCHEMA ` + quoted); err != nil {
		t.Fatal(err)
	}
	config, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["search_path"] = quoted
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Minute)
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoted + ` CASCADE`)
		_ = admin.Close()
	})
	control, err := NewControl(db)
	if err != nil {
		t.Fatal(err)
	}
	return db, control
}

// NewControl binds the PostgreSQL test control contract to an existing pool.
// The pool must already resolve its search_path to an isolated test schema.
func NewControl(db *sql.DB) (databasecontrol.Control, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL test database is required")
	}
	return databasecontrol.New(&backend{db: db})
}
