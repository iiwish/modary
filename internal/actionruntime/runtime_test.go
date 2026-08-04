package actionruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	. "github.com/iiwish/modary/action"
	. "github.com/iiwish/modary/internal/actionpersistence"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/testsupport"
	"github.com/iiwish/modary/scope"
)

const actionRuntimeSynchronizationTimeout = 5 * time.Second

func TestRuntimePreviewExecuteAndIdempotency(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	handler := &testHandler{}
	runtime, events := newTestRuntime(t, handler, &clock)
	request := testRequest()

	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	second, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatalf("second Preview() error = %v", err)
	}
	if preview.PlanHash != second.PlanHash {
		t.Fatalf("stable preview hash mismatch: %s != %s", preview.PlanHash, second.PlanHash)
	}

	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "run-once"
	result, err := runtime.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(result.Data) != `{"ok":true}` {
		t.Fatalf("result = %s", result.Data)
	}
	if handler.executions != 1 {
		t.Fatalf("executions = %d", handler.executions)
	}
	if _, err := runtime.Execute(context.Background(), request); err != nil {
		t.Fatalf("idempotent replay error = %v", err)
	}
	if handler.executions != 1 {
		t.Fatalf("idempotent replay executed handler %d times", handler.executions)
	}
	if len(events.events) < 4 {
		t.Fatalf("audit events = %d", len(events.events))
	}
}

func TestRuntimeRejectsStalePlan(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, _ := newTestRuntime(t, &testHandler{}, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(6 * time.Minute)
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "stale"
	_, err = runtime.Execute(context.Background(), request)
	if !IsCode(err, CodePlanStale) {
		t.Fatalf("error = %v", err)
	}
}

func TestRuntimeDistinguishesMissingPlanFromStoreFailure(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	request := testRequest()
	request.PlanHash = "sha256:" + strings.Repeat("a", 64)
	request.IdempotencyKey = "plan-errors"

	for _, test := range []struct {
		name  string
		store PlanStore
		code  string
	}{
		{name: "missing", store: newMemoryPlanStore(), code: CodePlanNotFound},
		{name: "operational failure", store: failingPlanStore{err: errors.New("database unavailable")}, code: CodeInternal},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := newTestRuntimeWithOptions(t, &testHandler{}, &clock, testsupport.DiscardAudit{}, test.store)
			if _, err := runtime.Execute(context.Background(), request); !IsCode(err, test.code) {
				t.Fatalf("Execute() error = %v, want %s", err, test.code)
			}
		})
	}
}

func TestRuntimeRejectsNegativePlanDeletionCounts(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	store := &negativeDeletePlanStore{testPlanStore: newMemoryPlanStore()}
	runtime := newTestRuntimeWithOptions(t, &testHandler{}, &clock, testsupport.DiscardAudit{}, store)
	if _, err := runtime.Preview(context.Background(), testRequest()); !IsCode(err, CodeInternal) {
		t.Fatalf("Preview() negative deletion count error = %v", err)
	}
	if deleted, err := runtime.CleanupExpiredPlans(context.Background()); deleted != 0 || !IsCode(err, CodeInternal) {
		t.Fatalf("CleanupExpiredPlans() = %d, %v", deleted, err)
	}
}

