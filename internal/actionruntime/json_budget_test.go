package actionruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/internal/actionpersistence"
)

func TestRuntimeRejectsActionJSONLimitsBeforeBusinessDependencies(t *testing.T) {
	documents := map[string]json.RawMessage{
		"bytes":  json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)) + `"`),
		"depth":  nestedJSON(action.MaxJSONNestingDepth + 1),
		"nodes":  arrayJSON(action.MaxJSONValueNodes),
		"number": numberJSON(action.MaxJSONNumberBytes + 1),
	}
	for name, document := range documents {
		for _, operation := range []string{"preview", "execute"} {
			t.Run(name+"/"+operation, func(t *testing.T) {
				fixture := newJSONBudgetRuntime(t, jsonBudgetHandler{})
				request := testRequest()
				request.Input = document
				var err error
				if operation == "preview" {
					_, err = fixture.runtime.Preview(context.Background(), request)
				} else {
					_, err = fixture.runtime.Execute(context.Background(), request)
				}
				if !action.IsCode(err, action.CodeLimitExceeded) {
					t.Fatalf("%s error = %v", operation, err)
				}
				if fixture.authorizer.calls != 0 || fixture.handler.planCalls != 0 || fixture.handler.executeCalls != 0 ||
					fixture.transactions.calls != 0 || fixture.plans.calls != 0 || fixture.idempotency.calls != 0 {
					t.Fatalf("limit failure touched business dependencies: %#v", fixture)
				}
				if len(fixture.audit.events) != 1 {
					t.Fatalf("limit failure audit events = %d, want 1", len(fixture.audit.events))
				}
				event := fixture.audit.events[0]
				if event.Decision != "rejected" || event.ErrorCode != action.CodeLimitExceeded || event.InputHash != "" {
					t.Fatalf("limit failure audit = %#v", event)
				}
			})
		}
	}
}

func TestRuntimeAcceptsPublishedActionJSONBoundaries(t *testing.T) {
	documents := map[string]json.RawMessage{
		"bytes":  json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-2) + `"`),
		"depth":  nestedJSON(action.MaxJSONNestingDepth),
		"nodes":  arrayJSON(action.MaxJSONValueNodes - 1),
		"number": numberJSON(action.MaxJSONNumberBytes),
	}
	for name, document := range documents {
		t.Run(name, func(t *testing.T) {
			fixture := newJSONBudgetRuntime(t, jsonBudgetHandler{})
			request := testRequest()
			request.Input = document
			if _, err := fixture.runtime.Preview(context.Background(), request); err != nil {
				t.Fatalf("Preview(exact %s boundary): %v", name, err)
			}
			if fixture.handler.planCalls != 1 || fixture.transactions.calls != 1 || fixture.plans.calls == 0 {
				t.Fatalf("accepted boundary did not complete Preview: %#v", fixture)
			}
		})
	}
}

func TestRuntimeRequiresExplicitActionInputDocument(t *testing.T) {
	for _, operation := range []string{"preview", "execute"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newJSONBudgetRuntime(t, jsonBudgetHandler{})
			request := testRequest()
			request.Input = nil
			var err error
			if operation == "preview" {
				_, err = fixture.runtime.Preview(context.Background(), request)
			} else {
				_, err = fixture.runtime.Execute(context.Background(), request)
			}
			if !action.IsCode(err, action.CodeValidationFailed) {
				t.Fatalf("%s missing input error = %v", operation, err)
			}
			if fixture.authorizer.calls != 0 || fixture.handler.planCalls != 0 || fixture.handler.executeCalls != 0 ||
				fixture.transactions.calls != 0 || fixture.plans.calls != 0 || fixture.idempotency.calls != 0 {
				t.Fatalf("missing input touched business dependencies: %#v", fixture)
			}
			if len(fixture.audit.events) != 1 {
				t.Fatalf("missing input audit events = %d, want 1", len(fixture.audit.events))
			}
			event := fixture.audit.events[0]
			if event.Decision != "rejected" || event.ErrorCode != action.CodeValidationFailed || event.InputHash != "" {
				t.Fatalf("missing input audit = %#v", event)
			}
		})
	}
}

