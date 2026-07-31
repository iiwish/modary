package actionruntime

import (
	"context"
	"encoding/json"
	"errors"
	. "github.com/iiwish/modary/action"
	. "github.com/iiwish/modary/internal/actionpersistence"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/internal/testsupport"
)

func TestRuntimeNeverFormatsRecoveredValuesOrUntrustedErrors(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		handler Handler
	}{
		{name: "hostile panic value", handler: hostileFailureHandler{panicValue: hostileStringer{secret: "panic-secret"}}},
		{name: "hostile error", handler: hostileFailureHandler{err: &hostileError{secret: "error-secret"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, events := newTestRuntime(t, test.handler, &clock)
			_, err := runtime.Preview(context.Background(), testRequest())
			if !IsCode(err, CodeInternal) {
				t.Fatalf("Preview() error = %v", err)
			}
			for _, secret := range []string{"panic-secret", "error-secret"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error disclosed %q: %v", secret, err)
				}
			}
			events.mu.Lock()
			defer events.mu.Unlock()
			if len(events.events) == 0 {
				t.Fatal("failure was not audited")
			}
			reason := events.events[len(events.events)-1].Reason
			if strings.Contains(reason, "panic-secret") || strings.Contains(reason, "error-secret") {
				t.Fatalf("audit reason disclosed hostile callback data: %q", reason)
			}
		})
	}
}

func TestActionErrorHelpersHandleTypedNilAndHostileUnwrap(t *testing.T) {
	var typedNil *Error
	var err error = typedNil
	if code := ErrorCode(err); code != CodeInternal {
		t.Fatalf("typed-nil ErrorCode() = %q", code)
	}
	if IsCode(err, CodeInternal) {
		t.Fatal("typed-nil error matched a concrete Action code")
	}
	wrapped := WithRequest(err, testRequest(), "test.execute")
	if got := wrapped.Error(); got != "INTERNAL_ERROR: action execution failed" {
		t.Fatalf("typed-nil WithRequest() = %q", got)
	}
	if got := typedNil.Error(); got != "INTERNAL_ERROR: action execution failed" {
		t.Fatalf("nil Error receiver = %q", got)
	}
	if typedNil.Unwrap() != nil {
		t.Fatal("nil Error receiver unwrapped a cause")
	}

	hostile := &hostileError{secret: "must-not-format"}
	if code := ErrorCode(hostile); code != CodeInternal || IsCode(hostile, CodeValidationFailed) {
		t.Fatalf("hostile error helpers = %q, %t", code, IsCode(hostile, CodeValidationFailed))
	}
	if got := WithRequest(hostile, testRequest(), "test.execute").Error(); strings.Contains(got, hostile.secret) {
		t.Fatalf("WithRequest disclosed hostile error: %q", got)
	}

	nested := NewError(CodePlanStale, "stale")
	matcher := &hostileMatcher{nested: nested}
	if code := ErrorCode(matcher); code != CodePlanStale || !safeErrorIs(matcher, nested) {
		t.Fatalf("bounded error walker did not traverse standard Unwrap: code=%q match=%t", code, safeErrorIs(matcher, nested))
	}
	if matcher.asCalled.Load() || matcher.isCalled.Load() {
		t.Fatal("error walker invoked custom As or Is behavior")
	}
	cycle := &cyclicError{}
	cycle.next = cycle
	if code := ErrorCode(cycle); code != CodeInternal || safeErrorIs(cycle, nested) {
		t.Fatalf("cyclic error graph was not bounded: code=%q", code)
	}
}

