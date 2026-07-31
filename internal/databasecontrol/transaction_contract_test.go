package databasecontrol

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/transactionoutcome"
)

type transactionBackendFunc func(context.Context, func(context.Context) error) error

type contractBackend struct {
	transaction transactionBackendFunc
	retained    func(context.Context) error
}

func (*contractBackend) Driver() string { return "contract" }

func (*contractBackend) ValidateMigration(string) error { return nil }

func (*contractBackend) ReadExecutor(context.Context) (database.Executor, error) {
	return nil, errors.New("read executor is not used by transaction contract tests")
}

func (*contractBackend) WriteExecutor(context.Context) (database.Executor, error) {
	return nil, errors.New("write executor is not used by transaction contract tests")
}

func (*contractBackend) AdminExecutor(context.Context) (database.Executor, error) {
	return nil, errors.New("admin executor is not used by transaction contract tests")
}

func (backend *contractBackend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if backend.transaction == nil {
		if err := operation(ctx); err != nil {
			return transactionoutcome.RolledBack(err)
		}
		return nil
	}
	return backend.transaction(ctx, operation)
}

func TestDatabaseTransactionAcceptsOnlyExactRootOutcomes(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var calls atomic.Int32
		err := runDatabaseTransactionContract(t, func(ctx context.Context, operation func(context.Context) error) error {
			return operation(ctx)
		}, func(context.Context) error {
			calls.Add(1)
			return nil
		})
		if err != nil || calls.Load() != 1 {
			t.Fatalf("transaction = %v, calls = %d", err, calls.Load())
		}
	})

	t.Run("setup failure before callback", func(t *testing.T) {
		want := errors.New("begin failed")
		err := runDatabaseTransactionContract(t, func(context.Context, func(context.Context) error) error {
			return want
		}, func(context.Context) error {
			t.Fatal("operation ran after setup failure")
			return nil
		})
		assertDatabaseTransactionFailure(t, err, want)
	})

	t.Run("commit failure after success", func(t *testing.T) {
		want := errors.New("commit failed")
		err := runDatabaseTransactionContract(t, func(ctx context.Context, operation func(context.Context) error) error {
			if err := operation(ctx); err != nil {
				return err
			}
			return transactionoutcome.CommitFailed(want)
		}, func(context.Context) error { return nil })
		assertDatabaseTransactionOutcome(t, err, transactionoutcome.StateCommitFailed, nil, want)
	})

	for _, outcome := range []struct {
		name  string
		state transactionoutcome.State
		make  func(error, error) error
	}{
		{
			name:  "rollback pending",
			state: transactionoutcome.StateRollbackPending,
			make:  func(operationErr, _ error) error { return transactionoutcome.RollbackPending(operationErr) },
		},
		{
			name:  "rolled back",
			state: transactionoutcome.StateRolledBack,
			make:  func(operationErr, _ error) error { return transactionoutcome.RolledBack(operationErr) },
		},
		{
			name:  "rollback failed",
			state: transactionoutcome.StateRollbackFailed,
			make: func(operationErr, rollbackErr error) error {
				return transactionoutcome.RollbackFailed(operationErr, rollbackErr)
			},
		},
	} {
		t.Run("operation failure "+outcome.name, func(t *testing.T) {
			operationCause := errors.New("operation failed")
			rollbackCause := errors.New("rollback failed")
			err := runDatabaseTransactionContract(t, func(ctx context.Context, operation func(context.Context) error) error {
				return outcome.make(operation(ctx), rollbackCause)
			}, func(context.Context) error { return operationCause })
			completion := error(nil)
			if outcome.state == transactionoutcome.StateRollbackFailed {
				completion = rollbackCause
			}
			assertDatabaseTransactionOutcome(t, err, outcome.state, operationCause, completion)
		})
	}
}

func TestDatabaseTransactionTreatsTypedNilErrorsAsFailures(t *testing.T) {
	var typedNil *databaseTransactionTypedNilError
	var cause error = typedNil

	t.Run("setup", func(t *testing.T) {
		err := runDatabaseTransactionContract(t, func(context.Context, func(context.Context) error) error {
			return cause
		}, func(context.Context) error {
			t.Fatal("operation ran after typed-nil setup failure")
			return nil
		})
		assertTypedNilDatabaseTransactionCause(t, err, cause)
	})

	t.Run("operation", func(t *testing.T) {
		err := runDatabaseTransactionContract(t, func(ctx context.Context, operation func(context.Context) error) error {
			return transactionoutcome.RolledBack(operation(ctx))
		}, func(context.Context) error { return cause })
		assertTypedNilDatabaseTransactionCause(t, err, cause)
		if errors.Is(err, errTransactionBackendContract) {
			t.Fatalf("faithfully propagated typed-nil error violated contract: %v", err)
		}
	})
}