func TestCanonicalJSONExpansionCannotExceedActionLimit(t *testing.T) {
	raw := json.RawMessage(`"` + strings.Repeat("<", int(action.MaxJSONDocumentBytes/5)) + `"`)
	if int64(len(raw)) >= action.MaxJSONDocumentBytes {
		t.Fatal("test input must fit before canonical escaping")
	}
	if _, _, err := canonicalInput(raw); !action.IsCode(err, action.CodeLimitExceeded) {
		t.Fatalf("canonicalInput(expanding string) = %v", err)
	}

	fixture := newJSONBudgetRuntime(t, jsonBudgetHandler{})
	request := testRequest()
	request.Input = raw
	if _, err := fixture.runtime.Preview(context.Background(), request); !action.IsCode(err, action.CodeLimitExceeded) {
		t.Fatalf("Preview(expanding string) = %v", err)
	}
	if fixture.handler.planCalls != 0 || fixture.authorizer.calls != 0 {
		t.Fatalf("canonical expansion reached business dependencies: %#v", fixture)
	}
}

func TestHandlerActionJSONBoundaryMatrix(t *testing.T) {
	for _, boundary := range actionJSONBoundaries() {
		for _, document := range []struct {
			name string
			run  func(*jsonBudgetFixture) error
		}{
			{
				name: "plan payload",
				run: func(fixture *jsonBudgetFixture) error {
					fixture.handler.plan = action.PlanData{Payload: boundary.value, Summary: json.RawMessage(`{}`)}
					_, err := fixture.runtime.Preview(context.Background(), testRequest())
					return err
				},
			},
			{
				name: "preview summary",
				run: func(fixture *jsonBudgetFixture) error {
					fixture.handler.plan = action.PlanData{Payload: json.RawMessage(`{}`), Summary: boundary.value}
					_, err := fixture.runtime.Preview(context.Background(), testRequest())
					return err
				},
			},
			{
				name: "result data",
				run: func(fixture *jsonBudgetFixture) error {
					fixture.handler.result = action.Result{Data: boundary.value}
					request := testRequest()
					request.IdempotencyKey = "bounded-result"
					_, err := fixture.runtime.Execute(context.Background(), request)
					return err
				},
			},
		} {
			t.Run(document.name+"/"+boundary.name, func(t *testing.T) {
				fixture := newJSONBudgetRuntime(t, jsonBudgetHandler{})
				err := document.run(fixture)
				if boundary.within {
					if err != nil {
						t.Fatalf("exact boundary rejected: %v", err)
					}
					if fixture.audit.allowed != 1 || fixture.transactions.calls != 1 {
						t.Fatalf("exact boundary did not reach one durable success path: %#v", fixture)
					}
					return
				}
				if !action.IsCode(err, action.CodeInternal) {
					t.Fatalf("above-boundary handler document error = %v", err)
				}
				if fixture.audit.allowed != 0 || fixture.plans.calls != 0 || fixture.idempotency.completes != 0 {
					t.Fatalf("invalid handler document reached durable success path: %#v", fixture)
				}
				if document.name != "result data" && fixture.transactions.calls != 0 {
					t.Fatalf("invalid planned document opened a transaction: %#v", fixture)
				}
				if document.name == "result data" && fixture.transactions.calls != 1 {
					t.Fatalf("result validation escaped its one transaction: %#v", fixture)
				}
			})
		}
	}
}

type actionJSONBoundary struct {
	name   string
	value  json.RawMessage
	within bool
}

func actionJSONBoundaries() []actionJSONBoundary {
	return []actionJSONBoundary{
		{name: "bytes exact", value: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-2) + `"`), within: true},
		{name: "bytes above", value: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-1) + `"`)},
		{name: "depth exact", value: nestedJSON(action.MaxJSONNestingDepth), within: true},
		{name: "depth above", value: nestedJSON(action.MaxJSONNestingDepth + 1)},
		{name: "nodes exact", value: arrayJSON(action.MaxJSONValueNodes - 1), within: true},
		{name: "nodes above", value: arrayJSON(action.MaxJSONValueNodes)},
		{name: "number exact", value: numberJSON(action.MaxJSONNumberBytes), within: true},
		{name: "number above", value: numberJSON(action.MaxJSONNumberBytes + 1)},
	}
}