func TestRuntimeContainsEveryExternalDependencyPanic(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	newRuntime := func(t *testing.T, options Options) *Engine {
		t.Helper()
		registry := newTestRegistry(t)
		if err := registry.Register("test", testDescriptor(), &testHandler{}); err != nil {
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
			options.Clock = func() time.Time { return clock }
		}
		if options.AuditFailure == nil {
			options.AuditFailure = func(context.Context, error, audit.Event) {}
		}
		runtime, err := New(registry, options)
		if err != nil {
			t.Fatal(err)
		}
		return runtime
	}

	tests := []struct {
		name string
		run  func(*testing.T) error
	}{
		{name: "clock", run: func(t *testing.T) error {
			runtime := newRuntime(t, Options{Clock: func() time.Time { panic(hostileStringer{secret: "clock-secret"}) }})
			_, err := runtime.Preview(context.Background(), testRequest())
			return err
		}},
		{name: "authorizer", run: func(t *testing.T) error {
			runtime := newRuntime(t, Options{Authorizer: panicAuthorizer{}})
			_, err := runtime.Preview(context.Background(), testRequest())
			return err
		}},
		{name: "audit hook", run: func(t *testing.T) error {
			runtime := newRuntime(t, Options{Audit: panicAuditHook{}, AuditTimeout: 10 * time.Millisecond})
			_, err := runtime.Preview(context.Background(), testRequest())
			return err
		}},
		{name: "plan store", run: func(t *testing.T) error {
			runtime := newRuntime(t, Options{Plans: panicPlanStore{}})
			_, err := runtime.Preview(context.Background(), testRequest())
			return err
		}},
		{name: "idempotency store", run: func(t *testing.T) error {
			runtime := newRuntime(t, Options{})
			request := testRequest()
			preview, err := runtime.Preview(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			runtime.idempotency = panicIdempotencyStore{}
			request.PlanHash = preview.PlanHash
			request.IdempotencyKey = "panic-store"
			_, err = runtime.Execute(context.Background(), request)
			return err
		}},
		{name: "transaction manager", run: func(t *testing.T) error {
			runtime := newRuntime(t, Options{Transactions: panicTransactionManager{}})
			_, err := runtime.Preview(context.Background(), testRequest())
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run(t)
			if !IsCode(err, CodeInternal) || !safeErrorIs(err, ErrCallbackPanic) {
				t.Fatalf("contained dependency panic = %v", err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("dependency panic leaked recovered value: %v", err)
			}
		})
	}
}

func TestRuntimeContainsNilPanicsAcrossGovernanceBoundaries(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

	t.Run("handler plan", func(t *testing.T) {
		handler := &nilPanicHandler{}
		handler.panicPlan.Store(true)
		runtime, events := newTestRuntime(t, handler, &clock)
		_, err := runtime.Preview(context.Background(), testRequest())
		if !IsCode(err, CodeInternal) || !safeErrorIs(err, ErrCallbackPanic) {
			t.Fatalf("Preview() nil panic error = %v", err)
		}
		events.mu.Lock()
		defer events.mu.Unlock()
		if len(events.events) == 0 || events.events[len(events.events)-1].Decision != "failed" {
			t.Fatalf("nil panic audit = %#v", events.events)
		}
	})

	t.Run("handler execute rollback and retry", func(t *testing.T) {
		handler := &nilPanicHandler{}
		transactions := &rollbackCountingTransactionManager{}
		registry := newTestRegistry(t)
		if err := registry.Register("test", testDescriptor(), handler); err != nil {
			t.Fatal(err)
		}
		runtime := mustRuntime(t, registry, Options{
			Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
			Idempotency: newMemoryIdempotencyStore(), Transactions: transactions,
			Clock: func() time.Time { return clock },
		})
		request := testRequest()
		preview, err := runtime.Preview(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		request.PlanHash = preview.PlanHash
		request.IdempotencyKey = "nil-panic-retry"
		handler.panicExecute.Store(true)
		if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) || !safeErrorIs(err, ErrCallbackPanic) {
			t.Fatalf("Execute() nil panic error = %v", err)
		}
		if got := transactions.rollbacks.Load(); got != 1 {
			t.Fatalf("transaction rollbacks = %d, want 1", got)
		}
		handler.panicExecute.Store(false)
		if _, err := runtime.Execute(context.Background(), request); err != nil {
			t.Fatalf("retry after nil panic = %v", err)
		}
		if got := handler.executions.Load(); got != 1 {
			t.Fatalf("successful executions = %d, want 1", got)
		}
		if got := transactions.rollbacks.Load(); got != 1 {
			t.Fatalf("retry added rollback: %d", got)
		}
	})

	t.Run("authorizer", func(t *testing.T) {
		registry := newTestRegistry(t)
		if err := registry.Register("test", testDescriptor(), &nilPanicHandler{}); err != nil {
			t.Fatal(err)
		}
		runtime := mustRuntime(t, registry, Options{
			Authorizer: nilPanicAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
			Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
			Clock: func() time.Time { return clock },
		})
		_, err := runtime.Preview(context.Background(), testRequest())
		if !IsCode(err, CodeInternal) || !safeErrorIs(err, ErrCallbackPanic) {
			t.Fatalf("authorizer nil panic error = %v", err)
		}
	})

	t.Run("audit failure callback", func(t *testing.T) {
		descriptor := testDescriptor()
		descriptor.Preview = PreviewNone
		descriptor.PreviewSchema = nil
		descriptor.RequiresIdempotency = false
		primary := NewError(CodePreconditionFailed, "handler precondition failed")
		registry := newTestRegistry(t)
		if err := registry.Register("test", descriptor, &testHandler{executeErr: primary}); err != nil {
			t.Fatal(err)
		}
		var callbackCalls atomic.Int32
		runtime := mustRuntime(t, registry, Options{
			Authorizer: testAuthorizer{}, Audit: failingAuditHook{}, Plans: newMemoryPlanStore(),
			Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
			Clock: func() time.Time { return clock }, AuditTimeout: 50 * time.Millisecond,
			AuditFailure: func(context.Context, error, audit.Event) {
				callbackCalls.Add(1)
				panic(nil)
			},
		})
		_, err := runtime.Execute(context.Background(), testRequest())
		if !IsCode(err, CodePreconditionFailed) || !safeErrorIs(err, primary) {
			t.Fatalf("audit callback replaced primary failure: %v", err)
		}
		if got := callbackCalls.Load(); got != 1 {
			t.Fatalf("AuditFailure calls = %d, want 1", got)
		}
	})
}

func TestRuntimeHandlesTypedNilErrorsFromCallbacks(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		run  func(*testing.T) error
	}{
		{name: "plan", run: func(t *testing.T) error {
			runtime, _ := newTestRuntime(t, &typedNilHandler{plan: true}, &clock)
			_, err := runtime.Preview(context.Background(), testRequest())
			return err
		}},
		{name: "execute", run: func(t *testing.T) error {
			handler := &typedNilHandler{}
			runtime, _ := newTestRuntime(t, handler, &clock)
			request := testRequest()
			preview, err := runtime.Preview(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			handler.execute = true
			request.PlanHash = preview.PlanHash
			request.IdempotencyKey = "typed-nil-execute"
			_, err = runtime.Execute(context.Background(), request)
			return err
		}},
		{name: "authorizer", run: func(t *testing.T) error {
			registry := newTestRegistry(t)
			if err := registry.Register("test", testDescriptor(), &testHandler{}); err != nil {
				t.Fatal(err)
			}
			runtime := mustRuntime(t, registry, Options{
				Authorizer: typedNilAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
				Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{}, Clock: func() time.Time { return clock },
			})
			_, err := runtime.Preview(context.Background(), testRequest())
			return err
		}},
		{name: "plan store", run: func(t *testing.T) error {
			runtime := newTestRuntimeWithOptions(t, &testHandler{}, &clock, testsupport.DiscardAudit{}, typedNilPlanStore{})
			_, err := runtime.Preview(context.Background(), testRequest())
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(t); !IsCode(err, CodeInternal) {
				t.Fatalf("typed-nil callback error = %v", err)
			}
		})
	}
}

func TestAuditFailureCallbackCannotReplaceOrBlockPrimaryFailure(t *testing.T) {
	run := func(t *testing.T, callback func(context.Context, error, audit.Event), release func()) {
		t.Helper()
		descriptor := testDescriptor()
		descriptor.Preview = PreviewNone
		descriptor.PreviewSchema = nil
		descriptor.RequiresIdempotency = false
		registry := newTestRegistry(t)
		primary := NewError(CodePreconditionFailed, "handler precondition failed")
		if err := registry.Register("test", descriptor, &testHandler{executeErr: primary}); err != nil {
			t.Fatal(err)
		}
		runtime := mustRuntime(t, registry, Options{
			Authorizer: testAuthorizer{}, Audit: failingAuditHook{}, Plans: newMemoryPlanStore(),
			Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
			AuditTimeout: 15 * time.Millisecond, AuditFailure: callback,
		})
		started := time.Now()
		_, err := runtime.Execute(context.Background(), testRequest())
		elapsed := time.Since(started)
		if release != nil {
			release()
		}
		if !IsCode(err, CodePreconditionFailed) || !safeErrorIs(err, primary) {
			t.Fatalf("primary failure was replaced: %v", err)
		}
		if elapsed > actionRuntimeSynchronizationTimeout {
			t.Fatalf("AuditFailure did not return within the test synchronization timeout: %v", elapsed)
		}
		if strings.Contains(err.Error(), "audit-failure-secret") {
			t.Fatalf("AuditFailure panic leaked: %v", err)
		}
	}

	t.Run("panic", func(t *testing.T) {
		run(t, func(context.Context, error, audit.Event) {
			panic(hostileStringer{secret: "audit-failure-secret"})
		}, nil)
	})
	t.Run("block", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		exited := make(chan struct{})
		var once sync.Once
		run(t, func(context.Context, error, audit.Event) {
			defer close(exited)
			once.Do(func() { close(entered) })
			<-release
		}, func() {
			select {
			case <-entered:
			case <-time.After(actionRuntimeSynchronizationTimeout):
				t.Fatal("AuditFailure callback was not invoked")
			}
			close(release)
			select {
			case <-exited:
			case <-time.After(actionRuntimeSynchronizationTimeout):
				t.Fatal("AuditFailure callback did not exit after release")
			}
		})
	})
}

func TestAuditFailureReceivesIndependentDeadlineAndCancellation(t *testing.T) {
	deadlineSeen := make(chan bool, 1)
	initialError := make(chan error, 1)
	cancellation := make(chan error, 1)
	runtime := &Engine{
		auditTimeout: 20 * time.Millisecond,
		auditFailure: func(ctx context.Context, _ error, _ audit.Event) {
			_, ok := ctx.Deadline()
			deadlineSeen <- ok
			initialError <- ctx.Err()
			<-ctx.Done()
			cancellation <- ctx.Err()
		},
	}

	started := time.Now()
	runtime.reportAuditFailure(errors.New("audit unavailable"), audit.Event{})
	if elapsed := time.Since(started); elapsed > actionRuntimeSynchronizationTimeout {
		t.Fatalf("AuditFailure did not return within the test synchronization timeout: %v", elapsed)
	}
	if !<-deadlineSeen {
		t.Fatal("AuditFailure context has no deadline")
	}
	if err := <-initialError; err != nil {
		t.Fatalf("AuditFailure context started canceled: %v", err)
	}
	if err := <-cancellation; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AuditFailure context error = %v, want DeadlineExceeded", err)
	}
}

