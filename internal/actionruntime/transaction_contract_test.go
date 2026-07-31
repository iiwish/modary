package actionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/internal/testsupport"
	"github.com/iiwish/modary/internal/transactionoutcome"
)

type transactionManagerFunc func(context.Context, func(context.Context) error) error

func (manager transactionManagerFunc) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return manager(ctx, operation)
}

func TestWithinTransactionAcceptsOnlyFaithfulSingleInvocation(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var calls atomic.Int32
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			return operation(ctx)
		}), func(context.Context) error {
			calls.Add(1)
			return nil
		})
		if err != nil || calls.Load() != 1 {
			t.Fatalf("transaction = %v, calls = %d", err, calls.Load())
		}
	})

	t.Run("setup failure before callback", func(t *testing.T) {
		want := errors.New("begin failed")
		err := runTransactionContract(t, transactionManagerFunc(func(context.Context, func(context.Context) error) error {
			return want
		}), func(context.Context) error {
			t.Fatal("operation ran after setup failure")
			return nil
		})
		assertTransactionManagerFailure(t, err, want)
	})

	t.Run("commit failure after success", func(t *testing.T) {
		want := errors.New("commit failed")
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			if err := operation(ctx); err != nil {
				return err
			}
			return transactionoutcome.CommitFailed(want)
		}), func(context.Context) error { return nil })
		assertTransactionAtomicityFailure(t, err, want)
	})

	t.Run("confirmed rollback preserves operation business code", func(t *testing.T) {
		operationCause := action.NewError(action.CodePlanStale, "state changed")
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			return transactionoutcome.RolledBack(operation(ctx))
		}), func(context.Context) error { return operationCause })
		if !action.IsCode(err, action.CodePlanStale) || !errors.Is(err, operationCause) || errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
			t.Fatalf("confirmed rollback classification = %v", err)
		}
	})

	t.Run("rollback failure cannot preserve operation business code", func(t *testing.T) {
		operationCause := action.NewError(action.CodePlanStale, "state changed")
		rollbackCause := errors.New("rollback failed")
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			return transactionoutcome.RollbackFailed(operation(ctx), rollbackCause)
		}), func(context.Context) error { return operationCause })
		assertTransactionAtomicityFailure(t, err, operationCause, rollbackCause)
	})

	t.Run("rollback pending cannot preserve operation business code", func(t *testing.T) {
		operationCause := action.NewError(action.CodePlanStale, "state changed")
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			return transactionoutcome.RollbackPending(operation(ctx))
		}), func(context.Context) error { return operationCause })
		assertTransactionAtomicityFailure(t, err, operationCause)
	})
}

func TestWithinTransactionDerivesBusinessCodeOnlyFromOperation(t *testing.T) {
	injected := action.NewError(action.CodeAuthzDenied, "manager-selected code")

	t.Run("operation business code wins", func(t *testing.T) {
		operationCause := action.NewError(action.CodePlanStale, "operation-selected code")
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			operationErr := operation(ctx)
			return transactionoutcome.RolledBack(&action.Error{Code: injected.Code, Message: injected.Message, Cause: operationErr})
		}), func(context.Context) error { return operationCause })
		if !action.IsCode(err, action.CodePlanStale) || action.IsCode(err, action.CodeAuthzDenied) || !errors.Is(err, operationCause) {
			t.Fatalf("operation business code = %v", err)
		}
	})

	t.Run("ordinary operation error stays internal", func(t *testing.T) {
		operationCause := errors.New("ordinary operation failure")
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			operationErr := operation(ctx)
			return transactionoutcome.RolledBack(&action.Error{Code: injected.Code, Message: injected.Message, Cause: operationErr})
		}), func(context.Context) error { return operationCause })
		if !action.IsCode(err, action.CodeInternal) || action.IsCode(err, action.CodeAuthzDenied) || !errors.Is(err, operationCause) {
			t.Fatalf("ordinary operation code = %v", err)
		}
	})
}

func TestWithinTransactionTreatsTypedNilErrorsAsFailures(t *testing.T) {
	var typedNil *transactionTypedNilError
	var cause error = typedNil

	t.Run("setup", func(t *testing.T) {
		err := runTransactionContract(t, transactionManagerFunc(func(context.Context, func(context.Context) error) error {
			return cause
		}), func(context.Context) error {
			t.Fatal("operation ran after typed-nil setup failure")
			return nil
		})
		assertTypedNilTransactionCause(t, err, cause)
	})

	t.Run("operation", func(t *testing.T) {
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			return transactionoutcome.RolledBack(operation(ctx))
		}), func(context.Context) error { return cause })
		assertTypedNilTransactionCause(t, err, cause)
		if errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
			t.Fatalf("faithfully propagated typed-nil error violated contract: %v", err)
		}
	})
}

