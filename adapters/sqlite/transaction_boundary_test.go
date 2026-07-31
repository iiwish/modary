package sqlite

import (
	"context"
	"fmt"
	"testing"
)

func TestSQLiteTransactionRejectsSQLThatEndsItsOwnedBoundary(t *testing.T) {
	for _, test := range []struct {
		name   string
		script string
	}{
		{
			name: "commit",
			script: `
				INSERT INTO transaction_boundary_probe(value) VALUES ('before');
				COMMIT;
				INSERT INTO transaction_boundary_probe(value) VALUES ('after')`,
		},
		{
			name: "rollback",
			script: `
				INSERT INTO transaction_boundary_probe(value) VALUES ('before');
				ROLLBACK;
				INSERT INTO transaction_boundary_probe(value) VALUES ('after')`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			services := startTestServices(t, Options{Path: ":memory:"})
			if _, err := services.db.Exec(`CREATE TABLE transaction_boundary_probe(value TEXT PRIMARY KEY)`); err != nil {
				t.Fatal(err)
			}
			err := services.control.WithinTransaction(context.Background(), func(txCtx context.Context) error {
				executor, err := services.control.Executor(txCtx)
				if err != nil {
					return err
				}
				// A hostile callback may deliberately ignore the statement error.
				_, _ = executor.ExecContext(txCtx, test.script)
				return nil
			})
			if err == nil {
				t.Fatal("transaction boundary escape was accepted")
			}
			assertTransactionBoundaryRows(t, services, 0)
		})
	}
}

func TestSQLiteTransactionStaysRolledBackAfterSchemaConflictPolicy(t *testing.T) {
	services := startTestServices(t, Options{Path: ":memory:"})
	if _, err := services.db.Exec(`
		CREATE TABLE transaction_boundary_probe(
			value TEXT PRIMARY KEY ON CONFLICT ROLLBACK
		);
		INSERT INTO transaction_boundary_probe(value) VALUES ('duplicate')`); err != nil {
		t.Fatal(err)
	}

	err := services.control.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		executor, err := services.control.Executor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(txCtx, `INSERT INTO transaction_boundary_probe(value) VALUES ('before')`); err != nil {
			return err
		}
		_, _ = executor.ExecContext(txCtx, `INSERT INTO transaction_boundary_probe(value) VALUES ('duplicate')`)
		_, _ = executor.ExecContext(txCtx, `INSERT INTO transaction_boundary_probe(value) VALUES ('after')`)
		return nil
	})
	if err == nil {
		t.Fatal("ROLLBACK conflict policy was not detected")
	}
	assertTransactionBoundaryRows(t, services, 1)
}

func TestSQLiteTransactionStaysRolledBackAfterTriggerRollback(t *testing.T) {
	services := startTestServices(t, Options{Path: ":memory:"})
	if _, err := services.db.Exec(`
		CREATE TABLE transaction_boundary_probe(value TEXT PRIMARY KEY);
		CREATE TRIGGER transaction_boundary_rollback
		BEFORE INSERT ON transaction_boundary_probe
		WHEN NEW.value = 'trigger'
		BEGIN
			SELECT RAISE(ROLLBACK, 'trigger rollback');
		END`); err != nil {
		t.Fatal(err)
	}

	err := services.control.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		executor, err := services.control.Executor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(txCtx, `INSERT INTO transaction_boundary_probe(value) VALUES ('before')`); err != nil {
			return err
		}
		_, _ = executor.ExecContext(txCtx, `INSERT INTO transaction_boundary_probe(value) VALUES ('trigger')`)
		_, _ = executor.ExecContext(txCtx, `INSERT INTO transaction_boundary_probe(value) VALUES ('after')`)
		return nil
	})
	if err == nil {
		t.Fatal("trigger RAISE(ROLLBACK) was not detected")
	}
	assertTransactionBoundaryRows(t, services, 0)
}

func assertTransactionBoundaryRows(t *testing.T, services testServices, want int) {
	t.Helper()
	var got int
	if err := services.db.QueryRow(`SELECT COUNT(*) FROM transaction_boundary_probe`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatal(fmt.Sprintf("transaction boundary rows = %d, want %d", got, want))
	}
}