func TestAuditFailureMayRunConcurrently(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	runtime := &Engine{
		auditTimeout: time.Second,
		auditFailure: func(ctx context.Context, _ error, _ audit.Event) {
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
		},
	}

	var calls sync.WaitGroup
	calls.Add(2)
	for range 2 {
		go func() {
			defer calls.Done()
			runtime.reportAuditFailure(errors.New("audit unavailable"), audit.Event{})
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(actionRuntimeSynchronizationTimeout):
			t.Fatal("AuditFailure callbacks did not overlap")
		}
	}
	close(release)
	calls.Wait()
}

func TestAuditFailureReceivesDefensiveEventCopy(t *testing.T) {
	event := audit.Event{
		Impact:     &audit.Impact{Resources: []string{"resource:original"}},
		ResultRefs: []audit.Reference{{Kind: "counter", ID: "original"}},
	}
	runtime := &Engine{
		auditTimeout: time.Second,
		auditFailure: func(_ context.Context, _ error, received audit.Event) {
			received.Impact.Resources[0] = "resource:mutated"
			received.ResultRefs[0].ID = "mutated"
		},
	}
	runtime.reportAuditFailure(errors.New("audit unavailable"), event)
	if got := event.Impact.Resources[0]; got != "resource:original" {
		t.Fatalf("AuditFailure mutated Impact resource = %q", got)
	}
	if got := event.ResultRefs[0].ID; got != "original" {
		t.Fatalf("AuditFailure mutated Result reference = %q", got)
	}
}