func TestDatabaseTransactionRejectsBackendContractViolations(t *testing.T) {
	operationCause := errors.New("operation failed")
	replacement := errors.New("replacement failure")
	replayedCause := errors.New("previous operation failed")
	replayedProof := transactionoutcome.RolledBack(replayedCause)
	tests := []struct {
		name        string
		transaction transactionBackendFunc
		body        func(context.Context) error
		calls       int32
		causes      []error
	}{
		{
			name:        "callback omitted without setup error",
			transaction: func(context.Context, func(context.Context) error) error { return nil },
			body:        func(context.Context) error { return nil },
		},
		{
			name: "outcome returned before callback",
			transaction: func(context.Context, func(context.Context) error) error {
				return transactionoutcome.CommitFailed(replacement)
			},
			body: func(context.Context) error { return nil },
		},
		{
			name: "raw commit error after successful callback",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				if err := operation(ctx); err != nil {
					return err
				}
				return replacement
			},
			body:   func(context.Context) error { return nil },
			calls:  1,
			causes: []error{replacement},
		},
		{
			name: "rollback outcome after successful callback",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				if err := operation(ctx); err != nil {
					return err
				}
				return transactionoutcome.RolledBack(replacement)
			},
			body:   func(context.Context) error { return nil },
			calls:  1,
			causes: []error{replacement},
		},
		{
			name: "operation error swallowed",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				_ = operation(ctx)
				return nil
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause},
		},
		{
			name: "operation error replaced",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				_ = operation(ctx)
				return replacement
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause, replacement},
		},
		{
			name: "raw operation error without outcome",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				return operation(ctx)
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause},
		},
		{
			name: "wrapped outcome",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				return fmt.Errorf("wrapped rollback: %w", transactionoutcome.RolledBack(operation(ctx)))
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause},
		},
		{
			name: "joined outcome",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				return errors.Join(transactionoutcome.RolledBack(operation(ctx)), replacement)
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause, replacement},
		},
		{
			name: "replayed exact outcome",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				_ = operation(ctx)
				return replayedProof
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause, replayedCause},
		},
		{
			name: "exact outcome for wrong operation",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				_ = operation(ctx)
				return transactionoutcome.RollbackFailed(replacement, errors.New("rollback failed"))
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause, replacement},
		},
		{
			name: "commit outcome after operation failure",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				_ = operation(ctx)
				return transactionoutcome.CommitFailed(replacement)
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause, replacement},
		},
		{
			name: "callback invoked twice",
			transaction: func(ctx context.Context, operation func(context.Context) error) error {
				return errors.Join(operation(ctx), operation(ctx))
			},
			body:  func(context.Context) error { return nil },
			calls: 1,
		},
		{
			name: "nil transaction context",
			transaction: func(_ context.Context, operation func(context.Context) error) error {
				return operation(nil)
			},
			body: func(context.Context) error { return nil },
		},
		{
			name: "typed-nil transaction context",
			transaction: func(_ context.Context, operation func(context.Context) error) error {
				var ctx *databaseTransactionTypedNilContext
				return operation(ctx)
			},
			body: func(context.Context) error { return nil },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			err := runDatabaseTransactionContract(t, test.transaction, func(ctx context.Context) error {
				calls.Add(1)
				return test.body(ctx)
			})
			assertDatabaseTransactionContractViolation(t, err, test.causes...)
			if calls.Load() != test.calls {
				t.Fatalf("operation calls = %d, want %d", calls.Load(), test.calls)
			}
		})
	}
}