func TestWithinTransactionRejectsNopManagerRollbackClaim(t *testing.T) {
	operationCause := action.NewError(action.CodePlanStale, "state changed")
	err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
		return operation(ctx)
	}), func(context.Context) error {
		return operationCause
	})
	assertTransactionContractViolation(t, err, operationCause)
	if action.IsCode(err, action.CodePlanStale) {
		t.Fatalf("non-atomic manager preserved a business code: %v", err)
	}
}

func TestWithinTransactionRejectsManagerContractViolations(t *testing.T) {
	operationCause := errors.New("operation failed")
	replacement := errors.New("replacement failure")
	tests := []struct {
		name    string
		manager transactionManagerFunc
		body    func(context.Context) error
		calls   int32
		causes  []error
	}{
		{
			name:    "callback omitted without setup error",
			manager: func(context.Context, func(context.Context) error) error { return nil },
			body:    func(context.Context) error { return nil },
		},
		{
			name: "operation error swallowed",
			manager: func(ctx context.Context, operation func(context.Context) error) error {
				_ = operation(ctx)
				return nil
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause},
		},
		{
			name: "operation error replaced",
			manager: func(ctx context.Context, operation func(context.Context) error) error {
				_ = operation(ctx)
				return replacement
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause, replacement},
		},
		{
			name: "raw operation error returned",
			manager: func(ctx context.Context, operation func(context.Context) error) error {
				return operation(ctx)
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause},
		},
		{
			name: "wrapped operation error returned",
			manager: func(ctx context.Context, operation func(context.Context) error) error {
				return fmt.Errorf("rollback: %w", operation(ctx))
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause},
		},
		{
			name: "joined operation error returned",
			manager: func(ctx context.Context, operation func(context.Context) error) error {
				return errors.Join(errors.New("rollback failed"), operation(ctx))
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause},
		},
		{
			name: "rollback proof wrapped",
			manager: func(ctx context.Context, operation func(context.Context) error) error {
				return fmt.Errorf("decorated: %w", transactionoutcome.RolledBack(operation(ctx)))
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause},
		},
		{
			name: "conflicting proofs joined",
			manager: func(ctx context.Context, operation func(context.Context) error) error {
				return errors.Join(transactionoutcome.RolledBack(operation(ctx)), transactionoutcome.CommitFailed(replacement))
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause, replacement},
		},
		{
			name: "proof names a different operation",
			manager: func(ctx context.Context, operation func(context.Context) error) error {
				_ = operation(ctx)
				return transactionoutcome.RolledBack(replacement)
			},
			body:   func(context.Context) error { return operationCause },
			calls:  1,
			causes: []error{operationCause, replacement},
		},
		{
			name: "callback invoked twice",
			manager: func(ctx context.Context, operation func(context.Context) error) error {
				first := operation(ctx)
				second := operation(ctx)
				return errors.Join(first, second)
			},
			body:  func(context.Context) error { return nil },
			calls: 1,
		},
		{
			name: "nil transaction context",
			manager: func(_ context.Context, operation func(context.Context) error) error {
				return operation(nil)
			},
			body: func(context.Context) error { return nil },
		},
		{
			name: "typed-nil transaction context",
			manager: func(_ context.Context, operation func(context.Context) error) error {
				var ctx *transactionTypedNilContext
				return operation(ctx)
			},
			body: func(context.Context) error { return nil },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			err := runTransactionContract(t, test.manager, func(ctx context.Context) error {
				calls.Add(1)
				return test.body(ctx)
			})
			assertTransactionContractViolation(t, err, test.causes...)
			if calls.Load() != test.calls {
				t.Fatalf("operation calls = %d, want %d", calls.Load(), test.calls)
			}
		})
	}
}