func TestAuditFailureCannotReplaceSuccessfulReplay(t *testing.T) {
	descriptor := testDescriptor()
	descriptor.Preview = PreviewNone
	descriptor.PreviewSchema = nil
	registry := newTestRegistry(t)
	if err := registry.Register("test", descriptor, &testHandler{}); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	exited := make(chan struct{})
	var once sync.Once
	runtime := mustRuntime(t, registry, Options{
		Authorizer: testAuthorizer{}, Audit: replayFailingAuditHook{}, Plans: newMemoryPlanStore(),
		Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{}, AuditTimeout: 15 * time.Millisecond,
		AuditFailure: func(context.Context, error, audit.Event) {
			defer close(exited)
			once.Do(func() { close(entered) })
			<-release
		},
	})
	request := testRequest()
	request.IdempotencyKey = "successful-replay"
	first, err := runtime.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	replayed, err := runtime.Execute(context.Background(), request)
	elapsed := time.Since(started)
	select {
	case <-entered:
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("replay audit failure was not reported")
	}
	close(release)
	select {
	case <-exited:
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("replay AuditFailure callback did not exit after release")
	}
	if err != nil || string(replayed.Data) != string(first.Data) {
		t.Fatalf("successful replay was replaced: first=%s replay=%s error=%v", first.Data, replayed.Data, err)
	}
	if elapsed > actionRuntimeSynchronizationTimeout {
		t.Fatalf("successful replay did not return within the test synchronization timeout: %v", elapsed)
	}
}

func TestRuntimeValidatesAuthorizationDecisionsBeforePlanning(t *testing.T) {
	base := func() authz.Decision { return authz.Decision{Allowed: true, Fingerprint: "policy-v1"} }
	tests := []struct {
		name   string
		mutate func(*authz.Decision)
	}{
		{name: "missing fingerprint", mutate: func(decision *authz.Decision) { decision.Fingerprint = "" }},
		{name: "oversized fingerprint", mutate: func(decision *authz.Decision) {
			decision.Fingerprint = strings.Repeat("f", authz.MaxFingerprintRunes+1)
		}},
		{name: "control fingerprint", mutate: func(decision *authz.Decision) { decision.Fingerprint = "policy\nv1" }},
		{name: "mismatched permission", mutate: func(decision *authz.Decision) { decision.RequiredPermission = "other.execute" }},
		{name: "negative constraint", mutate: func(decision *authz.Decision) { decision.Constraints.MaxRows = -1 }},
		{name: "allowed error code", mutate: func(decision *authz.Decision) { decision.Code = CodeAuthzDenied }},
		{name: "malformed error code", mutate: func(decision *authz.Decision) { decision.Allowed = false; decision.Code = "Denied!" }},
		{name: "control reason", mutate: func(decision *authz.Decision) { decision.Reason = "allowed\nbecause" }},
		{name: "oversized reason", mutate: func(decision *authz.Decision) { decision.Reason = strings.Repeat("r", authz.MaxDecisionReasonRunes+1) }},
		{name: "denied constraint", mutate: func(decision *authz.Decision) { decision.Allowed = false; decision.Constraints.MaxRows = 1 }},
		{name: "denied missing fingerprint", mutate: func(decision *authz.Decision) { decision.Allowed = false; decision.Fingerprint = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := base()
			test.mutate(&decision)
			handler := &countingPlanHandler{}
			registry := newTestRegistry(t)
			if err := registry.Register("test", testDescriptor(), handler); err != nil {
				t.Fatal(err)
			}
			runtime := mustRuntime(t, registry, Options{
				Authorizer: fixedDecisionAuthorizer{decision: decision}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
				Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
			})
			if _, err := runtime.Preview(context.Background(), testRequest()); !IsCode(err, CodeInternal) {
				t.Fatalf("invalid decision error = %v", err)
			}
			if handler.calls.Load() != 0 {
				t.Fatalf("invalid decision reached Handler.Plan %d time(s)", handler.calls.Load())
			}
		})
	}
}

