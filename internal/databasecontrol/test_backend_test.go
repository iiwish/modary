package databasecontrol

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

	. "github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/transactionoutcome"
)

const databaseControlTestURL = "postgres://modary:modary-test-password@127.0.0.1:55432/modary_test?sslmode=disable"

var (
	databaseControlTestSequence atomic.Uint64
	databaseControlTestPattern  = regexp.MustCompile(`[^a-z0-9_]+`)
)

type testSQLRunner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type testExecutor struct{ runner testSQLRunner }

func (executor testExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return executor.runner.ExecContext(ctx, query, args...)
}

func (executor testExecutor) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return executor.runner.QueryContext(ctx, query, args...)
}

func (executor testExecutor) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return executor.runner.QueryRowContext(ctx, query, args...)
}

type testBackend struct{ db *sql.DB }
type testTransactionKey struct{}
type testTransaction struct {
	backend *testBackend
	tx      *sql.Tx
}

func (*testBackend) Driver() string { return "postgres" }

func (*testBackend) ValidateMigration(string) error { return nil }

func (backend *testBackend) ReadExecutor(ctx context.Context) (Executor, error) {
	if tx, present := ctx.Value(testTransactionKey{}).(*testTransaction); present {
		if tx == nil || tx.backend != backend || tx.tx == nil {
			return nil, fmt.Errorf("transaction belongs to another database")
		}
		return testExecutor{runner: tx.tx}, nil
	}
	return testExecutor{runner: backend.db}, nil
}

func (backend *testBackend) WriteExecutor(ctx context.Context) (Executor, error) {
	tx, present := ctx.Value(testTransactionKey{}).(*testTransaction)
	if !present {
		return nil, ErrTransactionRequired
	}
	if tx == nil || tx.backend != backend || tx.tx == nil {
		return nil, fmt.Errorf("transaction belongs to another database: %w", ErrTransactionRequired)
	}
	return testExecutor{runner: tx.tx}, nil
}

func (backend *testBackend) AdminExecutor(ctx context.Context) (Executor, error) {
	if tx, present := ctx.Value(testTransactionKey{}).(*testTransaction); present {
		if tx == nil || tx.backend != backend || tx.tx == nil {
			return nil, fmt.Errorf("transaction belongs to another database")
		}
		return testExecutor{runner: tx.tx}, nil
	}
	return testExecutor{runner: backend.db}, nil
}

func (backend *testBackend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if existing, present := ctx.Value(testTransactionKey{}).(*testTransaction); present {
		if existing == nil || existing.backend != backend || existing.tx == nil {
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
	txCtx := context.WithValue(ctx, testTransactionKey{}, &testTransaction{backend: backend, tx: tx})
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

func newTestControl(t interface {
	Helper()
	Fatal(...any)
}, db *sql.DB) Control {
	t.Helper()
	control, err := New(&testBackend{db: db})
	if err != nil {
		t.Fatal(err)
	}
	return control
}

func openPostgresTestDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("MODARY_TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("MODARY_DATABASE_URL")
	}
	if url == "" {
		url = databaseControlTestURL
	}
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
	name := strings.ToLower(databaseControlTestPattern.ReplaceAllString(t.Name(), "_"))
	if len(name) > 30 {
		name = name[len(name)-30:]
	}
	schema := fmt.Sprintf("control_%s_%d", name, databaseControlTestSequence.Add(1))
	quoted := `"` + schema + `"`
	if _, err := admin.Exec(`CREATE SCHEMA ` + quoted); err != nil {
		_ = admin.Close()
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
	return db
}