func nestedJSON(depth int) json.RawMessage {
	return json.RawMessage(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
}

func arrayJSON(values int) json.RawMessage {
	return json.RawMessage("[" + strings.TrimSuffix(strings.Repeat("0,", values), ",") + "]")
}

func numberJSON(bytes int) json.RawMessage {
	return json.RawMessage("1" + strings.Repeat("0", bytes-1))
}

type jsonBudgetFixture struct {
	runtime      *Engine
	handler      *jsonBudgetHandler
	authorizer   *jsonBudgetAuthorizer
	transactions *jsonBudgetTransactions
	plans        *jsonBudgetPlanStore
	idempotency  *jsonBudgetIdempotencyStore
	audit        *jsonBudgetAudit
}

func newJSONBudgetRuntime(t *testing.T, supplied jsonBudgetHandler) *jsonBudgetFixture {
	t.Helper()
	handler := &supplied
	authorizer := &jsonBudgetAuthorizer{}
	transactions := &jsonBudgetTransactions{}
	plans := &jsonBudgetPlanStore{inner: newMemoryPlanStore()}
	idempotency := &jsonBudgetIdempotencyStore{inner: newMemoryIdempotencyStore()}
	auditHook := &jsonBudgetAudit{}
	registry := NewRegistry()
	descriptor := action.Descriptor{
		ID: "test.execute", Version: "0.1.0", Title: "JSON budget",
		InputSchema: json.RawMessage(`{}`), PreviewSchema: json.RawMessage(`{}`), OutputSchema: json.RawMessage(`{}`),
		Permission: "test.execute", Preview: action.PreviewOptional, AuditLevel: action.AuditDetailed,
		Channels: []action.Channel{action.ChannelHTTP, action.ChannelCLI, action.ChannelMCP},
	}
	if err := registry.Register("test", descriptor, handler); err != nil {
		t.Fatal(err)
	}
	runtime, err := New(registry, Options{
		Authorizer: authorizer, Audit: auditHook, Plans: plans, Idempotency: idempotency,
		Transactions: transactions, Clock: func() time.Time { return time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return &jsonBudgetFixture{runtime, handler, authorizer, transactions, plans, idempotency, auditHook}
}

type jsonBudgetHandler struct {
	plan         action.PlanData
	result       action.Result
	planCalls    int
	executeCalls int
}

func (handler *jsonBudgetHandler) Plan(context.Context, action.Request) (action.PlanData, error) {
	handler.planCalls++
	if handler.plan.Payload != nil || handler.plan.Summary != nil {
		return handler.plan, nil
	}
	return action.PlanData{Payload: json.RawMessage(`{}`), Summary: json.RawMessage(`{}`)}, nil
}

func (handler *jsonBudgetHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	handler.executeCalls++
	if handler.result.Data != nil {
		return handler.result, nil
	}
	return action.Result{Data: json.RawMessage(`{}`)}, nil
}

type jsonBudgetAuthorizer struct{ calls int }

func (authorizer *jsonBudgetAuthorizer) Authorize(ctx context.Context, request authz.Request) (authz.Decision, error) {
	authorizer.calls++
	return testAuthorizer{}.Authorize(ctx, request)
}

type jsonBudgetTransactions struct{ calls int }

func (transactions *jsonBudgetTransactions) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	transactions.calls++
	return confirmedTransactionManager{}.WithinTransaction(ctx, operation)
}

type jsonBudgetPlanStore struct {
	inner *testPlanStore
	calls int
}

func (store *jsonBudgetPlanStore) Save(ctx context.Context, plan action.Plan) error {
	store.calls++
	return store.inner.Save(ctx, plan)
}
func (store *jsonBudgetPlanStore) Get(ctx context.Context, hash string) (action.Plan, error) {
	store.calls++
	return store.inner.Get(ctx, hash)
}
func (store *jsonBudgetPlanStore) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	store.calls++
	return store.inner.DeleteExpired(ctx, before)
}

type jsonBudgetIdempotencyStore struct {
	inner     *testIdempotencyStore
	calls     int
	completes int
}

func (store *jsonBudgetIdempotencyStore) Lookup(ctx context.Context, record actionpersistence.IdempotencyRecord) (*actionpersistence.IdempotencyRecord, error) {
	store.calls++
	return store.inner.Lookup(ctx, record)
}
func (store *jsonBudgetIdempotencyStore) Reserve(ctx context.Context, record actionpersistence.IdempotencyRecord) (*actionpersistence.IdempotencyRecord, error) {
	store.calls++
	return store.inner.Reserve(ctx, record)
}
func (store *jsonBudgetIdempotencyStore) Complete(ctx context.Context, record actionpersistence.IdempotencyRecord) error {
	store.calls++
	store.completes++
	return store.inner.Complete(ctx, record)
}
func (store *jsonBudgetIdempotencyStore) Abort(ctx context.Context, record actionpersistence.IdempotencyRecord) error {
	store.calls++
	return store.inner.Abort(ctx, record)
}

type jsonBudgetAudit struct {
	allowed int
	events  []audit.Event
}

func (hook *jsonBudgetAudit) Record(_ context.Context, event audit.Event) error {
	hook.events = append(hook.events, event)
	if event.Decision == "allowed" || event.Decision == "previewed" || event.Decision == "idempotent_replay" {
		hook.allowed++
	}
	return nil
}