func TestRuntimeRejectsInvalidResultMetadata(t *testing.T) {
	references := make([]audit.Reference, audit.MaxReferences+1)
	for index := range references {
		references[index] = audit.Reference{Kind: "run", ID: "id" + strings.Repeat("x", index)}
	}
	tests := []struct {
		name   string
		result Result
	}{
		{name: "oversized summary", result: Result{Summary: strings.Repeat("s", audit.MaxSummaryRunes+1)}},
		{name: "control summary", result: Result{Summary: "finished\nnow"}},
		{name: "invalid UTF-8 summary", result: Result{Summary: string([]byte{0xff})}},
		{name: "too many references", result: Result{References: references}},
		{name: "empty reference kind", result: Result{References: []audit.Reference{{ID: "one"}}}},
		{name: "oversized reference id", result: Result{References: []audit.Reference{{Kind: "run", ID: strings.Repeat("i", audit.MaxIDRunes+1)}}}},
		{name: "whitespace reference kind", result: Result{References: []audit.Reference{{Kind: "customer record", ID: "one"}}}},
		{name: "whitespace reference id", result: Result{References: []audit.Reference{{Kind: "customer", ID: "record one"}}}},
		{name: "duplicate reference", result: Result{References: []audit.Reference{{Kind: "run", ID: "one"}, {Kind: "run", ID: "one"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &fixedResultHandler{result: test.result}
			runtime, _ := newTestRuntime(t, handler, pointerToTime(time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)))
			request := testRequest()
			preview, err := runtime.Preview(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			request.PlanHash = preview.PlanHash
			request.IdempotencyKey = "invalid-result-" + strings.ReplaceAll(test.name, " ", "-")
			if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) {
				t.Fatalf("invalid Result error = %v", err)
			}
		})
	}
}

func TestRuntimePreflightsMetadataCollectionSizesBeforeOwnership(t *testing.T) {
	clock := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

	t.Run("handler impact", func(t *testing.T) {
		handler := &testHandler{resources: make([]string, audit.MaxResources+1)}
		runtime, _ := newTestRuntime(t, handler, &clock)
		if _, err := runtime.Preview(context.Background(), testRequest()); !IsCode(err, CodeInternal) {
			t.Fatalf("oversized handler impact error = %v", err)
		}
	})

	t.Run("inline handler impact", func(t *testing.T) {
		fixture := newJSONBudgetRuntime(t, jsonBudgetHandler{plan: PlanData{
			Payload: json.RawMessage(`{}`), Summary: json.RawMessage(`{}`),
			Impact: authz.Impact{Resources: make([]string, audit.MaxResources+1)},
		}})
		request := testRequest()
		request.IdempotencyKey = "oversized-inline-handler-impact"
		if _, err := fixture.runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) {
			t.Fatalf("oversized inline handler impact error = %v", err)
		}
		if fixture.handler.executeCalls != 0 {
			t.Fatalf("invalid inline impact reached Execute %d time(s)", fixture.handler.executeCalls)
		}
	})

	t.Run("handler result references", func(t *testing.T) {
		fixture := newJSONBudgetRuntime(t, jsonBudgetHandler{result: Result{
			Data: json.RawMessage(`{}`), References: make([]audit.Reference, audit.MaxReferences+1),
		}})
		request := testRequest()
		request.IdempotencyKey = "oversized-handler-result-references"
		if _, err := fixture.runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) {
			t.Fatalf("oversized handler result references error = %v", err)
		}
		if fixture.idempotency.completes != 0 {
			t.Fatal("invalid handler result reached idempotency completion")
		}
	})

	t.Run("stored plan impact", func(t *testing.T) {
		store := fixedMetadataPlanStore{plan: Plan{
			Payload: json.RawMessage(`{}`),
			Impact:  authz.Impact{Resources: make([]string, audit.MaxResources+1)},
		}}
		runtime := newTestRuntimeWithOptions(t, &testHandler{}, &clock, testsupport.DiscardAudit{}, store)
		request := testRequest()
		request.PlanHash = "sha256:" + strings.Repeat("a", 64)
		request.IdempotencyKey = "oversized-stored-plan-impact"
		if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodePlanStale) {
			t.Fatalf("oversized stored plan impact error = %v", err)
		}
	})

	for _, test := range []struct {
		name   string
		record IdempotencyRecord
	}{
		{name: "stored idempotency impact", record: IdempotencyRecord{Impact: authz.Impact{Resources: make([]string, audit.MaxResources+1)}}},
		{name: "stored idempotency references", record: IdempotencyRecord{Result: Result{References: make([]audit.Reference, audit.MaxReferences+1)}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := newTestRegistry(t)
			if err := registry.Register("test", testDescriptor(), &testHandler{}); err != nil {
				t.Fatal(err)
			}
			store := &fixedMetadataIdempotencyStore{record: test.record}
			runtime := mustRuntime(t, registry, Options{
				Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
				Idempotency: store, Transactions: confirmedTransactionManager{}, Clock: func() time.Time { return clock },
			})
			request := testRequest()
			preview, err := runtime.Preview(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			request.PlanHash = preview.PlanHash
			request.IdempotencyKey = "oversized-stored-idempotency-metadata"
			if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) {
				t.Fatalf("oversized stored idempotency metadata error = %v", err)
			}
			if store.lookups != 2 {
				t.Fatalf("idempotency lookups = %d, want 2", store.lookups)
			}
		})
	}
}

func TestMetadataCollectionSizePreflightDoesNotAllocate(t *testing.T) {
	exactImpact := authz.Impact{Resources: make([]string, audit.MaxResources)}
	exactResult := Result{References: make([]audit.Reference, audit.MaxReferences)}
	if err := validateImpactCollectionSize(exactImpact); err != nil {
		t.Fatalf("exact impact collection limit = %v", err)
	}
	if err := validateResultCollectionSize(exactResult); err != nil {
		t.Fatalf("exact result collection limit = %v", err)
	}
	impact := authz.Impact{Resources: make([]string, audit.MaxResources+1)}
	result := Result{References: make([]audit.Reference, audit.MaxReferences+1)}
	if validateImpactCollectionSize(impact) == nil || validateResultCollectionSize(result) == nil {
		t.Fatal("oversized metadata collection passed preflight")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_ = validateImpactCollectionSize(impact)
		_ = validateResultCollectionSize(result)
	}); allocations != 0 {
		t.Fatalf("metadata collection preflight allocations = %v", allocations)
	}
}