func TestWithinTransactionContainsOperationAndManagerPanics(t *testing.T) {
	t.Run("operation panic faithfully returned by manager", func(t *testing.T) {
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			return transactionoutcome.RolledBack(operation(ctx))
		}), func(context.Context) error { panic(transactionHostilePanic{}) })
		if !action.IsCode(err, action.CodeInternal) || !errors.Is(err, action.ErrCallbackPanic) || errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
			t.Fatalf("operation panic = %v", err)
		}
	})

	t.Run("operation panic swallowed", func(t *testing.T) {
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			_ = operation(ctx)
			return nil
		}), func(context.Context) error { panic(transactionHostilePanic{}) })
		assertTransactionContractViolation(t, err, action.ErrCallbackPanic)
	})

	t.Run("manager panic before callback", func(t *testing.T) {
		err := runTransactionContract(t, transactionManagerFunc(func(context.Context, func(context.Context) error) error {
			panic(transactionHostilePanic{})
		}), func(context.Context) error {
			t.Fatal("operation ran after manager panic")
			return nil
		})
		if !action.IsCode(err, action.CodeInternal) || !errors.Is(err, action.ErrCallbackPanic) || errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
			t.Fatalf("manager panic = %v", err)
		}
	})

	t.Run("manager panic after successful callback", func(t *testing.T) {
		var calls atomic.Int32
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			if err := operation(ctx); err != nil {
				return err
			}
			panic(transactionHostilePanic{})
		}), func(context.Context) error {
			calls.Add(1)
			return nil
		})
		if calls.Load() != 1 || !errors.Is(err, action.ErrCallbackPanic) || !errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
			t.Fatalf("manager post-callback panic = %v, calls = %d", err, calls.Load())
		}
	})

	t.Run("nil operation panic is contained after confirmed rollback", func(t *testing.T) {
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			return transactionoutcome.RolledBack(operation(ctx))
		}), func(context.Context) error { panic(nil) })
		if !action.IsCode(err, action.CodeInternal) || !errors.Is(err, action.ErrCallbackPanic) || errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
			t.Fatalf("nil operation panic = %v", err)
		}
	})

	t.Run("nil manager panic after callback is a contract violation", func(t *testing.T) {
		err := runTransactionContract(t, transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			if err := operation(ctx); err != nil {
				return err
			}
			panic(nil)
		}), func(context.Context) error { return nil })
		assertTransactionContractViolation(t, err, action.ErrCallbackPanic)
	})
}

func TestWithinTransactionRejectsConcurrentAndEscapedInvocations(t *testing.T) {
	t.Run("concurrent repeat", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		var calls atomic.Int32
		manager := transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			first := make(chan error, 1)
			go func() { first <- operation(ctx) }()
			<-entered
			second := operation(ctx)
			close(release)
			return errors.Join(<-first, second)
		})
		err := runTransactionContract(t, manager, func(context.Context) error {
			calls.Add(1)
			close(entered)
			<-release
			return nil
		})
		assertTransactionContractViolation(t, err)
		if calls.Load() != 1 {
			t.Fatalf("operation calls = %d", calls.Load())
		}
	})

	t.Run("escaped callback", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		returned := make(chan error, 1)
		manager := transactionManagerFunc(func(ctx context.Context, operation func(context.Context) error) error {
			go func() { _ = operation(ctx) }()
			<-entered
			return nil
		})
		go func() {
			returned <- runTransactionContract(t, manager, func(context.Context) error {
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
		assertTransactionContractViolation(t, <-returned)
	})

	t.Run("retained late callback", func(t *testing.T) {
		setupErr := errors.New("begin failed")
		var retained func(context.Context) error
		var calls atomic.Int32
		err := runTransactionContract(t, transactionManagerFunc(func(_ context.Context, operation func(context.Context) error) error {
			retained = operation
			return setupErr
		}), func(context.Context) error {
			calls.Add(1)
			return nil
		})
		assertTransactionManagerFailure(t, err, setupErr)
		if lateErr := retained(context.Background()); lateErr == nil || calls.Load() != 0 {
			t.Fatalf("late callback = %v, operation calls = %d", lateErr, calls.Load())
		}
	})
}

func TestRuntimeRejectsSkippedAndRepeatedTransactionCallbacks(t *testing.T) {
	tests := []struct {
		name    string
		manager transactionManagerFunc
		calls   int32
	}{
		{
			name:    "skipped",
			manager: func(context.Context, func(context.Context) error) error { return nil },
		},
		{
			name: "repeated",
			manager: func(ctx context.Context, operation func(context.Context) error) error {
				return errors.Join(operation(ctx), operation(ctx))
			},
			calls: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &transactionEffectHandler{}
			registry := NewRegistry()
			descriptor := testDescriptor()
			descriptor.Preview = action.PreviewNone
			descriptor.PreviewSchema = nil
			descriptor.RequiresIdempotency = false
			if err := registry.Register("test", descriptor, handler); err != nil {
				t.Fatal(err)
			}
			runtime, err := New(registry, Options{
				Authorizer:   testAuthorizer{},
				Audit:        testsupport.DiscardAudit{},
				Plans:        testsupport.NewMemoryPlanStore(),
				Idempotency:  testsupport.NewMemoryIdempotencyStore(),
				Transactions: test.manager,
				Clock:        func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) },
				AuditFailure: func(context.Context, error, audit.Event) {},
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := runtime.Execute(context.Background(), testRequest())
			if !action.IsCode(err, action.CodeInternal) || !errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
				t.Fatalf("Execute() = %#v, %v", result, err)
			}
			if handler.calls.Load() != test.calls {
				t.Fatalf("Handler.Execute calls = %d, want %d", handler.calls.Load(), test.calls)
			}
		})
	}
}

