package actionruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/testsupport"
)

func TestRuntimeDependenciesCannotInjectBusinessErrorCodes(t *testing.T) {
	injected := action.NewError(action.CodeAuthzDenied, "dependency-selected business failure")
	defaultDescriptor := testDescriptor()
	previewNoneDescriptor := defaultDescriptor
	previewNoneDescriptor.Preview = action.PreviewNone
	previewNoneDescriptor.PreviewSchema = nil
	previewNoneDescriptor.RequiresIdempotency = false

	tests := []struct {
		name       string
		descriptor action.Descriptor
		options    Options
		run        func(*Engine) error
	}{
		{
			name:       "authorizer",
			descriptor: defaultDescriptor,
			options:    Options{Authorizer: errorAuthorizer{err: injected}},
			run: func(runtime *Engine) error {
				_, err := runtime.Preview(context.Background(), testRequest())
				return err
			},
		},
		{
			name:       "plan store",
			descriptor: defaultDescriptor,
			options:    Options{Plans: failingPlanStore{err: injected}},
			run: func(runtime *Engine) error {
				_, err := runtime.Preview(context.Background(), testRequest())
				return err
			},
		},
		{
			name:       "idempotency store",
			descriptor: previewNoneDescriptor,
			options:    Options{Idempotency: errorIdempotencyStore{err: injected}},
			run: func(runtime *Engine) error {
				request := testRequest()
				request.IdempotencyKey = "dependency-code-boundary"
				_, err := runtime.Execute(context.Background(), request)
				return err
			},
		},
		{
			name:       "required audit hook",
			descriptor: defaultDescriptor,
			options:    Options{Audit: errorAuditHook{err: injected}},
			run: func(runtime *Engine) error {
				_, err := runtime.Preview(context.Background(), testRequest())
				return err
			},
		},
		{
			name:       "transaction manager",
			descriptor: defaultDescriptor,
			options: Options{Transactions: transactionManagerFunc(func(context.Context, func(context.Context) error) error {
				return injected
			})},
			run: func(runtime *Engine) error {
				_, err := runtime.Preview(context.Background(), testRequest())
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := newDependencyCodeRuntime(t, test.descriptor, dependencyCodeHandler{}, test.options)
			err := test.run(runtime)
			if !action.IsCode(err, action.CodeInternal) || action.IsCode(err, action.CodeAuthzDenied) || !errors.Is(err, injected) {
				t.Fatalf("dependency failure classification = %v", err)
			}
		})
	}
}

func TestRuntimePreservesHandlerBusinessErrorCodes(t *testing.T) {
	for _, test := range []struct {
		name       string
		descriptor action.Descriptor
		handler    action.Handler
		run        func(*Engine) error
	}{
		{
			name:       "plan",
			descriptor: testDescriptor(),
			handler:    dependencyCodeHandler{planErr: action.NewError(action.CodePreconditionFailed, "planning precondition failed")},
			run: func(runtime *Engine) error {
				_, err := runtime.Preview(context.Background(), testRequest())
				return err
			},
		},
		{
			name: "execute",
			descriptor: func() action.Descriptor {
				descriptor := testDescriptor()
				descriptor.Preview = action.PreviewNone
				descriptor.PreviewSchema = nil
				descriptor.RequiresIdempotency = false
				return descriptor
			}(),
			handler: dependencyCodeHandler{executeErr: action.NewError(action.CodePreconditionFailed, "execution precondition failed")},
			run: func(runtime *Engine) error {
				_, err := runtime.Execute(context.Background(), testRequest())
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := newDependencyCodeRuntime(t, test.descriptor, test.handler, Options{})
			err := test.run(runtime)
			if !action.IsCode(err, action.CodePreconditionFailed) {
				t.Fatalf("handler business failure classification = %v", err)
			}
		})
	}
}

func newDependencyCodeRuntime(t *testing.T, descriptor action.Descriptor, handler action.Handler, options Options) *Engine {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register("test", descriptor, handler); err != nil {
		t.Fatal(err)
	}
	if options.Authorizer == nil {
		options.Authorizer = testAuthorizer{}
	}
	if options.Audit == nil {
		options.Audit = testsupport.DiscardAudit{}
	}
	if options.Plans == nil {
		options.Plans = newMemoryPlanStore()
	}
	if options.Idempotency == nil {
		options.Idempotency = newMemoryIdempotencyStore()
	}
	if options.Transactions == nil {
		options.Transactions = confirmedTransactionManager{}
	}
	if options.Clock == nil {
		options.Clock = func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) }
	}
	options.AuditFailure = func(context.Context, error, audit.Event) {}
	runtime, err := New(registry, options)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

type errorAuthorizer struct{ err error }

func (authorizer errorAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{}, authorizer.err
}

type errorAuditHook struct{ err error }

func (hook errorAuditHook) Record(context.Context, audit.Event) error { return hook.err }

type errorIdempotencyStore struct{ err error }

func (store errorIdempotencyStore) Lookup(context.Context, actionpersistence.IdempotencyRecord) (*actionpersistence.IdempotencyRecord, error) {
	return nil, store.err
}

func (store errorIdempotencyStore) Reserve(context.Context, actionpersistence.IdempotencyRecord) (*actionpersistence.IdempotencyRecord, error) {
	return nil, store.err
}

func (store errorIdempotencyStore) Complete(context.Context, actionpersistence.IdempotencyRecord) error {
	return store.err
}

func (store errorIdempotencyStore) Abort(context.Context, actionpersistence.IdempotencyRecord) error {
	return store.err
}

type dependencyCodeHandler struct {
	planErr    error
	executeErr error
}

func (handler dependencyCodeHandler) Plan(context.Context, action.Request) (action.PlanData, error) {
	if handler.planErr != nil {
		return action.PlanData{}, handler.planErr
	}
	return action.PlanData{
		Payload: []byte(`{"value":1}`),
		Summary: []byte(`{"matched_rows":1}`),
		Impact:  authz.Impact{Rows: 1},
	}, nil
}

func (handler dependencyCodeHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	if handler.executeErr != nil {
		return action.Result{}, handler.executeErr
	}
	return action.Result{Data: []byte(`{"ok":true}`)}, nil
}