func TestMetadataAuditCannotRetainResultOrImpactDetails(t *testing.T) {
	descriptor := testDescriptor()
	descriptor.Preview = PreviewNone
	descriptor.PreviewSchema = nil
	descriptor.AuditLevel = AuditMetadata
	descriptor.RequiresIdempotency = false
	registry := newTestRegistry(t)
	handler := &fixedResultHandler{result: Result{
		Summary:    "customer record changed",
		References: []audit.Reference{{Kind: "customer", ID: "customer-secret"}},
	}}
	if err := registry.Register("test", descriptor, handler); err != nil {
		t.Fatal(err)
	}
	events := &collectingAudit{}
	runtime := mustRuntime(t, registry, Options{
		Authorizer: testAuthorizer{}, Audit: events, Plans: newMemoryPlanStore(),
		Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
	})
	result, err := runtime.Execute(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary == "" || len(result.References) != 1 {
		t.Fatalf("caller result was unexpectedly stripped: %#v", result)
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	event := events.events[len(events.events)-1]
	if event.ResultSummary != "" || event.Impact != nil || len(event.ResultRefs) != 0 {
		t.Fatalf("metadata audit retained detailed data: %#v", event)
	}
}

func TestRuntimeCanonicalizesInputBeforePlan(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	handler := &capturingInputHandler{}
	runtime, _ := newTestRuntime(t, handler, &clock)
	var hashes []string
	for _, input := range []json.RawMessage{
		json.RawMessage(`{"value":1}`),
		json.RawMessage(`{ "value" : 1.0 }`),
		json.RawMessage(`{"value":1e0}`),
	} {
		request := testRequest()
		request.Input = input
		preview, err := runtime.Preview(context.Background(), request)
		if err != nil {
			t.Fatalf("Preview(%s) error = %v", input, err)
		}
		hashes = append(hashes, preview.PlanHash)
	}
	if hashes[0] != hashes[1] || hashes[1] != hashes[2] {
		t.Fatalf("equivalent JSON inputs produced different plans: %v", hashes)
	}
	handler.mu.Lock()
	inputs := append([]string(nil), handler.inputs...)
	handler.mu.Unlock()
	for _, input := range inputs {
		if input != `{"value":1}` {
			t.Fatalf("Handler.Plan received non-canonical input %q", input)
		}
	}

	before := len(inputs)
	for _, test := range []struct {
		input json.RawMessage
		code  string
	}{
		{input: json.RawMessage(`{"value":1e+}`), code: CodeValidationFailed},
		{input: json.RawMessage(`{"value":` + strings.Repeat("1", MaxJSONNumberBytes+1) + `}`), code: CodeLimitExceeded},
	} {
		request := testRequest()
		request.Input = test.input
		if _, err := runtime.Preview(context.Background(), request); !IsCode(err, test.code) {
			t.Fatalf("invalid numeric input %q error = %v, want %s", test.input[:min(len(test.input), 32)], err, test.code)
		}
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.inputs) != before {
		t.Fatalf("invalid numeric input reached Handler.Plan: calls=%d want=%d", len(handler.inputs), before)
	}
}

func TestPreviewPlanAndRequiredAuditShareTransaction(t *testing.T) {
	plans := newMemoryPlanStore()
	transactions := &rollbackPlanTransaction{plans: plans}
	auditErr := errors.New("preview audit unavailable")
	hook := transactionalPreviewAudit{err: auditErr}
	failures := make(chan error, 1)
	authorizer := &countingAuthorizer{}
	registry := newTestRegistry(t)
	if err := registry.Register("test", testDescriptor(), &testHandler{}); err != nil {
		t.Fatal(err)
	}
	runtime := mustRuntime(t, registry, Options{
		Authorizer: authorizer, Audit: hook, Plans: plans, Idempotency: newMemoryIdempotencyStore(), Transactions: transactions,
		AuditFailure: func(_ context.Context, err error, _ audit.Event) { failures <- err },
	})
	if _, err := runtime.Preview(context.Background(), testRequest()); !IsCode(err, CodeInternal) || !safeErrorIs(err, auditErr) {
		t.Fatalf("Preview() error = %v", err)
	}
	plans.mu.RLock()
	remaining := len(plans.plans)
	plans.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("failed required audit left %d durable plan(s)", remaining)
	}
	if transactions.calls.Load() != 1 {
		t.Fatalf("Preview transaction calls = %d", transactions.calls.Load())
	}
	if authorizer.calls.Load() < 4 {
		t.Fatalf("Preview did not reauthorize intent and impact in transaction: calls=%d", authorizer.calls.Load())
	}
	select {
	case err := <-failures:
		if !errors.Is(err, auditErr) {
			t.Fatalf("AuditFailure error = %v", err)
		}
	default:
		t.Fatal("required audit failure was not reported")
	}
}

func TestRegistryRevokeIsSynchronousIdempotentAndDrainable(t *testing.T) {
	registry := newTestRegistry(t)
	if err := registry.Register("test", testDescriptor(), &testHandler{}); err != nil {
		t.Fatal(err)
	}
	executionCtx, release, revoked := registry.acquireLease(context.Background())
	if revoked {
		t.Fatal("fresh registry rejected a lease")
	}
	if err := registry.Revoke(); err != nil {
		t.Fatal(err)
	}
	if err := registry.Revoke(); err != nil {
		t.Fatalf("idempotent Revoke() error = %v", err)
	}
	select {
	case <-executionCtx.Done():
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("Revoke did not synchronously cancel active execution")
	}
	if _, ok := registry.Resolve("test.execute"); ok || registry.available() {
		t.Fatal("Revoke retained catalog bindings or availability")
	}

	var failures atomic.Int32
	var group sync.WaitGroup
	for range 32 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, _, acquiredRelease, _, isRevoked := registry.acquire(context.Background(), "test.execute")
			if acquiredRelease != nil {
				acquiredRelease()
			}
			if !isRevoked {
				failures.Add(1)
			}
		}()
	}
	group.Wait()
	if failures.Load() != 0 {
		t.Fatalf("%d acquisition(s) crossed the revoke boundary", failures.Load())
	}

	resetCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := registry.Reset(resetCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Reset before release error = %v", err)
	}
	release()
	if err := registry.Reset(context.Background()); err != nil {
		t.Fatalf("Reset after drain error = %v", err)
	}
}