func TestDatabaseTransactionPreservesOperationPanicSemantics(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "value", value: &databaseTransactionPanic{}},
		{name: "nil", value: nil},
	} {
		t.Run("faithful rollback re-panics "+test.name, func(t *testing.T) {
			got, panicked := captureDatabaseTransactionPanic(func() {
				_ = runDatabaseTransactionContract(t, func(ctx context.Context, operation func(context.Context) error) error {
					return transactionoutcome.RolledBack(operation(ctx))
				}, func(context.Context) error { panic(test.value) })
			})
			if !panicked {
				t.Fatal("faithfully rolled-back operation panic was swallowed")
			}
			if test.value != nil && got != test.value {
				t.Fatalf("panic identity = %#v, want %#v", got, test.value)
			}
		})
	}

	t.Run("swallowed panic becomes contract failure", func(t *testing.T) {
		err := runDatabaseTransactionContract(t, func(ctx context.Context, operation func(context.Context) error) error {
			_ = operation(ctx)
			return nil
		}, func(context.Context) error { panic(&databaseTransactionPanic{}) })
		assertDatabaseTransactionContractViolation(t, err)
	})

	for _, test := range []struct {
		name  string
		panic func()
	}{
		{name: "value", panic: func() { panic(&databaseTransactionPanic{}) }},
		{name: "nil", panic: func() { panic(nil) }},
	} {
		t.Run("backend panic before callback "+test.name, func(t *testing.T) {
			err := runDatabaseTransactionContract(t, func(context.Context, func(context.Context) error) error {
				test.panic()
				return nil
			}, func(context.Context) error {
				t.Fatal("operation ran after backend panic")
				return nil
			})
			if !database.IsDependencyPanic(err) || errors.Is(err, errTransactionBackendContract) {
				t.Fatalf("backend panic = %v", err)
			}
		})

		t.Run("backend panic after callback "+test.name, func(t *testing.T) {
			var calls atomic.Int32
			err := runDatabaseTransactionContract(t, func(ctx context.Context, operation func(context.Context) error) error {
				if err := operation(ctx); err != nil {
					return err
				}
				test.panic()
				return nil
			}, func(context.Context) error {
				calls.Add(1)
				return nil
			})
			if calls.Load() != 1 {
				t.Fatalf("operation calls = %d, want 1", calls.Load())
			}
			assertDatabaseTransactionContractViolation(t, err)
			if !database.IsDependencyPanic(err) {
				t.Fatalf("backend panic classification was lost: %v", err)
			}
		})
	}
}

func TestDatabaseTransactionRejectsConcurrentEscapedAndLateInvocations(t *testing.T) {
	t.Run("concurrent repeat", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		var calls atomic.Int32
		err := runDatabaseTransactionContract(t, func(ctx context.Context, operation func(context.Context) error) error {
			first := make(chan error, 1)
			go func() { first <- operation(ctx) }()
			<-entered
			second := operation(ctx)
			close(release)
			return errors.Join(<-first, second)
		}, func(context.Context) error {
			calls.Add(1)
			close(entered)
			<-release
			return nil
		})
		assertDatabaseTransactionContractViolation(t, err)
		if calls.Load() != 1 {
			t.Fatalf("operation calls = %d", calls.Load())
		}
	})

	t.Run("escaped callback", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		returned := make(chan error, 1)
		go func() {
			returned <- runDatabaseTransactionContract(t, func(ctx context.Context, operation func(context.Context) error) error {
				go func() { _ = operation(ctx) }()
				<-entered
				return nil
			}, func(context.Context) error {
				close(entered)
				<-release
				return nil
			})
		}()
		select {
		case err := <-returned:
			t.Fatalf("transaction returned before escaped callback finished: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
		close(release)
		assertDatabaseTransactionContractViolation(t, <-returned)
	})

	t.Run("retained late callback", func(t *testing.T) {
		setupErr := errors.New("begin failed")
		backend := &contractBackend{}
		backend.transaction = func(_ context.Context, operation func(context.Context) error) error {
			backend.retained = operation
			return setupErr
		}
		control, err := New(backend)
		if err != nil {
			t.Fatal(err)
		}
		var calls atomic.Int32
		err = control.WithinTransaction(context.Background(), func(context.Context) error {
			calls.Add(1)
			return nil
		})
		assertDatabaseTransactionFailure(t, err, setupErr)
		if lateErr := backend.retained(context.Background()); lateErr == nil || calls.Load() != 0 {
			t.Fatalf("late callback = %v, operation calls = %d", lateErr, calls.Load())
		}
	})
}