func runTransactionContract(t *testing.T, manager runtimecontrol.TransactionManager, operation func(context.Context) error) error {
	t.Helper()
	runtime := &Engine{tx: manager}
	return runtime.withinTransaction(context.Background(), operation)
}

func assertTransactionManagerFailure(t *testing.T, err, cause error) {
	t.Helper()
	if !action.IsCode(err, action.CodeInternal) || !errors.Is(err, cause) || errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
		t.Fatalf("transaction manager failure = %v", err)
	}
}

func assertTransactionAtomicityFailure(t *testing.T, err error, causes ...error) {
	t.Helper()
	if !action.IsCode(err, action.CodeInternal) || action.IsCode(err, action.CodePlanStale) || errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
		t.Fatalf("transaction atomicity failure = %v", err)
	}
	for _, cause := range causes {
		if !errors.Is(err, cause) {
			t.Fatalf("transaction atomicity failure lost cause %#v: %v", cause, err)
		}
	}
}

func assertTransactionContractViolation(t *testing.T, err error, causes ...error) {
	t.Helper()
	if !action.IsCode(err, action.CodeInternal) || !errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
		t.Fatalf("transaction contract violation = %v", err)
	}
	for _, cause := range causes {
		if !errors.Is(err, cause) {
			t.Fatalf("transaction contract error lost cause %#v: %v", cause, err)
		}
	}
}

func assertTypedNilTransactionCause(t *testing.T, err, cause error) {
	t.Helper()
	if action.ErrorCode(err) != action.CodeInternal || !errors.Is(err, cause) {
		t.Fatalf("typed-nil transaction cause = %v", err)
	}
	typed := &transactionTypedNilError{}
	if !errors.As(err, &typed) || typed != nil {
		t.Fatalf("typed-nil errors.As = %#v, %v", typed, err)
	}
}

type transactionTypedNilError struct{}

func (*transactionTypedNilError) Error() string { panic("typed-nil Error invoked") }
func (*transactionTypedNilError) Is(error) bool { panic("typed-nil Is invoked") }
func (*transactionTypedNilError) As(any) bool   { panic("typed-nil As invoked") }
func (*transactionTypedNilError) Unwrap() error { panic("typed-nil Unwrap invoked") }

type transactionTypedNilContext struct{}

func (*transactionTypedNilContext) Deadline() (time.Time, bool) { panic("typed-nil context used") }
func (*transactionTypedNilContext) Done() <-chan struct{}       { panic("typed-nil context used") }
func (*transactionTypedNilContext) Err() error                  { panic("typed-nil context used") }
func (*transactionTypedNilContext) Value(any) any               { panic("typed-nil context used") }

type transactionHostilePanic struct{}

func (transactionHostilePanic) Error() string  { panic("panic Error invoked") }
func (transactionHostilePanic) String() string { panic("panic String invoked") }

type transactionEffectHandler struct{ calls atomic.Int32 }

func (*transactionEffectHandler) Plan(context.Context, action.Request) (action.PlanData, error) {
	return action.PlanData{Payload: json.RawMessage(`{}`), Impact: authz.Impact{}}, nil
}

func (handler *transactionEffectHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	handler.calls.Add(1)
	return action.Result{Data: json.RawMessage(`{"ok":true}`)}, nil
}