func TestRegistryAcquireCannotCrossRevokeWhileCreatingContext(t *testing.T) {
	registry := newTestRegistry(t)
	parent := &blockingDoneContext{
		Context: context.Background(),
		entered: make(chan struct{}),
		proceed: make(chan struct{}),
	}
	type acquisition struct {
		ctx     context.Context
		release func()
		revoked bool
	}
	result := make(chan acquisition, 1)
	go func() {
		_, executionCtx, release, _, revoked := registry.acquire(parent, "test.execute")
		result <- acquisition{ctx: executionCtx, release: release, revoked: revoked}
	}()
	select {
	case <-parent.entered:
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("acquire did not enter parent context construction")
	}

	revokeResult := make(chan error, 1)
	go func() { revokeResult <- registry.Revoke() }()
	select {
	case err := <-revokeResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(actionRuntimeSynchronizationTimeout):
		close(parent.proceed)
		t.Fatal("Revoke waited on unregistered context construction")
	}
	close(parent.proceed)

	select {
	case acquired := <-result:
		if !acquired.revoked || acquired.ctx != nil || acquired.release != nil {
			t.Fatalf("acquire crossed completed Revoke: %#v", acquired)
		}
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("acquire did not observe completed Revoke")
	}
}

type hostileStringer struct{ secret string }

func (value hostileStringer) String() string { panic("hostile formatter invoked") }

type hostileError struct{ secret string }

func (*hostileError) Error() string { panic("hostile error formatter invoked") }
func (*hostileError) Unwrap() error { panic("hostile unwrap invoked") }

type hostileMatcher struct {
	nested   error
	asCalled atomic.Bool
	isCalled atomic.Bool
}

func (*hostileMatcher) Error() string { return "hostile matcher" }
func (matcher *hostileMatcher) As(any) bool {
	matcher.asCalled.Store(true)
	panic("custom As invoked")
}
func (matcher *hostileMatcher) Is(error) bool {
	matcher.isCalled.Store(true)
	panic("custom Is invoked")
}
func (matcher *hostileMatcher) Unwrap() error { return matcher.nested }

type cyclicError struct{ next error }

func (*cyclicError) Error() string     { return "cyclic error" }
func (err *cyclicError) Unwrap() error { return err.next }

type blockingDoneContext struct {
	context.Context
	entered chan struct{}
	proceed chan struct{}
	once    sync.Once
}

func (ctx *blockingDoneContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.entered) })
	<-ctx.proceed
	return ctx.Context.Done()
}

type hostileFailureHandler struct {
	panicValue any
	err        error
}

func (handler hostileFailureHandler) Plan(context.Context, Request) (PlanData, error) {
	if handler.panicValue != nil {
		panic(handler.panicValue)
	}
	return PlanData{}, handler.err
}

func (hostileFailureHandler) Execute(context.Context, Plan) (Result, error) {
	return Result{}, nil
}

type panicAuthorizer struct{}

func (panicAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	panic(hostileStringer{secret: "authorizer-secret"})
}

type panicAuditHook struct{}

func (panicAuditHook) Record(context.Context, audit.Event) error {
	panic(hostileStringer{secret: "audit-secret"})
}

type panicPlanStore struct{}

func (panicPlanStore) Save(context.Context, Plan) error {
	panic(hostileStringer{secret: "plan-secret"})
}
func (panicPlanStore) Get(context.Context, string) (Plan, error) {
	panic(hostileStringer{secret: "plan-secret"})
}
func (panicPlanStore) DeleteExpired(context.Context, time.Time) (int64, error) {
	panic(hostileStringer{secret: "plan-secret"})
}

type panicIdempotencyStore struct{}

func (panicIdempotencyStore) Lookup(context.Context, IdempotencyRecord) (*IdempotencyRecord, error) {
	panic(hostileStringer{secret: "idempotency-secret"})
}
func (panicIdempotencyStore) Reserve(context.Context, IdempotencyRecord) (*IdempotencyRecord, error) {
	panic(hostileStringer{secret: "idempotency-secret"})
}
func (panicIdempotencyStore) Complete(context.Context, IdempotencyRecord) error {
	panic(hostileStringer{secret: "idempotency-secret"})
}
func (panicIdempotencyStore) Abort(context.Context, IdempotencyRecord) error {
	panic(hostileStringer{secret: "idempotency-secret"})
}

type panicTransactionManager struct{}

func (panicTransactionManager) WithinTransaction(context.Context, func(context.Context) error) error {
	panic(hostileStringer{secret: "transaction-secret"})
}

type nilPanicHandler struct {
	panicPlan    atomic.Bool
	panicExecute atomic.Bool
	executions   atomic.Int32
}

func (handler *nilPanicHandler) Plan(context.Context, Request) (PlanData, error) {
	if handler.panicPlan.Load() {
		panic(nil)
	}
	return PlanData{
		Payload: json.RawMessage(`{"value":1}`), Summary: json.RawMessage(`{"matched_rows":1}`),
		Impact: authz.Impact{Rows: 1},
	}, nil
}

func (handler *nilPanicHandler) Execute(context.Context, Plan) (Result, error) {
	if handler.panicExecute.Load() {
		panic(nil)
	}
	handler.executions.Add(1)
	return Result{Data: json.RawMessage(`{"ok":true}`)}, nil
}

type nilPanicAuthorizer struct{}

func (nilPanicAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	panic(nil)
}

type rollbackCountingTransactionManager struct{ rollbacks atomic.Int32 }

func (manager *rollbackCountingTransactionManager) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	err := operation(ctx)
	if err != nil {
		manager.rollbacks.Add(1)
	}
	return confirmTestRollback(err)
}

type typedNilHandler struct {
	plan    bool
	execute bool
}

func (handler *typedNilHandler) Plan(context.Context, Request) (PlanData, error) {
	if handler.plan {
		var err *Error
		return PlanData{}, err
	}
	return PlanData{Payload: json.RawMessage(`{"value":1}`), Summary: json.RawMessage(`{"matched_rows":1}`), Impact: authz.Impact{Rows: 1}}, nil
}

