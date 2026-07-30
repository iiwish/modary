package action

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"modary/core/audit"
	"modary/core/authz"
	"modary/core/identity"
)

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

func TestRuntimeRejectsDeniedActorAndInvalidInput(t *testing.T) {
	clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	runtime, _ := newTestRuntime(t, &testHandler{}, &clock)
	request := testRequest()
	request.Actor.Permissions = nil
	if _, err := runtime.Preview(context.Background(), request); !IsCode(err, CodeAuthzDenied) {
		t.Fatalf("denied error = %v", err)
	}
	request = testRequest()
	request.Input = json.RawMessage(`{"value":"not-an-integer"}`)
	if _, err := runtime.Preview(context.Background(), request); !IsCode(err, CodeValidationFailed) {
		t.Fatalf("validation error = %v", err)
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

func TestRegistryRejectsIncompleteGovernanceDescriptor(t *testing.T) {
	registry := NewRegistry()
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
}

type testHandler struct {
	executions int
	executeErr error
}

func (h *testHandler) Plan(_ context.Context, request Request) (PlanData, error) {
	return PlanData{
		Payload: json.RawMessage(`{"value":1}`),
		Summary: json.RawMessage(`{"matched_rows":1}`),
		Impact:  authz.Impact{Rows: 1},
	}, nil
}

func (h *testHandler) Execute(context.Context, Plan) (Result, error) {
	h.executions++
	if h.executeErr != nil {
		return Result{}, h.executeErr
	}
	return Result{Data: json.RawMessage(`{"ok":true}`), Summary: "executed one row"}, nil
}

type testAuthorizer struct{}

func (testAuthorizer) Authorize(_ context.Context, request authz.Request) (authz.Decision, error) {
	for _, permission := range request.Actor.Permissions {
		if permission == request.Permission {
			return authz.Decision{Allowed: true, Fingerprint: "policy-v1"}, nil
		}
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

func newTestRuntime(t *testing.T, handler Handler, clock *time.Time) (*Runtime, *collectingAudit) {
	t.Helper()
	registry := NewRegistry()
	err := registry.Register("test", testDescriptor(), handler)
	if err != nil {
		t.Fatal(err)
	}
	events := &collectingAudit{}
	runtime, err := NewRuntime(RuntimeOptions{
		Registry:   registry,
		Authorizer: testAuthorizer{},
		Audit:      events,
		Clock:      func() time.Time { return *clock },
		PlanTTL:    5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtime, events
}

func testDescriptor() Descriptor {
	return Descriptor{
		ID:                  "test.execute",
		Title:               "Test execute",
		InputSchema:         ObjectSchema(`"value":{"type":"integer"}`, "value"),
		OutputSchema:        ObjectSchema(`"ok":{"type":"boolean"}`, "ok"),
		Permission:          "test.execute",
		Preview:             PreviewRequired,
		AuditLevel:          AuditDetailed,
		Channels:            []string{"http", "cli", "mcp"},
		RequiresIdempotency: true,
	}
}

func testRequest() Request {
	return Request{
		RequestID:   "req_test",
		Actor:       identity.Actor{ID: "user_1", Type: "user", WorkspaceID: "ws_default", Permissions: []string{"test.execute"}},
		Channel:     "http",
		ActionID:    "test.execute",
		WorkspaceID: "ws_default",
		Input:       json.RawMessage(`{"value":1}`),
	}
}