func TestRuntimeRejectsMalformedStoredIdempotencyRecords(t *testing.T) {
	mutations := []struct {
		name  string
		code  string
		apply func(*IdempotencyRecord)
	}{
		{name: "empty decision fingerprint", code: CodeInternal, apply: func(record *IdempotencyRecord) {
			record.DecisionFingerprint = ""
		}},
		{name: "invalid status", code: CodeInternal, apply: func(record *IdempotencyRecord) {
			record.Status = IdempotencyStatus("unknown")
		}},
		{name: "invalid result", code: CodeInternal, apply: func(record *IdempotencyRecord) {
			record.Result.Data = json.RawMessage(`{"ok":`)
		}},
		{name: "changed decision fingerprint", code: CodePlanStale, apply: func(record *IdempotencyRecord) {
			record.DecisionFingerprint = "policy-v2"
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
			handler := &testHandler{}
			registry := newTestRegistry(t)
			if err := registry.Register("test", testDescriptor(), handler); err != nil {
				t.Fatal(err)
			}
			store := &maliciousIdempotencyStore{}
			plans := newMemoryPlanStore()
			runtime, err := New(registry, Options{
				Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: plans,
				Idempotency: store, Transactions: confirmedTransactionManager{}, Clock: func() time.Time { return clock },
			})
			if err != nil {
				t.Fatal(err)
			}
			request := testRequest()
			preview, err := runtime.Preview(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			registered, ok := registry.resolve(request.ActionID)
			if !ok {
				t.Fatal("registered Action is missing")
			}
			_, inputHash, err := canonicalInput(request.Input)
			if err != nil {
				t.Fatal(err)
			}
			request.PlanHash = preview.PlanHash
			request.IdempotencyKey = "malicious-store"
			store.record = IdempotencyRecord{
				Scope: request.Scope, ActorID: request.Actor.ID, ActorType: request.Actor.Type,
				ActionID: request.ActionID, ActionVersion: registered.Descriptor.Version,
				ContractHash: registered.ContractHash, Channel: request.Channel,
				Key: request.IdempotencyKey, InputHash: inputHash, PlanHash: preview.PlanHash,
				Impact:              authz.Impact{Rows: 1, Resources: []string{"record:1"}},
				DecisionFingerprint: "policy-v1", Status: IdempotencyCompleted,
				Result: Result{Data: json.RawMessage(`{"ok":true}`), Summary: "stored result"},
			}
			test.apply(&store.record)
			if _, err := runtime.Execute(context.Background(), request); !IsCode(err, test.code) {
				t.Fatalf("Execute() error = %v, want %s", err, test.code)
			}
			if handler.executions != 0 {
				t.Fatalf("malformed stored record executed Handler %d times", handler.executions)
			}
		})
	}
}

func TestRuntimeOptionalPreviewBindsExplicitPlanHash(t *testing.T) {
	newRuntime := func(t *testing.T, policy PreviewPolicy, handler Handler, clock *time.Time, plans *testPlanStore) *Engine {
		t.Helper()
		descriptor := testDescriptor()
		descriptor.Preview = policy
		if policy == PreviewNone {
			descriptor.PreviewSchema = nil
		}
		registry := newTestRegistry(t)
		if err := registry.Register("test", descriptor, handler); err != nil {
			t.Fatal(err)
		}
		runtime, err := New(registry, Options{
			Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: plans,
			Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
			Clock: func() time.Time { return *clock }, PlanTTL: 5 * time.Minute,
		})
		if err != nil {
			t.Fatal(err)
		}
		return runtime
	}

	t.Run("explicit hash executes the previewed payload without replanning", func(t *testing.T) {
		clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
		plans := newMemoryPlanStore()
		handler := &trackingPlanHandler{payload: json.RawMessage(`{"value":1}`)}
		runtime := newRuntime(t, PreviewOptional, handler, &clock, plans)
		request := testRequest()
		preview, err := runtime.Preview(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		handler.payload = json.RawMessage(`{"value":2}`)
		request.PlanHash = preview.PlanHash
		request.IdempotencyKey = "optional-bound"
		if _, err := runtime.Execute(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if handler.planCalls != 1 {
			t.Fatalf("Plan() calls = %d, want preview call only", handler.planCalls)
		}
		if got := string(handler.executedPlan.Payload); got != `{"value":1}` {
			t.Fatalf("executed payload = %s, want previewed payload", got)
		}
	})

	t.Run("explicit hash rejects tampering and expiry", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			mutate func(*time.Time, *testPlanStore, string)
		}{
			{name: "tampered", mutate: func(_ *time.Time, plans *testPlanStore, hash string) {
				plans.mu.Lock()
				plan := plans.plans[hash]
				plan.Payload = json.RawMessage(`{"value":999}`)
				plans.plans[hash] = plan
				plans.mu.Unlock()
			}},
			{name: "expired", mutate: func(clock *time.Time, _ *testPlanStore, _ string) {
				*clock = clock.Add(6 * time.Minute)
			}},
		} {
			t.Run(test.name, func(t *testing.T) {
				clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
				plans := newMemoryPlanStore()
				handler := &trackingPlanHandler{payload: json.RawMessage(`{"value":1}`)}
				runtime := newRuntime(t, PreviewOptional, handler, &clock, plans)
				request := testRequest()
				preview, err := runtime.Preview(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				test.mutate(&clock, plans, preview.PlanHash)
				request.PlanHash = preview.PlanHash
				request.IdempotencyKey = "optional-stale"
				if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodePlanStale) {
					t.Fatalf("Execute() error = %v", err)
				}
				if handler.planCalls != 1 {
					t.Fatalf("stale plan triggered replanning: calls = %d", handler.planCalls)
				}
			})
		}
	})

	t.Run("missing optional hash plans inline", func(t *testing.T) {
		clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
		handler := &trackingPlanHandler{}
		runtime := newRuntime(t, PreviewOptional, handler, &clock, newMemoryPlanStore())
		request := testRequest()
		request.IdempotencyKey = "optional-inline"
		if _, err := runtime.Execute(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Execute(context.Background(), request); err != nil {
			t.Fatalf("idempotent replay without plan hash: %v", err)
		}
		if handler.planCalls != 1 {
			t.Fatalf("Plan() calls = %d, want one inline plan across replay", handler.planCalls)
		}
		if handler.executeCalls != 1 {
			t.Fatalf("Execute() calls = %d, want one", handler.executeCalls)
		}
	})

	t.Run("preview-none rejects a supplied hash", func(t *testing.T) {
		clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
		handler := &trackingPlanHandler{}
		runtime := newRuntime(t, PreviewNone, handler, &clock, newMemoryPlanStore())
		request := testRequest()
		request.PlanHash = "sha256:" + strings.Repeat("a", 64)
		request.IdempotencyKey = "none-with-plan"
		if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodePreconditionFailed) {
			t.Fatalf("Execute() error = %v", err)
		}
		if handler.planCalls != 0 {
			t.Fatalf("unsupported hash triggered Plan(): calls = %d", handler.planCalls)
		}
	})

	t.Run("preview-none executes idempotently without a supplied hash", func(t *testing.T) {
		clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
		handler := &trackingPlanHandler{}
		runtime := newRuntime(t, PreviewNone, handler, &clock, newMemoryPlanStore())
		request := testRequest()
		request.IdempotencyKey = "none-inline"
		if _, err := runtime.Execute(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Execute(context.Background(), request); err != nil {
			t.Fatalf("idempotent replay without plan hash: %v", err)
		}
		if handler.planCalls != 1 {
			t.Fatalf("Plan() calls = %d, want one inline plan across replay", handler.planCalls)
		}
		if handler.executeCalls != 1 {
			t.Fatalf("Execute() calls = %d, want one", handler.executeCalls)
		}
	})
}

func TestRuntimeSealsInternalPlansWithAuthorizationProvenance(t *testing.T) {
	for _, policy := range []PreviewPolicy{PreviewNone, PreviewOptional} {
		t.Run(string(policy), func(t *testing.T) {
			clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
			descriptor := testDescriptor()
			descriptor.Preview = policy
			if policy == PreviewNone {
				descriptor.PreviewSchema = nil
			}
			handler := &trackingPlanHandler{}
			registry := newTestRegistry(t)
			if err := registry.Register("test", descriptor, handler); err != nil {
				t.Fatal(err)
			}
			idempotency := newMemoryIdempotencyStore()
			events := &collectingAudit{}
			runtime, err := New(registry, Options{
				Authorizer: testAuthorizer{}, Audit: events, Plans: newMemoryPlanStore(),
				Idempotency: idempotency, Transactions: confirmedTransactionManager{},
				Clock: func() time.Time { return clock }, PlanTTL: 5 * time.Minute,
			})
			if err != nil {
				t.Fatal(err)
			}
			request := testRequest()
			request.IdempotencyKey = "internal-" + string(policy)
			if _, err := runtime.Execute(context.Background(), request); err != nil {
				t.Fatal(err)
			}

			plan := handler.executedPlan
			if plan.DecisionFingerprint != "policy-v1" {
				t.Fatalf("executed plan fingerprint = %q", plan.DecisionFingerprint)
			}
			if err := ValidatePlanRecord(plan); err != nil {
				t.Fatalf("executed plan is not persistable: %v", err)
			}
			computed, err := hashPlan(plan)
			if err != nil || computed != plan.Hash {
				t.Fatalf("executed plan hash = %q, recomputed %q, error %v", plan.Hash, computed, err)
			}
			changed := clonePlan(plan)
			changed.DecisionFingerprint = "policy-v2"
			changedHash, err := hashPlan(changed)
			if err != nil || changedHash == plan.Hash {
				t.Fatalf("authorization provenance did not bind hash: %q / %v", changedHash, err)
			}

			events.mu.Lock()
			auditPlanHash := events.events[len(events.events)-1].PlanHash
			events.mu.Unlock()
			if auditPlanHash != plan.Hash {
				t.Fatalf("audit plan hash = %q, want %q", auditPlanHash, plan.Hash)
			}
			idempotency.mu.Lock()
			for _, record := range idempotency.records {
				if record.PlanHash != plan.Hash || record.DecisionFingerprint != plan.DecisionFingerprint {
					idempotency.mu.Unlock()
					t.Fatalf("idempotency provenance = %#v", record)
				}
			}
			if len(idempotency.records) != 1 {
				idempotency.mu.Unlock()
				t.Fatalf("idempotency record count = %d", len(idempotency.records))
			}
			idempotency.mu.Unlock()
		})
	}
}

func TestConcurrentInlineIdempotencyDoesNotBindGeneratedPlanAsClientConstraint(t *testing.T) {
	descriptor := testDescriptor()
	descriptor.Preview = PreviewNone
	descriptor.PreviewSchema = nil
	handler := &blockingInlineHandler{entered: make(chan struct{}, 1), release: make(chan struct{})}
	registry := newTestRegistry(t)
	if err := registry.Register("test", descriptor, handler); err != nil {
		t.Fatal(err)
	}
	idempotency := &barrierLookupIdempotencyStore{testIdempotencyStore: newMemoryIdempotencyStore(), ready: make(chan struct{})}
	runtime, err := New(registry, Options{
		Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
		Idempotency: idempotency, Transactions: confirmedTransactionManager{},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest()
	request.IdempotencyKey = "concurrent-inline"
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, executeErr := runtime.Execute(context.Background(), request)
			results <- executeErr
		}()
	}
	select {
	case <-handler.entered:
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("winning execution did not enter the Handler")
	}
	var loserErr error
	select {
	case loserErr = <-results:
	case <-time.After(actionRuntimeSynchronizationTimeout):
		close(handler.release)
		<-results
		<-results
		t.Fatal("concurrent duplicate reached the Handler instead of observing the reservation")
	}
	if !IsCode(loserErr, CodeIdempotencyProgress) {
		t.Fatalf("concurrent duplicate error = %v, want %s", loserErr, CodeIdempotencyProgress)
	}
	close(handler.release)
	if winnerErr := <-results; winnerErr != nil {
		t.Fatalf("winning execution error = %v", winnerErr)
	}
	if got := handler.executeCalls.Load(); got != 1 {
		t.Fatalf("Handler Execute() calls = %d, want one", got)
	}
	if got := handler.planCalls.Load(); got != 2 {
		t.Fatalf("Handler Plan() calls = %d, want two independently resolved inline plans", got)
	}
}

func TestRuntimeUsesIntentFingerprintWhenImpactFingerprintIsEmpty(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	registry := newTestRegistry(t)
	if err := registry.Register("test", testDescriptor(), &testHandler{}); err != nil {
		t.Fatal(err)
	}
	runtime, err := New(registry, Options{
		Authorizer: intentFingerprintAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
		Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
		Clock: func() time.Time { return clock }, PlanTTL: 5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "intent-fingerprint"
	if _, err := runtime.Execute(context.Background(), request); err != nil {
		t.Fatalf("Execute() rejected the symmetric fingerprint fallback: %v", err)
	}
}

func TestRuntimeEnforcesPreviewPolicyBeforeIdempotencyLookup(t *testing.T) {
	newRuntime := func(t *testing.T, descriptor Descriptor, hook audit.Hook, store *countingIdempotencyStore) *Engine {
		t.Helper()
		registry := newTestRegistry(t)
		if err := registry.Register("test", descriptor, &testHandler{}); err != nil {
			t.Fatal(err)
		}
		runtime, err := New(registry, Options{
			Authorizer: testAuthorizer{}, Audit: hook, Plans: newMemoryPlanStore(), Idempotency: store,
			Transactions: confirmedTransactionManager{},
		})
		if err != nil {
			t.Fatal(err)
		}
		return runtime
	}

	t.Run("required preview still requires the original hash on replay", func(t *testing.T) {
		store := &countingIdempotencyStore{testIdempotencyStore: newMemoryIdempotencyStore()}
		runtime := newRuntime(t, testDescriptor(), testsupport.DiscardAudit{}, store)
		request := testRequest()
		preview, err := runtime.Preview(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		request.PlanHash = preview.PlanHash
		request.IdempotencyKey = "required-policy"
		if _, err := runtime.Execute(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		store.lookups = 0
		request.PlanHash = ""
		if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodePlanRequired) {
			t.Fatalf("Execute() error = %v", err)
		}
		if store.lookups != 0 {
			t.Fatalf("invalid required-preview request performed %d lookup(s)", store.lookups)
		}
	})

	t.Run("preview-none rejects a supplied hash before replay lookup", func(t *testing.T) {
		descriptor := testDescriptor()
		descriptor.Preview = PreviewNone
		descriptor.PreviewSchema = nil
		store := &countingIdempotencyStore{testIdempotencyStore: newMemoryIdempotencyStore()}
		events := &collectingAudit{}
		runtime := newRuntime(t, descriptor, events, store)
		request := testRequest()
		request.IdempotencyKey = "none-policy"
		if _, err := runtime.Execute(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		storedPlanHash := events.events[len(events.events)-1].PlanHash
		if storedPlanHash == "" {
			t.Fatal("allowed audit omitted the internally resolved plan hash")
		}
		store.lookups = 0
		request.PlanHash = storedPlanHash
		if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodePreconditionFailed) {
			t.Fatalf("Execute() error = %v", err)
		}
		if store.lookups != 0 {
			t.Fatalf("invalid preview-none request performed %d lookup(s)", store.lookups)
		}
	})
}

func TestRuntimeEnforcesImpactRowConstraints(t *testing.T) {
	newRuntime := func(t *testing.T, policy PreviewPolicy) (*Engine, *testPlanStore) {
		t.Helper()
		descriptor := testDescriptor()
		descriptor.Preview = policy
		registry := newTestRegistry(t)
		if err := registry.Register("test", descriptor, &trackingPlanHandler{rows: 11}); err != nil {
			t.Fatal(err)
		}
		plans := newMemoryPlanStore()
		runtime, err := New(registry, Options{
			Authorizer: constrainedAuthorizer{maxRows: 10}, Audit: testsupport.DiscardAudit{}, Plans: plans,
			Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
		})
		if err != nil {
			t.Fatal(err)
		}
		return runtime, plans
	}

	t.Run("preview", func(t *testing.T) {
		runtime, plans := newRuntime(t, PreviewRequired)
		if _, err := runtime.Preview(context.Background(), testRequest()); !IsCode(err, CodeLimitExceeded) {
			t.Fatalf("Preview() error = %v", err)
		}
		plans.mu.RLock()
		defer plans.mu.RUnlock()
		if len(plans.plans) != 0 {
			t.Fatalf("over-limit preview saved %d plan(s)", len(plans.plans))
		}
	})

	t.Run("inline execution", func(t *testing.T) {
		runtime, _ := newRuntime(t, PreviewOptional)
		request := testRequest()
		request.IdempotencyKey = "over-limit"
		if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeLimitExceeded) {
			t.Fatalf("Execute() error = %v", err)
		}
	})
}

func TestRuntimeReauthorizesInsideTheWriteTransaction(t *testing.T) {
	descriptor := testDescriptor()
	handler := &trackingPlanHandler{}
	registry := newTestRegistry(t)
	if err := registry.Register("test", descriptor, handler); err != nil {
		t.Fatal(err)
	}
	authorizer := &revocableAuthorizer{}
	transactions := &transactionGate{entered: make(chan struct{}), release: make(chan struct{})}
	idempotency := newMemoryIdempotencyStore()
	events := &collectingAudit{}
	runtime, err := New(registry, Options{
		Authorizer: authorizer, Audit: events, Plans: newMemoryPlanStore(),
		Idempotency: idempotency, Transactions: transactions,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	transactions.block.Store(true)
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "revoked-before-transaction"

	result := make(chan error, 1)
	go func() {
		_, err := runtime.Execute(context.Background(), request)
		result <- err
	}()
	select {
	case <-transactions.entered:
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("Execute did not reach the transaction boundary")
	}
	authorizer.revoked.Store(true)
	close(transactions.release)
	select {
	case err := <-result:
		if !IsCode(err, CodeAuthzDenied) {
			t.Fatalf("Execute error = %v, want %s", err, CodeAuthzDenied)
		}
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("Execute did not return after transaction release")
	}
	if handler.executeCalls != 0 {
		t.Fatalf("revoked execution reached Handler %d time(s)", handler.executeCalls)
	}
	idempotency.mu.Lock()
	reservations := len(idempotency.records)
	idempotency.mu.Unlock()
	if reservations != 0 {
		t.Fatalf("revoked execution persisted %d idempotency record(s)", reservations)
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	for _, event := range events.events {
		if event.Decision == "allowed" {
			t.Fatalf("revoked execution emitted allowed audit: %#v", event)
		}
	}
}

func TestRuntimeReauthorizesReplayInsideTheTransaction(t *testing.T) {
	descriptor := testDescriptor()
	descriptor.Preview = PreviewNone
	descriptor.PreviewSchema = nil
	handler := &trackingPlanHandler{}
	registry := newTestRegistry(t)
	if err := registry.Register("test", descriptor, handler); err != nil {
		t.Fatal(err)
	}
	authorizer := &revocableAuthorizer{}
	transactions := &controllableTransactionGate{entered: make(chan struct{}), release: make(chan struct{})}
	events := &collectingAudit{}
	runtime, err := New(registry, Options{
		Authorizer: authorizer, Audit: events, Plans: newMemoryPlanStore(),
		Idempotency: newMemoryIdempotencyStore(), Transactions: transactions,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest()
	request.IdempotencyKey = "replay-revoked-before-transaction"
	first, err := runtime.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	transactions.block.Store(true)

	result := make(chan struct {
		value Result
		err   error
	}, 1)
	go func() {
		value, err := runtime.Execute(context.Background(), request)
		result <- struct {
			value Result
			err   error
		}{value: value, err: err}
	}()
	select {
	case <-transactions.entered:
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("replay did not reach the transaction boundary")
	}
	authorizer.revoked.Store(true)
	close(transactions.release)
	select {
	case replay := <-result:
		if !IsCode(replay.err, CodeAuthzDenied) || len(replay.value.Data) != 0 || bytes.Equal(replay.value.Data, first.Data) {
			t.Fatalf("revoked replay = %s, %v", replay.value.Data, replay.err)
		}
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("replay did not return after transaction release")
	}
	if handler.executeCalls != 1 {
		t.Fatalf("replay executed Handler %d time(s), want one original execution", handler.executeCalls)
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	for _, event := range events.events {
		if event.Decision == "idempotent_replay" {
			t.Fatalf("revoked replay emitted replay audit: %#v", event)
		}
	}
}

func TestRuntimeRejectsTamperedPersistedPlan(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(*Plan)
	}{
		{
			name: "stored hash differs from requested hash",
			tamper: func(plan *Plan) {
				plan.Hash = "sha256:forged"
			},
		},
		{
			name: "payload changed without updating hash",
			tamper: func(plan *Plan) {
				plan.Payload = json.RawMessage(`{"value":999}`)
			},
		},
		{
			name: "impact changed without updating hash",
			tamper: func(plan *Plan) {
				plan.Impact.Rows = 999
			},
		},
		{
			name: "expiry extended without updating hash",
			tamper: func(plan *Plan) {
				plan.ExpiresAt = plan.ExpiresAt.Add(24 * time.Hour)
			},
		},
		{
			name: "creation time changed without updating hash",
			tamper: func(plan *Plan) {
				plan.CreatedAt = plan.CreatedAt.Add(-24 * time.Hour)
			},
		},
		{
			name: "payload changed with recomputed hash",
			tamper: func(plan *Plan) {
				plan.Payload = json.RawMessage(`{"value":999}`)
				plan.Hash, _ = hashPlan(*plan)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
			plans := newMemoryPlanStore()
			handler := &testHandler{}
			runtime := newTestRuntimeWithOptions(t, handler, &clock, testsupport.DiscardAudit{}, plans)
			request := testRequest()
			preview, err := runtime.Preview(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}

			plans.mu.Lock()
			plan := plans.plans[preview.PlanHash]
			test.tamper(&plan)
			plans.plans[preview.PlanHash] = plan
			plans.mu.Unlock()

			request.PlanHash = preview.PlanHash
			request.IdempotencyKey = "tampered-plan"
			if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodePlanStale) {
				t.Fatalf("tampered plan error = %v", err)
			}
			if handler.executions != 0 {
				t.Fatalf("tampered plan reached handler %d times", handler.executions)
			}
		})
	}
}

func TestRuntimeRejectsPlanFromDifferentActionContract(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Descriptor)
	}{
		{name: "version", mutate: func(descriptor *Descriptor) { descriptor.Version = "0.2.0" }},
		{name: "schema", mutate: func(descriptor *Descriptor) {
			descriptor.OutputSchema = Object(map[string]Field{
				"ok":   RequiredField(Boolean()),
				"note": OptionalField(String()),
			}).JSON()
		}},
		{name: "audit level", mutate: func(descriptor *Descriptor) { descriptor.AuditLevel = AuditMetadata }},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
			plans := newMemoryPlanStore()
			build := func(t *testing.T, descriptor Descriptor, handler Handler) *Engine {
				t.Helper()
				registry := newTestRegistry(t)
				if err := registry.Register("test", descriptor, handler); err != nil {
					t.Fatal(err)
				}
				runtime, err := New(registry, Options{
					Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: plans,
					Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
					Clock: func() time.Time { return clock },
				})
				if err != nil {
					t.Fatal(err)
				}
				return runtime
			}
			descriptor := testDescriptor()
			previewRuntime := build(t, descriptor, &testHandler{})
			request := testRequest()
			preview, err := previewRuntime.Preview(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&descriptor)
			executeHandler := &testHandler{}
			executeRuntime := build(t, descriptor, executeHandler)
			request.PlanHash = preview.PlanHash
			request.IdempotencyKey = "cross-contract"
			if _, err := executeRuntime.Execute(context.Background(), request); !IsCode(err, CodePlanStale) {
				t.Fatalf("Execute() error = %v", err)
			}
			if executeHandler.executions != 0 {
				t.Fatalf("stale plan reached changed handler %d time(s)", executeHandler.executions)
			}
		})
	}
}

func TestPreviewAndHandlerCannotMutateStoredPlan(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	handler := &testHandler{resources: []string{"counter/one"}}
	runtime, _ := newTestRuntime(t, handler, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	preview.Impact.Resources[0] = "mutated-preview"
	handler.resources[0] = "mutated-handler"
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "owned-plan"
	if _, err := runtime.Execute(context.Background(), request); err != nil {
		t.Fatalf("aliases changed the stored plan: %v", err)
	}
}

func TestRuntimeRejectsInvalidHandlerPlanPayloadBeforeSave(t *testing.T) {
	for name, payload := range map[string]json.RawMessage{
		"unterminated object": json.RawMessage(`{"value":`),
		"unterminated array":  json.RawMessage(`[1,`),
	} {
		t.Run(name, func(t *testing.T) {
			clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
			plans := newMemoryPlanStore()
			runtime := newTestRuntimeWithOptions(t, &testHandler{payload: payload}, &clock, testsupport.DiscardAudit{}, plans)
			if _, err := runtime.Preview(context.Background(), testRequest()); !IsCode(err, CodeInternal) {
				t.Fatalf("Preview() error = %v", err)
			}
			plans.mu.RLock()
			stored := len(plans.plans)
			plans.mu.RUnlock()
			if stored != 0 {
				t.Fatalf("invalid handler payload saved %d plan(s)", stored)
			}
		})
	}
}

func TestCanonicalInputPreservesLargeIntegerIdentity(t *testing.T) {
	_, first, err := canonicalInput(json.RawMessage(`{"value":9007199254740992}`))
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := canonicalInput(json.RawMessage(`{"value":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("distinct large integers share canonical hash %s", first)
	}
	var equivalentHash string
	for _, input := range []json.RawMessage{json.RawMessage(`{"value":1}`), json.RawMessage(`{"value":1.0}`), json.RawMessage(`{"value":1e0}`), json.RawMessage(`{"value":10e-1}`)} {
		_, hash, err := canonicalInput(input)
		if err != nil {
			t.Fatal(err)
		}
		if equivalentHash == "" {
			equivalentHash = hash
		} else if hash != equivalentHash {
			t.Fatalf("equivalent numeric input %s hashed as %s, want %s", input, hash, equivalentHash)
		}
	}
	if _, _, err := canonicalInput(json.RawMessage(`{} {}`)); !IsCode(err, CodeValidationFailed) {
		t.Fatalf("trailing JSON error = %v", err)
	}
	if _, _, err := canonicalInput(json.RawMessage(`{"value":1,"value":2}`)); !IsCode(err, CodeValidationFailed) {
		t.Fatalf("duplicate property error = %v", err)
	}
}

func TestCanonicalJSONKeepsBoundedIntegersGoDecodable(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{input: `{"value":10}`, want: 10},
		{input: `{"value":-120}`, want: -120},
		{input: `{"value":1.2e1}`, want: 12},
		{input: `{"value":9223372036854775807}`, want: 9223372036854775807},
		{input: `{"value":-9223372036854775808}`, want: -9223372036854775808},
	}
	for _, test := range tests {
		canonical, _, err := canonicalInput(json.RawMessage(test.input))
		if err != nil {
			t.Fatalf("canonicalInput(%s): %v", test.input, err)
		}
		var decoded struct {
			Value int64 `json:"value"`
		}
		if err := json.Unmarshal(canonical, &decoded); err != nil {
			t.Fatalf("json.Unmarshal(%s) from %s: %v", canonical, test.input, err)
		}
		if decoded.Value != test.want {
			t.Fatalf("canonical value from %s = %d, want %d", test.input, decoded.Value, test.want)
		}
	}
}

func TestRuntimeReplaysCompletedResultAfterPlanExpires(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	handler := &testHandler{}
	runtime, events := newTestRuntime(t, handler, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "replay-after-expiry"
	first, err := runtime.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(24 * time.Hour)
	deleted, err := runtime.CleanupExpiredPlans(context.Background())
	if err != nil || deleted != 1 {
		t.Fatalf("cleanup deleted=%d err=%v", deleted, err)
	}
	replayed, err := runtime.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("completed replay after plan expiry: %v", err)
	}
	if !bytes.Equal(first.Data, replayed.Data) || handler.executions != 1 {
		t.Fatalf("replay=%s executions=%d", replayed.Data, handler.executions)
	}
	if events.events[len(events.events)-1].Decision != "idempotent_replay" {
		t.Fatalf("last audit = %#v", events.events[len(events.events)-1])
	}
	prepared, err := PrepareDescriptor(testDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	last := events.events[len(events.events)-1]
	if last.ActionVersion != testDescriptor().Version || last.ContractHash != prepared.ContractHash() {
		t.Fatalf("replay audit provenance = version %q contract %q", last.ActionVersion, last.ContractHash)
	}
	if last.PlanHash != preview.PlanHash {
		t.Fatalf("replay audit plan hash = %q, want original %q", last.PlanHash, preview.PlanHash)
	}
	if last.Impact == nil || last.Impact.Rows != 1 {
		t.Fatalf("replay audit omitted stored impact: %#v", last.Impact)
	}
}

func TestRuntimeRejectsIdempotencyReplayAcrossChannelOrPlan(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	handler := &testHandler{}
	runtime, _ := newTestRuntime(t, handler, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "execution-binding"
	if _, err := runtime.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	differentActorType := request
	differentActorType.Actor.Type = "service"
	if _, err := runtime.Execute(context.Background(), differentActorType); !IsCode(err, CodePlanStale) {
		t.Fatalf("plan replay across actor types = %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*Request)
	}{
		{name: "channel", mutate: func(candidate *Request) { candidate.Channel = "mcp" }},
		{name: "plan hash", mutate: func(candidate *Request) { candidate.PlanHash = "sha256:" + strings.Repeat("f", 64) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := request
			test.mutate(&candidate)
			if _, err := runtime.Execute(context.Background(), candidate); !IsCode(err, CodeIdempotencyConflict) {
				t.Fatalf("cross-binding replay error = %v", err)
			}
			if handler.executions != 1 {
				t.Fatalf("handler executed %d times", handler.executions)
			}
		})
	}
}

func TestRuntimeRejectsIdempotencyReplayFromDifferentActionContract(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	plans := newMemoryPlanStore()
	idempotency := newMemoryIdempotencyStore()
	newRuntime := func(t *testing.T, descriptor Descriptor, handler Handler) *Engine {
		t.Helper()
		registry := newTestRegistry(t)
		if err := registry.Register("test", descriptor, handler); err != nil {
			t.Fatal(err)
		}
		runtime, err := New(registry, Options{
			Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: plans,
			Idempotency: idempotency, Transactions: confirmedTransactionManager{}, Clock: func() time.Time { return clock },
		})
		if err != nil {
			t.Fatal(err)
		}
		return runtime
	}

	v1Handler := &testHandler{}
	v1 := newRuntime(t, testDescriptor(), v1Handler)
	request := testRequest()
	preview, err := v1.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "contract-upgrade"
	if _, err := v1.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*Descriptor)
	}{
		{name: "version", mutate: func(descriptor *Descriptor) { descriptor.Version = "0.2.0" }},
		{name: "audit level", mutate: func(descriptor *Descriptor) { descriptor.AuditLevel = AuditMetadata }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := testDescriptor()
			test.mutate(&changed)
			changedHandler := &testHandler{}
			changedRuntime := newRuntime(t, changed, changedHandler)
			if _, err := changedRuntime.Execute(context.Background(), request); !IsCode(err, CodeIdempotencyConflict) {
				t.Fatalf("cross-contract replay error = %v", err)
			}
			if changedHandler.executions != 0 {
				t.Fatalf("changed handler executed %d times", changedHandler.executions)
			}
		})
	}
}

func TestRuntimeReauthorizesStoredImpactBeforeIdempotencyReplay(t *testing.T) {
	for _, test := range []struct {
		name       string
		mutate     func(*mutableReplayAuthorizer)
		wantedCode string
	}{
		{
			name: "tighter row grant",
			mutate: func(authorizer *mutableReplayAuthorizer) {
				authorizer.maxRows = 1
			},
			wantedCode: CodeLimitExceeded,
		},
		{
			name: "changed grant fingerprint",
			mutate: func(authorizer *mutableReplayAuthorizer) {
				authorizer.fingerprint = "grant:v2"
			},
			wantedCode: CodePlanStale,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
			authorizer := &mutableReplayAuthorizer{maxRows: 10, fingerprint: "grant:v1"}
			handler := &trackingPlanHandler{rows: 2}
			registry := newTestRegistry(t)
			if err := registry.Register("test", testDescriptor(), handler); err != nil {
				t.Fatal(err)
			}
			runtime, err := New(registry, Options{
				Authorizer: authorizer, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
				Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{}, Clock: func() time.Time { return clock },
			})
			if err != nil {
				t.Fatal(err)
			}
			request := testRequest()
			preview, err := runtime.Preview(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			request.PlanHash = preview.PlanHash
			request.IdempotencyKey = "reauthorize-" + strings.ReplaceAll(test.name, " ", "-")
			if _, err := runtime.Execute(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			test.mutate(authorizer)
			if _, err := runtime.Execute(context.Background(), request); !IsCode(err, test.wantedCode) {
				t.Fatalf("replay error = %v, want %s", err, test.wantedCode)
			}
		})
	}
}

func TestRuntimeReplayStillRequiresCurrentIntentAuthorization(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, _ := newTestRuntime(t, &testHandler{}, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "authorized-replay"
	if _, err := runtime.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(24 * time.Hour)
	request.Actor.Type = "denied"
	if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeAuthzDenied) {
		t.Fatalf("unauthorized replay error = %v", err)
	}
}

func TestRuntimeReplayRejectsDifferentInputBeforePlanLookup(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, _ := newTestRuntime(t, &testHandler{}, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "bound-replay"
	if _, err := runtime.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(24 * time.Hour)
	request.Input = json.RawMessage(`{"value":2}`)
	if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeIdempotencyConflict) {
		t.Fatalf("different-input replay error = %v", err)
	}
}

func TestRuntimeFailureAuditSurvivesCanceledRequest(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	events := &contextAwareAudit{}
	runtime := newTestRuntimeWithOptions(t, &testHandler{executeErr: errors.New("database unavailable")}, &clock, events, nil)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "canceled-failure"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.Execute(ctx, request); !IsCode(err, CodeInternal) {
		t.Fatalf("error = %v", err)
	}
	if len(events.events) != 2 || events.events[1].Decision != "failed" {
		t.Fatalf("cancellation-safe audit events = %#v", events.events)
	}
}

func TestRuntimeReportsDetachedAuditFailureWithinBoundedTimeout(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	registry := newTestRegistry(t)
	if err := registry.Register("test", testDescriptor(), &testHandler{executeErr: errors.New("write failed")}); err != nil {
		t.Fatal(err)
	}
	failures := make(chan error, 1)
	runtime, err := New(registry, Options{
		Authorizer: testAuthorizer{}, Audit: selectiveBlockingAudit{},
		Plans: newMemoryPlanStore(), Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
		Clock: func() time.Time { return clock }, PlanTTL: 5 * time.Minute, AuditTimeout: 10 * time.Millisecond,
		AuditFailure: func(_ context.Context, err error, _ audit.Event) { failures <- err },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "audit-timeout"
	started := time.Now()
	if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) {
		t.Fatalf("primary error = %v", err)
	}
	if time.Since(started) > 500*time.Millisecond {
		t.Fatalf("detached audit exceeded timeout: %v", time.Since(started))
	}
	select {
	case err := <-failures:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("audit failure = %v", err)
		}
	default:
		t.Fatal("audit persistence failure was not reported")
	}
}

func TestRuntimeDoesNotPersistPreviewSummarySamples(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	plans := newMemoryPlanStore()
	runtime := newTestRuntimeWithOptions(t, &testHandler{}, &clock, &collectingAudit{}, plans)
	preview, err := runtime.Preview(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := plans.Get(context.Background(), preview.PlanHash)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(plan)
	if bytes.Contains(encoded, []byte(`"summary"`)) || bytes.Contains(encoded, []byte(`"matched_rows"`)) || bytes.Contains(encoded, []byte(`"input":`)) {
		t.Fatalf("persisted plan contains redundant request or display data: %s", encoded)
	}
}

func TestRuntimeRejectsUnboundedResultSummary(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, events := newTestRuntime(t, &testHandler{resultSummary: strings.Repeat("界", 2000)}, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "bounded-summary"
	if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) {
		t.Fatalf("Execute() error = %v", err)
	}
	last := events.events[len(events.events)-1]
	if last.Decision != "failed" || last.ResultSummary != "" {
		t.Fatalf("invalid result metadata audit = %#v", last)
	}
}

func TestRuntimeRejectsDeniedActorAndInvalidInput(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, _ := newTestRuntime(t, &testHandler{}, &clock)
	request := testRequest()
	request.Actor.Type = "denied"
	if _, err := runtime.Preview(context.Background(), request); !IsCode(err, CodeAuthzDenied) {
		t.Fatalf("denied error = %v", err)
	}
	request = testRequest()
	request.Input = json.RawMessage(`{"value":"not-an-integer"}`)
	if _, err := runtime.Preview(context.Background(), request); !IsCode(err, CodeValidationFailed) {
		t.Fatalf("validation error = %v", err)
	}
}

func TestRuntimeRejectsInvalidScopeAndDelegatesValidScopeToAuthorization(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, _ := newTestRuntime(t, &testHandler{}, &clock)

	missing := testRequest()
	missing.Scope = scope.Execution{}
	if _, err := runtime.Preview(context.Background(), missing); ErrorCode(err) != CodeAuthzDenied {
		t.Fatalf("missing scope error = %v", err)
	}

	mismatched := testRequest()
	mismatched.Scope = scope.Must("tenant", "other")
	if _, err := runtime.Preview(context.Background(), mismatched); err != nil {
		t.Fatalf("valid independently resolved scope error = %v", err)
	}
}

func TestRuntimeRejectsChangedInputForPlan(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, _ := newTestRuntime(t, &testHandler{}, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "changed"
	request.Input = json.RawMessage(`{"value":2}`)
	if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodePlanStale) {
		t.Fatalf("error = %v", err)
	}
}

func TestRuntimeAuditsHandlerFailureAsFailed(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, events := newTestRuntime(t, &testHandler{executeErr: errors.New("database unavailable")}, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "failure"
	if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) {
		t.Fatalf("error = %v", err)
	}
	last := events.events[len(events.events)-1]
	if last.Decision != "failed" || last.ErrorCode != CodeInternal || last.RequestID != request.RequestID {
		t.Fatalf("failure audit = %#v", last)
	}
}

func TestRuntimeContainsHandlerPanicsAndReleasesReservation(t *testing.T) {
	t.Run("plan", func(t *testing.T) {
		clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
		handler := &testHandler{panicPlan: true}
		runtime, events := newTestRuntime(t, handler, &clock)
		if _, err := runtime.Preview(context.Background(), testRequest()); !IsCode(err, CodeInternal) || !errors.Is(err, ErrCallbackPanic) {
			t.Fatalf("Preview() error = %v", err)
		}
		events.mu.Lock()
		defer events.mu.Unlock()
		if len(events.events) == 0 || events.events[len(events.events)-1].Decision != "failed" {
			t.Fatalf("panic audit = %#v", events.events)
		}
	})

	t.Run("execute", func(t *testing.T) {
		clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
		handler := &testHandler{}
		runtime, _ := newTestRuntime(t, handler, &clock)
		request := testRequest()
		preview, err := runtime.Preview(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		request.PlanHash = preview.PlanHash
		request.IdempotencyKey = "panic-retry"
		handler.panicExecute = true
		if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) || !errors.Is(err, ErrCallbackPanic) {
			t.Fatalf("Execute() panic error = %v", err)
		}
		handler.panicExecute = false
		if _, err := runtime.Execute(context.Background(), request); err != nil {
			t.Fatalf("retry after contained panic = %v", err)
		}
		if handler.executions != 1 {
			t.Fatalf("successful executions = %d", handler.executions)
		}
	})
}

func TestRuntimeFailedExecutionReleasesIdempotencyReservation(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	handler := &testHandler{executeErr: errors.New("temporary failure")}
	runtime, _ := newTestRuntime(t, handler, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "retry-after-failure"
	if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) {
		t.Fatalf("first error = %v", err)
	}
	handler.executeErr = nil
	if _, err := runtime.Execute(context.Background(), request); err != nil {
		t.Fatalf("retry after failed transaction = %v", err)
	}
}

func TestRuntimeRejectsBusinessCodeWhenAbortAlsoFails(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	primary := NewError(CodePreconditionFailed, "handler precondition failed")
	abortErr := errors.New("idempotency abort failed")
	handler := &testHandler{executeErr: primary}
	registry := newTestRegistry(t)
	if err := registry.Register("test", testDescriptor(), handler); err != nil {
		t.Fatal(err)
	}
	events := &collectingAudit{}
	runtime, err := New(registry, Options{
		Authorizer: testAuthorizer{}, Audit: events, Plans: newMemoryPlanStore(),
		Idempotency:  &abortFailingIdempotencyStore{testIdempotencyStore: newMemoryIdempotencyStore(), err: abortErr},
		Transactions: confirmedTransactionManager{}, Clock: func() time.Time { return clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "joined-abort"
	_, err = runtime.Execute(context.Background(), request)
	if !IsCode(err, CodeInternal) || IsCode(err, CodePreconditionFailed) || !errors.Is(err, primary) || !errors.Is(err, abortErr) {
		t.Fatalf("Execute() error graph = %v", err)
	}
	if strings.Contains(err.Error(), abortErr.Error()) {
		t.Fatalf("Execute() error text leaked dependency detail: %v", err)
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	last := events.events[len(events.events)-1]
	if strings.Contains(last.Reason, abortErr.Error()) {
		t.Fatalf("failure audit leaked abort failure detail: %#v", last)
	}
}

func TestRuntimeAuditFailureDoesNotPermitDuplicateEffect(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	handler := &testHandler{}
	registry := newTestRegistry(t)
	if err := registry.Register("test", testDescriptor(), handler); err != nil {
		t.Fatal(err)
	}
	auditErr := errors.New("allowed audit failed")
	runtime, err := New(registry, Options{
		Authorizer: testAuthorizer{}, Audit: failAllowedAudit{err: auditErr}, Plans: newMemoryPlanStore(),
		Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{}, Clock: func() time.Time { return clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "audit-ambiguity"
	if _, err := runtime.Execute(context.Background(), request); !errors.Is(err, auditErr) {
		t.Fatalf("first Execute() error = %v", err)
	}
	if _, err := runtime.Execute(context.Background(), request); err != nil {
		t.Fatalf("retry should replay committed result: %v", err)
	}
	if handler.executions != 1 {
		t.Fatalf("handler executed %d times after ambiguous response", handler.executions)
	}
}

func TestRuntimeRejectsHandlerOutputThatViolatesDescriptor(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, _ := newTestRuntime(t, &testHandler{output: json.RawMessage(`{"ok":"yes"}`)}, &clock)
	request := testRequest()
	preview, err := runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PlanHash = preview.PlanHash
	request.IdempotencyKey = "invalid-output"
	if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeInternal) {
		t.Fatalf("invalid handler output error = %v", err)
	}
}

func TestRuntimeRejectsHandlerPreviewThatViolatesDescriptor(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, _ := newTestRuntime(t, &testHandler{summary: json.RawMessage(`{"matched_rows":"one"}`)}, &clock)
	_, err := runtime.Preview(context.Background(), testRequest())
	if !IsCode(err, CodeInternal) {
		t.Fatalf("invalid preview summary error = %v", err)
	}
}

func TestRegistryRejectsIncompleteGovernanceDescriptor(t *testing.T) {
	registry := newTestRegistry(t)
	descriptor := testDescriptor()
	descriptor.AuditLevel = ""
	if err := registry.Register("test", descriptor, &testHandler{}); err == nil {
		t.Fatal("descriptor without audit level was accepted")
	}
	descriptor = testDescriptor()
	descriptor.Channels = nil
	if err := registry.Register("test", descriptor, &testHandler{}); err == nil {
		t.Fatal("descriptor without channels was accepted")
	}
	descriptor = testDescriptor()
	descriptor.OutputSchema = json.RawMessage(`{"type":7}`)
	if err := registry.Register("test", descriptor, &testHandler{}); err == nil {
		t.Fatal("descriptor with invalid JSON Schema was accepted")
	}
	descriptor = testDescriptor()
	descriptor.PreviewSchema = nil
	if err := registry.Register("test", descriptor, &testHandler{}); err == nil {
		t.Fatal("preview-capable descriptor without preview schema was accepted")
	}
	for _, test := range []struct {
		name   string
		mutate func(*Descriptor)
	}{
		{name: "empty title", mutate: func(descriptor *Descriptor) { descriptor.Title = "" }},
		{name: "invalid version", mutate: func(descriptor *Descriptor) { descriptor.Version = "1.0" }},
		{name: "invalid action id", mutate: func(descriptor *Descriptor) { descriptor.ID = "Bad Action" }},
		{name: "path action id", mutate: func(descriptor *Descriptor) { descriptor.ID = "example/read" }},
		{name: "dot segment traversal", mutate: func(descriptor *Descriptor) { descriptor.ID = "example..read" }},
		{name: "trailing dot", mutate: func(descriptor *Descriptor) { descriptor.ID = "example.read." }},
		{name: "empty permission", mutate: func(descriptor *Descriptor) { descriptor.Permission = "" }},
		{name: "path permission", mutate: func(descriptor *Descriptor) { descriptor.Permission = "example/read" }},
		{name: "empty channel", mutate: func(descriptor *Descriptor) { descriptor.Channels = []Channel{""} }},
		{name: "surrounding channel whitespace", mutate: func(descriptor *Descriptor) { descriptor.Channels = []Channel{" http"} }},
		{name: "control channel", mutate: func(descriptor *Descriptor) { descriptor.Channels = []Channel{"http\n"} }},
		{name: "invalid UTF-8 channel", mutate: func(descriptor *Descriptor) { descriptor.Channels = []Channel{Channel(string([]byte{0xff}))} }},
		{name: "duplicate channel", mutate: func(descriptor *Descriptor) { descriptor.Channels = []Channel{ChannelHTTP, ChannelHTTP} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := newTestRegistry(t)
			descriptor := testDescriptor()
			test.mutate(&descriptor)
			if err := registry.Register("test", descriptor, &testHandler{}); err == nil {
				t.Fatalf("invalid descriptor was accepted: %#v", descriptor)
			}
		})
	}
}

func TestRuntimeRequiresExplicitGovernanceDependencies(t *testing.T) {
	registry := newTestRegistry(t)
	base := func() Options {
		return Options{
			Authorizer:   testAuthorizer{},
			Audit:        testsupport.DiscardAudit{},
			Plans:        newMemoryPlanStore(),
			Idempotency:  newMemoryIdempotencyStore(),
			Transactions: confirmedTransactionManager{},
		}
	}
	tests := []struct {
		name   string
		unset  func(*Options)
		wanted string
	}{
		{name: "authorizer", unset: func(options *Options) { options.Authorizer = nil }, wanted: "authorizer is required"},
		{name: "audit hook", unset: func(options *Options) { options.Audit = nil }, wanted: "audit hook is required"},
		{name: "plan store", unset: func(options *Options) { options.Plans = nil }, wanted: "plan store is required"},
		{name: "idempotency store", unset: func(options *Options) { options.Idempotency = nil }, wanted: "idempotency store is required"},
		{name: "transaction manager", unset: func(options *Options) { options.Transactions = nil }, wanted: "transaction manager is required"},
		{name: "typed nil authorizer", unset: func(options *Options) { var value *testAuthorizer; options.Authorizer = value }, wanted: "authorizer is required"},
		{name: "typed nil audit hook", unset: func(options *Options) { var value *collectingAudit; options.Audit = value }, wanted: "audit hook is required"},
		{name: "typed nil plan store", unset: func(options *Options) { var value *testPlanStore; options.Plans = value }, wanted: "plan store is required"},
		{name: "typed nil idempotency store", unset: func(options *Options) { var value *testIdempotencyStore; options.Idempotency = value }, wanted: "idempotency store is required"},
		{name: "typed nil transaction manager", unset: func(options *Options) { var value transactionManagerFunc; options.Transactions = value }, wanted: "transaction manager is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := base()
			test.unset(&options)
			if _, err := New(registry, options); err == nil || !strings.Contains(err.Error(), test.wanted) {
				t.Fatalf("NewRuntime() error = %v, want %q", err, test.wanted)
			}
		})
	}
}

func TestRuntimeRejectsNegativeDurationOptions(t *testing.T) {
	registry := newTestRegistry(t)
	base := Options{
		Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
		Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
	}
	for _, test := range []struct {
		name   string
		mutate func(*Options)
	}{
		{name: "plan TTL", mutate: func(options *Options) { options.PlanTTL = -time.Second }},
		{name: "audit timeout", mutate: func(options *Options) { options.AuditTimeout = -time.Second }},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := base
			test.mutate(&options)
			if runtime, err := New(registry, options); err == nil || runtime != nil {
				t.Fatalf("NewRuntime() = %#v, %v", runtime, err)
			}
		})
	}
}

func TestRuntimeValidatesEnvelopeBeforePersistence(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Request)
	}{
		{name: "invalid action id", mutate: func(request *Request) { request.ActionID = "Bad Action" }},
		{name: "empty channel", mutate: func(request *Request) { request.Channel = "" }},
		{name: "control actor id", mutate: func(request *Request) { request.Actor.ID = "user\nadmin" }},
		{name: "oversized request id", mutate: func(request *Request) { request.RequestID = strings.Repeat("r", 129) }},
		{name: "invalid plan hash", mutate: func(request *Request) { request.PlanHash = "sha256:not-a-digest" }},
		{name: "control idempotency key", mutate: func(request *Request) { request.IdempotencyKey = "once\nagain" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
			plans := &countingPlanStore{testPlanStore: newMemoryPlanStore()}
			idempotency := &countingIdempotencyStore{testIdempotencyStore: newMemoryIdempotencyStore()}
			registry := newTestRegistry(t)
			if err := registry.Register("test", testDescriptor(), &testHandler{}); err != nil {
				t.Fatal(err)
			}
			runtime, err := New(registry, Options{
				Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: plans, Idempotency: idempotency,
				Transactions: confirmedTransactionManager{}, Clock: func() time.Time { return clock },
			})
			if err != nil {
				t.Fatal(err)
			}
			request := testRequest()
			test.mutate(&request)
			if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeValidationFailed) {
				t.Fatalf("Execute() error = %v", err)
			}
			if plans.gets != 0 || idempotency.lookups != 0 {
				t.Fatalf("invalid envelope touched stores: plan gets=%d idempotency lookups=%d", plans.gets, idempotency.lookups)
			}
		})
	}
}

func TestCleanupExpiredPlansParticipatesInRuntimeDrain(t *testing.T) {
	registry := newTestRegistry(t)
	store := &blockingCleanupPlanStore{
		testPlanStore: newMemoryPlanStore(),
		entered:       make(chan struct{}),
		release:       make(chan struct{}),
	}
	runtime, err := New(registry, Options{
		Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: store,
		Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanupResult := make(chan error, 1)
	go func() {
		_, cleanupErr := runtime.CleanupExpiredPlans(context.Background())
		cleanupResult <- cleanupErr
	}()
	select {
	case <-store.entered:
	case <-time.After(actionRuntimeSynchronizationTimeout):
		t.Fatal("plan cleanup did not enter store")
	}
	resetResult := make(chan error, 1)
	go func() { resetResult <- registry.Reset(context.Background()) }()
	select {
	case err := <-resetResult:
		t.Fatalf("Reset completed before plan cleanup drained: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(store.release)
	if err := <-cleanupResult; err != nil {
		t.Fatalf("CleanupExpiredPlans() error = %v", err)
	}
	if err := <-resetResult; err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeLeaseCoversClockAndRevokedCallsDoNotTouchIt(t *testing.T) {
	for _, operation := range []string{"preview", "execute"} {
		t.Run(operation, func(t *testing.T) {
			registry := newTestRegistry(t)
			if err := registry.Register("test", testDescriptor(), &testHandler{}); err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			releaseClock := make(chan struct{})
			var first sync.Once
			var clockCalls atomic.Int32
			runtime, err := New(registry, Options{
				Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
				Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
				Clock: func() time.Time {
					clockCalls.Add(1)
					first.Do(func() {
						close(entered)
						<-releaseClock
					})
					return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			call := func() error {
				if operation == "preview" {
					_, err := runtime.Preview(context.Background(), testRequest())
					return err
				}
				_, err := runtime.Execute(context.Background(), testRequest())
				return err
			}
			callResult := make(chan error, 1)
			go func() { callResult <- call() }()
			select {
			case <-entered:
			case <-time.After(actionRuntimeSynchronizationTimeout):
				t.Fatal("Runtime did not enter clock")
			}
			resetResult := make(chan error, 1)
			go func() { resetResult <- registry.Reset(context.Background()) }()
			select {
			case err := <-resetResult:
				t.Fatalf("Reset crossed an active clock lease: %v", err)
			case <-time.After(20 * time.Millisecond):
			}
			close(releaseClock)
			select {
			case <-callResult:
			case <-time.After(actionRuntimeSynchronizationTimeout):
				t.Fatal("Runtime call did not drain")
			}
			if err := <-resetResult; err != nil {
				t.Fatal(err)
			}
			before := clockCalls.Load()
			if err := call(); !IsCode(err, CodeUnavailable) {
				t.Fatalf("revoked Runtime error = %v", err)
			}
			if clockCalls.Load() != before {
				t.Fatal("revoked Runtime call touched Clock")
			}
		})
	}
}

func TestUnknownActionReleasesRuntimeLease(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	registry := newTestRegistry(t)
	runtime, err := New(registry, Options{
		Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
		Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{}, Clock: func() time.Time { return clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest()
	request.ActionID = "unknown.execute"
	if _, err := runtime.Execute(context.Background(), request); !IsCode(err, CodeActionNotFound) {
		t.Fatalf("Execute() error = %v", err)
	}
	resetCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := registry.Reset(resetCtx); err != nil {
		t.Fatalf("unknown action leaked a runtime lease: %v", err)
	}
}

func TestInternalRegistryRejectsNilAndRevokedConstruction(t *testing.T) {
	var missing *Registry
	if err := missing.Register("test", testDescriptor(), &testHandler{}); err == nil {
		t.Fatal("nil Registry accepted a binding")
	}
	registry := newTestRegistry(t)
	if err := registry.Register("test", testDescriptor(), &testHandler{}); err != nil {
		t.Fatal(err)
	}
	options := Options{
		Authorizer: testAuthorizer{}, Audit: testsupport.DiscardAudit{}, Plans: newMemoryPlanStore(),
		Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
	}
	runtime, err := New(registry, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Resolve(testDescriptor().ID); ok || len(registry.List()) != 0 {
		t.Fatal("authorized Reset retained Action bindings")
	}
	if _, err := runtime.Preview(context.Background(), testRequest()); !IsCode(err, CodeUnavailable) {
		t.Fatalf("existing Runtime after Reset error = %v", err)
	}
	if runtime, err := New(registry, options); err == nil || runtime != nil {
		t.Fatalf("NewRuntime(invalid) = %#v, %v", runtime, err)
	}
}

type testHandler struct {
	executions    int
	panicPlan     bool
	panicExecute  bool
	executeErr    error
	output        json.RawMessage
	summary       json.RawMessage
	resultSummary string
	resources     []string
	payload       json.RawMessage
}

type abortFailingIdempotencyStore struct {
	*testIdempotencyStore
	err error
}

type failingPlanStore struct{ err error }

func (store failingPlanStore) Save(context.Context, Plan) error { return store.err }
func (store failingPlanStore) Get(context.Context, string) (Plan, error) {
	return Plan{}, store.err
}
func (store failingPlanStore) DeleteExpired(context.Context, time.Time) (int64, error) {
	return 0, store.err
}

type countingPlanStore struct {
	*testPlanStore
	gets int
}

func (store *countingPlanStore) Get(ctx context.Context, hash string) (Plan, error) {
	store.gets++
	return store.testPlanStore.Get(ctx, hash)
}

type countingIdempotencyStore struct {
	*testIdempotencyStore
	lookups int
}

func (store *countingIdempotencyStore) Lookup(ctx context.Context, record IdempotencyRecord) (*IdempotencyRecord, error) {
	store.lookups++
	return store.testIdempotencyStore.Lookup(ctx, record)
}

type barrierLookupIdempotencyStore struct {
	*testIdempotencyStore
	lookups atomic.Int32
	ready   chan struct{}
	once    sync.Once
}

func (store *barrierLookupIdempotencyStore) Lookup(ctx context.Context, _ IdempotencyRecord) (*IdempotencyRecord, error) {
	if store.lookups.Add(1) == 2 {
		store.once.Do(func() { close(store.ready) })
	}
	select {
	case <-store.ready:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type blockingCleanupPlanStore struct {
	*testPlanStore
	entered chan struct{}
	release chan struct{}
}

func (store *blockingCleanupPlanStore) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	close(store.entered)
	<-store.release
	return store.testPlanStore.DeleteExpired(ctx, before)
}

func (store *abortFailingIdempotencyStore) Abort(context.Context, IdempotencyRecord) error {
	return store.err
}

type trackingPlanHandler struct {
	payload      json.RawMessage
	rows         int
	planCalls    int
	executeCalls int
	executedPlan Plan
}

type blockingInlineHandler struct {
	planCalls    atomic.Int32
	executeCalls atomic.Int32
	entered      chan struct{}
	release      chan struct{}
}

func (handler *blockingInlineHandler) Plan(context.Context, Request) (PlanData, error) {
	call := handler.planCalls.Add(1)
	return PlanData{
		Payload: json.RawMessage(fmt.Sprintf(`{"value":%d}`, call)),
		Impact:  authz.Impact{Rows: 1},
	}, nil
}

func (handler *blockingInlineHandler) Execute(context.Context, Plan) (Result, error) {
	handler.executeCalls.Add(1)
	select {
	case handler.entered <- struct{}{}:
	default:
	}
	<-handler.release
	return Result{Data: json.RawMessage(`{"ok":true}`)}, nil
}

func (handler *trackingPlanHandler) Plan(context.Context, Request) (PlanData, error) {
	handler.planCalls++
	payload := handler.payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{"value":1}`)
	}
	rows := handler.rows
	if rows == 0 {
		rows = 1
	}
	return PlanData{
		Payload: payload,
		Summary: json.RawMessage(`{"matched_rows":1}`),
		Impact:  authz.Impact{Rows: rows},
	}, nil
}

func (handler *trackingPlanHandler) Execute(_ context.Context, plan Plan) (Result, error) {
	handler.executeCalls++
	handler.executedPlan = clonePlan(plan)
	return Result{Data: json.RawMessage(`{"ok":true}`)}, nil
}

func (h *testHandler) Plan(_ context.Context, request Request) (PlanData, error) {
	if h.panicPlan {
		panic("plan panic")
	}
	summary := h.summary
	if len(summary) == 0 {
		summary = json.RawMessage(`{"matched_rows":1}`)
	}
	payload := h.payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{"value":1}`)
	}
	return PlanData{
		Payload: payload,
		Summary: summary,
		Impact:  authz.Impact{Rows: 1, Resources: h.resources},
	}, nil
}

func (h *testHandler) Execute(context.Context, Plan) (Result, error) {
	if h.panicExecute {
		panic("execute panic")
	}
	h.executions++
	if h.executeErr != nil {
		return Result{}, h.executeErr
	}
	if len(h.output) > 0 {
		return Result{Data: h.output, Summary: "returned configured output"}, nil
	}
	summary := h.resultSummary
	if summary == "" {
		summary = "executed one row"
	}
	return Result{Data: json.RawMessage(`{"ok":true}`), Summary: summary}, nil
}

type testAuthorizer struct{}

type intentFingerprintAuthorizer struct{}

type constrainedAuthorizer struct{ maxRows int }

type mutableReplayAuthorizer struct {
	maxRows     int
	fingerprint string
}

type revocableAuthorizer struct{ revoked atomic.Bool }

func (authorizer *revocableAuthorizer) Authorize(_ context.Context, request authz.Request) (authz.Decision, error) {
	if authorizer.revoked.Load() {
		return authz.Decision{Reason: "grant was revoked", Fingerprint: "policy:v2"}, nil
	}
	return authz.Decision{Allowed: true, Fingerprint: "policy:v1"}, nil
}

type transactionGate struct {
	block   atomic.Bool
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (manager *transactionGate) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if !manager.block.Load() {
		return confirmTestRollback(operation(ctx))
	}
	manager.once.Do(func() { close(manager.entered) })
	select {
	case <-manager.release:
		return confirmTestRollback(operation(ctx))
	case <-ctx.Done():
		return ctx.Err()
	}
}

type controllableTransactionGate struct {
	block   atomic.Bool
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (manager *controllableTransactionGate) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if !manager.block.Load() {
		return confirmTestRollback(operation(ctx))
	}
	manager.once.Do(func() { close(manager.entered) })
	select {
	case <-manager.release:
		return confirmTestRollback(operation(ctx))
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (authorizer *mutableReplayAuthorizer) Authorize(_ context.Context, request authz.Request) (authz.Decision, error) {
	decision := authz.Decision{Allowed: true, Fingerprint: authorizer.fingerprint}
	if request.Phase == authz.PhaseImpact {
		decision.Constraints.MaxRows = authorizer.maxRows
	}
	return decision, nil
}

func (authorizer constrainedAuthorizer) Authorize(_ context.Context, request authz.Request) (authz.Decision, error) {
	decision := authz.Decision{Allowed: true, Fingerprint: "constrained-policy-v1"}
	if request.Phase == authz.PhaseImpact {
		decision.Constraints.MaxRows = authorizer.maxRows
	}
	return decision, nil
}

func (intentFingerprintAuthorizer) Authorize(_ context.Context, request authz.Request) (authz.Decision, error) {
	decision := authz.Decision{Allowed: true}
	if request.Phase == authz.PhaseIntent {
		decision.Fingerprint = "intent-policy-v1"
	}
	return decision, nil
}

func (testAuthorizer) Authorize(_ context.Context, request authz.Request) (authz.Decision, error) {
	if request.Actor.Type != "denied" {
		return authz.Decision{Allowed: true, Fingerprint: "policy-v1"}, nil
	}
	return authz.Decision{Allowed: false, Reason: "permission is missing", RequiredPermission: request.Permission, Fingerprint: "policy-v1"}, nil
}

type collectingAudit struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *collectingAudit) Record(_ context.Context, event audit.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, event)
	return nil
}

type contextAwareAudit struct {
	events []audit.Event
}

type selectiveBlockingAudit struct{}

type failAllowedAudit struct{ err error }

func (hook failAllowedAudit) Record(_ context.Context, event audit.Event) error {
	if event.Decision == "allowed" {
		return hook.err
	}
	return nil
}

func (selectiveBlockingAudit) Record(ctx context.Context, event audit.Event) error {
	if event.Decision != "failed" && event.Decision != "denied" {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

func (a *contextAwareAudit) Record(ctx context.Context, event audit.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.events = append(a.events, event)
	return nil
}

func newTestRuntime(t *testing.T, handler Handler, clock *time.Time) (*Engine, *collectingAudit) {
	t.Helper()
	events := &collectingAudit{}
	return newTestRuntimeWithOptions(t, handler, clock, events, nil), events
}

func newTestRuntimeWithOptions(t *testing.T, handler Handler, clock *time.Time, hook audit.Hook, plans PlanStore) *Engine {
	t.Helper()
	registry := newTestRegistry(t)
	err := registry.Register("test", testDescriptor(), handler)
	if err != nil {
		t.Fatal(err)
	}
	if hook == nil {
		hook = testsupport.DiscardAudit{}
	}
	if plans == nil {
		plans = newMemoryPlanStore()
	}
	runtime, err := New(registry, Options{
		Authorizer:   testAuthorizer{},
		Audit:        hook,
		Plans:        plans,
		Idempotency:  newMemoryIdempotencyStore(),
		Transactions: confirmedTransactionManager{},
		Clock:        func() time.Time { return *clock },
		PlanTTL:      5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	return NewRegistry()
}

func testDescriptor() Descriptor {
	return Descriptor{
		ID:                  "test.execute",
		Version:             "0.1.0",
		Title:               "Test execute",
		InputSchema:         Object(map[string]Field{"value": RequiredField(Integer())}).JSON(),
		PreviewSchema:       Object(map[string]Field{"matched_rows": RequiredField(Integer())}).JSON(),
		OutputSchema:        Object(map[string]Field{"ok": RequiredField(Boolean())}).JSON(),
		Permission:          "test.execute",
		Preview:             PreviewRequired,
		AuditLevel:          AuditDetailed,
		Channels:            []Channel{ChannelHTTP, ChannelCLI, ChannelMCP},
		RequiresIdempotency: true,
	}
}

func testRequest() Request {
	executionScope := scope.Must("tenant", "test")
	return Request{
		RequestID: "req_test",
		Actor:     identity.Actor{ID: "user_1", Type: "user"},
		Channel:   ChannelHTTP,
		ActionID:  "test.execute",
		Scope:     executionScope,
		Input:     json.RawMessage(`{"value":1}`),
	}
}