func (handler *typedNilHandler) Execute(context.Context, Plan) (Result, error) {
	if handler.execute {
		var err *Error
		return Result{}, err
	}
	return Result{Data: json.RawMessage(`{"ok":true}`)}, nil
}

type typedNilAuthorizer struct{}

func (typedNilAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	var err *Error
	return authz.Decision{}, err
}

type typedNilPlanStore struct{}

func (typedNilPlanStore) Save(context.Context, Plan) error {
	var err *Error
	return err
}
func (typedNilPlanStore) Get(context.Context, string) (Plan, error) {
	var err *Error
	return Plan{}, err
}
func (typedNilPlanStore) DeleteExpired(context.Context, time.Time) (int64, error) {
	var err *Error
	return 0, err
}

type failingAuditHook struct{}

func (failingAuditHook) Record(context.Context, audit.Event) error {
	return errors.New("audit sink unavailable")
}

type replayFailingAuditHook struct{}

func (replayFailingAuditHook) Record(_ context.Context, event audit.Event) error {
	if event.Decision == "idempotent_replay" {
		return errors.New("replay audit sink unavailable")
	}
	return nil
}

type fixedDecisionAuthorizer struct{ decision authz.Decision }

func (authorizer fixedDecisionAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authorizer.decision, nil
}

type countingPlanHandler struct{ calls atomic.Int32 }

func (handler *countingPlanHandler) Plan(context.Context, Request) (PlanData, error) {
	handler.calls.Add(1)
	return PlanData{Payload: json.RawMessage(`{"value":1}`), Summary: json.RawMessage(`{"matched_rows":1}`), Impact: authz.Impact{Rows: 1}}, nil
}

func (*countingPlanHandler) Execute(context.Context, Plan) (Result, error) {
	return Result{Data: json.RawMessage(`{"ok":true}`)}, nil
}

type fixedResultHandler struct{ result Result }

func (*fixedResultHandler) Plan(context.Context, Request) (PlanData, error) {
	return PlanData{Payload: json.RawMessage(`{"value":1}`), Summary: json.RawMessage(`{"matched_rows":1}`), Impact: authz.Impact{Rows: 1}}, nil
}

func (handler *fixedResultHandler) Execute(context.Context, Plan) (Result, error) {
	result := cloneResult(handler.result)
	result.Data = json.RawMessage(`{"ok":true}`)
	return result, nil
}

type fixedMetadataPlanStore struct{ plan Plan }

func (store fixedMetadataPlanStore) Save(context.Context, Plan) error { return nil }
func (store fixedMetadataPlanStore) Get(context.Context, string) (Plan, error) {
	return store.plan, nil
}
func (fixedMetadataPlanStore) DeleteExpired(context.Context, time.Time) (int64, error) {
	return 0, nil
}

type fixedMetadataIdempotencyStore struct {
	record  IdempotencyRecord
	lookups int
}

func (store *fixedMetadataIdempotencyStore) Lookup(context.Context, IdempotencyRecord) (*IdempotencyRecord, error) {
	store.lookups++
	return &store.record, nil
}

func (*fixedMetadataIdempotencyStore) Reserve(context.Context, IdempotencyRecord) (*IdempotencyRecord, error) {
	return nil, errors.New("unexpected idempotency reservation")
}

func (*fixedMetadataIdempotencyStore) Complete(context.Context, IdempotencyRecord) error {
	return errors.New("unexpected idempotency completion")
}

func (*fixedMetadataIdempotencyStore) Abort(context.Context, IdempotencyRecord) error {
	return errors.New("unexpected idempotency abort")
}

type capturingInputHandler struct {
	mu     sync.Mutex
	inputs []string
}

func (handler *capturingInputHandler) Plan(_ context.Context, request Request) (PlanData, error) {
	handler.mu.Lock()
	handler.inputs = append(handler.inputs, string(request.Input))
	handler.mu.Unlock()
	return PlanData{Payload: json.RawMessage(`{"value":1}`), Summary: json.RawMessage(`{"matched_rows":1}`), Impact: authz.Impact{Rows: 1}}, nil
}

func (*capturingInputHandler) Execute(context.Context, Plan) (Result, error) {
	return Result{Data: json.RawMessage(`{"ok":true}`)}, nil
}

type transactionMarker struct{}

type rollbackPlanTransaction struct {
	plans *testPlanStore
	calls atomic.Int32
}

func (manager *rollbackPlanTransaction) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	manager.calls.Add(1)
	manager.plans.mu.RLock()
	snapshot := make(map[string]Plan, len(manager.plans.plans))
	for hash, plan := range manager.plans.plans {
		snapshot[hash] = clonePlan(plan)
	}
	manager.plans.mu.RUnlock()
	err := operation(context.WithValue(ctx, transactionMarker{}, true))
	if err != nil {
		manager.plans.mu.Lock()
		manager.plans.plans = snapshot
		manager.plans.mu.Unlock()
	}
	return confirmTestRollback(err)
}

type transactionalPreviewAudit struct{ err error }

func (hook transactionalPreviewAudit) Record(ctx context.Context, event audit.Event) error {
	if event.Decision != "previewed" {
		return nil
	}
	if marker, _ := ctx.Value(transactionMarker{}).(bool); !marker {
		return errors.New("preview audit was outside its transaction")
	}
	return hook.err
}

type countingAuthorizer struct{ calls atomic.Int32 }

func (authorizer *countingAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	authorizer.calls.Add(1)
	return authz.Decision{Allowed: true, Fingerprint: "counted-policy-v1"}, nil
}

func mustRuntime(t *testing.T, registry *Registry, options Options) *Engine {
	t.Helper()
	runtime, err := New(registry, options)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func pointerToTime(value time.Time) *time.Time { return &value }