func TestApplyMigrationsRejectsSkippedTransactionCallback(t *testing.T) {
	backend := &contractBackend{transaction: func(context.Context, func(context.Context) error) error { return nil }}
	control, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"0001_probe.sql": &fstest.MapFile{Mode: fs.FileMode(0o600), Data: []byte("CREATE TABLE probe (id INTEGER);")},
	}
	err = control.ApplyMigrations(context.Background(), "probe", files)
	assertDatabaseTransactionContractViolation(t, err)
}

func runDatabaseTransactionContract(t *testing.T, transaction transactionBackendFunc, operation func(context.Context) error) error {
	t.Helper()
	control, err := New(&contractBackend{transaction: transaction})
	if err != nil {
		t.Fatal(err)
	}
	return control.WithinTransaction(context.Background(), operation)
}

func assertDatabaseTransactionFailure(t *testing.T, err, cause error) {
	t.Helper()
	if err == nil || !errors.Is(err, cause) || errors.Is(err, errTransactionBackendContract) {
		t.Fatalf("database transaction failure = %v", err)
	}
}

func assertDatabaseTransactionOutcome(
	t *testing.T,
	err error,
	wantState transactionoutcome.State,
	wantOperation error,
	wantCompletion error,
) {
	t.Helper()
	proof, ok := transactionoutcome.Root(err)
	if !ok || proof.State != wantState {
		t.Fatalf("database transaction outcome = %#v, %t (%v)", proof, ok, err)
	}
	if errors.Is(err, errTransactionBackendContract) {
		t.Fatalf("exact outcome was classified as a contract violation: %v", err)
	}
	if wantOperation == nil {
		if proof.Operation != nil {
			t.Fatalf("unexpected operation cause: %v", proof.Operation)
		}
	} else if !errors.Is(proof.Operation, wantOperation) {
		t.Fatalf("operation cause %#v was not preserved", wantOperation)
	}
	if wantCompletion == nil {
		if proof.Completion != nil {
			t.Fatalf("unexpected completion cause: %v", proof.Completion)
		}
	} else if !errors.Is(proof.Completion, wantCompletion) {
		t.Fatalf("completion cause %#v was not preserved", wantCompletion)
	}
}

func assertDatabaseTransactionContractViolation(t *testing.T, err error, causes ...error) {
	t.Helper()
	if err == nil || !errors.Is(err, errTransactionBackendContract) {
		t.Fatalf("database transaction contract violation = %v", err)
	}
	for _, cause := range causes {
		if !errors.Is(err, cause) {
			t.Fatalf("database transaction contract lost cause %#v: %v", cause, err)
		}
	}
}

func assertTypedNilDatabaseTransactionCause(t *testing.T, err, cause error) {
	t.Helper()
	if err == nil || !errors.Is(err, cause) {
		t.Fatalf("typed-nil database transaction cause = %v", err)
	}
	typed := &databaseTransactionTypedNilError{}
	if !errors.As(err, &typed) || typed != nil {
		t.Fatalf("typed-nil errors.As = %#v, %v", typed, err)
	}
}

func captureDatabaseTransactionPanic(operation func()) (recovered any, panicked bool) {
	returned := false
	defer func() {
		if !returned {
			recovered = recover()
			panicked = true
		}
	}()
	operation()
	returned = true
	return nil, false
}

type databaseTransactionTypedNilError struct{}

func (*databaseTransactionTypedNilError) Error() string { panic("typed-nil Error invoked") }
func (*databaseTransactionTypedNilError) Is(error) bool { panic("typed-nil Is invoked") }
func (*databaseTransactionTypedNilError) As(any) bool   { panic("typed-nil As invoked") }
func (*databaseTransactionTypedNilError) Unwrap() error { panic("typed-nil Unwrap invoked") }

type databaseTransactionTypedNilContext struct{}

func (*databaseTransactionTypedNilContext) Deadline() (time.Time, bool) {
	panic("typed-nil context used")
}
func (*databaseTransactionTypedNilContext) Done() <-chan struct{} { panic("typed-nil context used") }
func (*databaseTransactionTypedNilContext) Err() error            { panic("typed-nil context used") }
func (*databaseTransactionTypedNilContext) Value(any) any         { panic("typed-nil context used") }

type databaseTransactionPanic struct{}

func (*databaseTransactionPanic) Error() string  { panic("panic Error invoked") }
func (*databaseTransactionPanic) String() string { panic("panic String invoked") }
