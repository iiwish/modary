package sqlite

import (
	"context"
	"errors"
	"testing"
)

func TestSQLiteNestedTransactionSuccessJoinsOuterUnit(t *testing.T) {
	services := newNestedTransactionServices(t)
	err := services.control.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		if err := insertNestedProbe(services, txCtx, "outer"); err != nil {
			return err
		}
		return services.control.WithinTransaction(txCtx, func(nested context.Context) error {
			return insertNestedProbe(services, nested, "inner")
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNestedProbeCount(t, services, 2)
}

func TestSQLiteNestedTransactionFailureMarksOuterRollbackOnly(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
	}{
		{name: "ordinary", cause: errors.New("nested operation failed")},
		{name: "typed nil", cause: nestedTypedNilError()},
	} {
		t.Run(test.name, func(t *testing.T) {
			services := newNestedTransactionServices(t)
			err := services.control.WithinTransaction(context.Background(), func(txCtx context.Context) error {
				if err := insertNestedProbe(services, txCtx, "outer-before"); err != nil {
					return err
				}
				nestedErr := services.control.WithinTransaction(txCtx, func(nested context.Context) error {
					if err := insertNestedProbe(services, nested, "inner"); err != nil {
						return err
					}
					return test.cause
				})
				if !errors.Is(nestedErr, test.cause) {
					t.Fatalf("nested error = %v", nestedErr)
				}
				// Swallowing the nested failure cannot restore commit eligibility.
				return insertNestedProbe(services, txCtx, "outer-after")
			})
			if err == nil || !errors.Is(err, errNestedTransactionRollback) || !errors.Is(err, test.cause) {
				t.Fatalf("outer rollback-only error = %v", err)
			}
			assertNestedProbeCount(t, services, 0)
		})
	}
}

func TestSQLiteNestedTransactionPanicMarksOuterRollbackOnly(t *testing.T) {
	for _, test := range []struct {
		name  string
		panic func()
	}{
		{name: "value", panic: func() { panic("nested panic") }},
		{name: "nil", panic: func() { panic(nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			services := newNestedTransactionServices(t)
			err := services.control.WithinTransaction(context.Background(), func(txCtx context.Context) error {
				if err := insertNestedProbe(services, txCtx, "outer-before"); err != nil {
					return err
				}
				if !didPanic(func() {
					_ = services.control.WithinTransaction(txCtx, func(nested context.Context) error {
						if err := insertNestedProbe(services, nested, "inner"); err != nil {
							return err
						}
						test.panic()
						return nil
					})
				}) {
					t.Fatal("nested panic did not propagate")
				}
				return insertNestedProbe(services, txCtx, "outer-after")
			})
			if err == nil || !errors.Is(err, errNestedTransactionRollback) {
				t.Fatalf("outer panic rollback-only error = %v", err)
			}
			assertNestedProbeCount(t, services, 0)
		})
	}
}

func newNestedTransactionServices(t *testing.T) testServices {
	t.Helper()
	services := startTestServices(t, Options{Path: ":memory:"})
	if _, err := services.db.Exec(`CREATE TABLE nested_transaction_probe(value TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	return services
}

func insertNestedProbe(services testServices, ctx context.Context, value string) error {
	executor, err := services.control.Executor(ctx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO nested_transaction_probe(value) VALUES (?)`, value)
	return err
}

func assertNestedProbeCount(t *testing.T, services testServices, want int) {
	t.Helper()
	var got int
	if err := services.db.QueryRow(`SELECT COUNT(*) FROM nested_transaction_probe`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("nested transaction rows = %d, want %d", got, want)
	}
}

func didPanic(callback func()) (panicked bool) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			panicked = true
		}
	}()
	callback()
	returned = true
	return false
}

type nestedNilError struct{}

func (*nestedNilError) Error() string { panic("typed-nil error must remain opaque") }

func nestedTypedNilError() error {
	var err *nestedNilError
	return err
}
