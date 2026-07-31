package actionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
)

func TestNormalizeHandlerFailureEnforcesDeclaredSourceContract(t *testing.T) {
	descriptor := testDescriptor()
	descriptor.Errors = []action.ErrorSpec{
		{Code: "COUNTER.NOT_READY", Kind: action.ErrorKindPrecondition},
		{Code: "COUNTER.MISSING", Kind: action.ErrorKindNotFound},
		{Code: "COUNTER.RETRY_LATER", Kind: action.ErrorKindUnavailable},
		{Code: "POLICY.ACCOUNT_LOCKED", Kind: action.ErrorKindDenied},
	}

	valid := []struct {
		name string
		err  *action.Error
		kind action.ErrorKind
	}{
		{name: "validation", err: action.NewError(action.CodeValidationFailed, "input is invalid"), kind: action.ErrorKindValidation},
		{name: "precondition", err: action.NewError(action.CodePreconditionFailed, "state is not ready"), kind: action.ErrorKindPrecondition},
		{name: "stale", err: action.NewError(action.CodePlanStale, "state changed"), kind: action.ErrorKindConflict},
		{name: "limit", err: action.NewError(action.CodeLimitExceeded, "value exceeds the limit"), kind: action.ErrorKindLimit},
		{name: "declared custom", err: action.NewError("COUNTER.NOT_READY", "counter is not ready"), kind: action.ErrorKindPrecondition},
		{name: "declared unavailable", err: action.NewError("COUNTER.RETRY_LATER", "counter is temporarily unavailable"), kind: action.ErrorKindUnavailable},
	}
	for _, test := range valid {
		t.Run("valid "+test.name, func(t *testing.T) {
			got := normalizeHandlerFailure(fmt.Errorf("handler: %w", test.err), descriptor)
			if action.ErrorCode(got) != test.err.Code || action.ErrorKindOf(got) != test.kind || !errors.Is(got, test.err) {
				t.Fatalf("normalized error = %#v / %v", got, got)
			}
			projected, ok := findActionError(got)
			if !ok || projected.Message != test.err.Message || projected.Kind != test.kind {
				t.Fatalf("projected error = %#v", projected)
			}
		})
	}

	typedNil := (*action.Error)(nil)
	conflicting := errors.Join(
		action.NewError(action.CodeValidationFailed, "first"),
		action.NewError(action.CodePreconditionFailed, "second"),
	)
	nested := action.NewError(action.CodeValidationFailed, "outer")
	nested.Cause = action.NewError(action.CodeValidationFailed, "inner")
	invalid := []struct {
		name string
		err  error
	}{
		{name: "ordinary", err: errors.New("database password=secret")},
		{name: "typed nil", err: typedNil},
		{name: "multiple Action errors", err: conflicting},
		{name: "nested Action errors", err: nested},
		{name: "undeclared custom", err: action.NewError("COUNTER.UNKNOWN", "unknown failure")},
		{name: "declared denial", err: action.NewError("POLICY.ACCOUNT_LOCKED", "account is locked")},
		{name: "authorization built-in", err: action.NewError(action.CodeAuthzDenied, "not allowed")},
		{name: "not-found built-in", err: action.NewError(action.CodeActionNotFound, "not found")},
		{name: "plan-required built-in", err: action.NewError(action.CodePlanRequired, "plan required")},
		{name: "idempotency built-in", err: action.NewError(action.CodeIdempotencyConflict, "key conflict")},
		{name: "unavailable built-in", err: action.NewError(action.CodeUnavailable, "unavailable")},
		{name: "internal built-in", err: action.NewError(action.CodeInternal, "secret internal detail")},
		{name: "mismatched kind", err: &action.Error{Code: action.CodeValidationFailed, Kind: action.ErrorKindDenied, Message: "invalid"}},
		{name: "empty message", err: action.NewError(action.CodeValidationFailed, "")},
		{name: "surrounding whitespace", err: action.NewError(action.CodeValidationFailed, " invalid ")},
		{name: "control message", err: action.NewError(action.CodeValidationFailed, "invalid\nvalue")},
		{name: "line separator", err: action.NewError(action.CodeValidationFailed, "invalid\u2028value")},
		{name: "invalid UTF-8", err: action.NewError(action.CodeValidationFailed, string([]byte{0xff}))},
		{name: "oversized message", err: action.NewError(action.CodeValidationFailed, strings.Repeat("x", action.MaxErrorMessageRunes+1))},
	}
	for _, test := range invalid {
		t.Run("invalid "+test.name, func(t *testing.T) {
			got := normalizeHandlerFailure(test.err, descriptor)
			projected, ok := findActionError(got)
			if !ok || projected.Code != action.CodeInternal || projected.Kind != action.ErrorKindInternal || projected.Message != "handler returned an invalid error" {
				t.Fatalf("invalid error projection = %#v / %v", projected, got)
			}
			if strings.Contains(got.Error(), "secret") {
				t.Fatalf("invalid handler detail escaped: %v", got)
			}
		})
	}
}

func TestNormalizeHandlerFailureAcceptsWithRequestEnrichment(t *testing.T) {
	descriptor := testDescriptor()
	request := testRequest()
	handlerErr := action.WithRequest(action.NewError(action.CodePreconditionFailed, "state is not ready"), request, descriptor.Permission)
	got := normalizeHandlerFailure(handlerErr, descriptor)
	if action.ErrorCode(got) != action.CodePreconditionFailed || action.ErrorKindOf(got) != action.ErrorKindPrecondition {
		t.Fatalf("normalized enriched handler error = %v", got)
	}
}

func TestFinalizeRuntimeFailureRejectsMalformedEnvelopes(t *testing.T) {
	malformed := []error{
		errors.New("ordinary failure"),
		&action.Error{Code: action.CodeValidationFailed, Kind: action.ErrorKindDenied, Message: "invalid"},
		&action.Error{Code: "NOT_QUALIFIED", Kind: action.ErrorKindConflict, Message: "conflict"},
		&action.Error{Code: action.CodeValidationFailed, Kind: action.ErrorKindValidation, Message: strings.Repeat("x", action.MaxErrorMessageRunes+1)},
	}
	for _, input := range malformed {
		got := finalizeRuntimeFailure(input)
		projected, ok := findActionError(got)
		if !ok || projected.Code != action.CodeInternal || projected.Kind != action.ErrorKindInternal || projected.Message != "action execution failed" {
			t.Fatalf("finalized malformed error = %#v", projected)
		}
	}
	valid := action.NewError(action.CodeValidationFailed, "input is invalid")
	if got := finalizeRuntimeFailure(valid); got != valid {
		t.Fatalf("valid error identity changed: %#v", got)
	}
}

func TestRuntimeRestrictsAuthorizerDenialCodesToDeclaredDeniedKind(t *testing.T) {
	descriptor := testDescriptor()
	descriptor.Errors = []action.ErrorSpec{
		{Code: "POLICY.ACCOUNT_LOCKED", Kind: action.ErrorKindDenied},
		{Code: "COUNTER.NOT_READY", Kind: action.ErrorKindPrecondition},
	}
	tests := []struct {
		name     string
		decision authz.Decision
		code     string
		kind     action.ErrorKind
		outcome  string
	}{
		{name: "default denial", decision: deniedDecision("", "permission is missing"), code: action.CodeAuthzDenied, kind: action.ErrorKindDenied, outcome: "denied"},
		{name: "declared custom denial", decision: deniedDecision("POLICY.ACCOUNT_LOCKED", "account is locked"), code: "POLICY.ACCOUNT_LOCKED", kind: action.ErrorKindDenied, outcome: "denied"},
		{name: "validation spoof", decision: deniedDecision(action.CodeValidationFailed, "spoofed"), code: action.CodeInternal, kind: action.ErrorKindInternal, outcome: "failed"},
		{name: "internal spoof", decision: deniedDecision(action.CodeInternal, "private"), code: action.CodeInternal, kind: action.ErrorKindInternal, outcome: "failed"},
		{name: "unavailable spoof", decision: deniedDecision(action.CodeUnavailable, "spoofed"), code: action.CodeInternal, kind: action.ErrorKindInternal, outcome: "failed"},
		{name: "wrong custom kind", decision: deniedDecision("COUNTER.NOT_READY", "spoofed"), code: action.CodeInternal, kind: action.ErrorKindInternal, outcome: "failed"},
		{name: "undeclared custom", decision: deniedDecision("POLICY.UNKNOWN", "spoofed"), code: action.CodeInternal, kind: action.ErrorKindInternal, outcome: "failed"},
		{name: "line separator reason", decision: deniedDecision("", "first\u2028second"), code: action.CodeInternal, kind: action.ErrorKindInternal, outcome: "failed"},
		{name: "paragraph separator reason", decision: deniedDecision("", "first\u2029second"), code: action.CodeInternal, kind: action.ErrorKindInternal, outcome: "failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := NewRegistry()
			handler := &countingPlanHandler{}
			if err := registry.Register("test", descriptor, handler); err != nil {
				t.Fatal(err)
			}
			events := &collectingAudit{}
			runtime := mustRuntime(t, registry, Options{
				Authorizer: fixedDecisionAuthorizer{decision: test.decision}, Audit: events,
				Plans: newMemoryPlanStore(), Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
				Clock: func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) },
			})
			_, err := runtime.Preview(context.Background(), testRequest())
			if action.ErrorCode(err) != test.code || action.ErrorKindOf(err) != test.kind {
				t.Fatalf("denial error = %v, kind=%q", err, action.ErrorKindOf(err))
			}
			if handler.calls.Load() != 0 {
				t.Fatalf("invalid or denied decision reached Handler.Plan %d time(s)", handler.calls.Load())
			}
			assertLastFailureAudit(t, events, test.outcome, test.code, test.kind)
		})
	}
}

func TestRuntimePublishesDeclaredCustomHandlerErrorAcrossAudit(t *testing.T) {
	descriptor := testDescriptor()
	descriptor.Preview = action.PreviewNone
	descriptor.PreviewSchema = nil
	descriptor.RequiresIdempotency = false
	descriptor.Errors = []action.ErrorSpec{{Code: "COUNTER.NOT_READY", Kind: action.ErrorKindPrecondition}}
	handlerErr := action.NewError("COUNTER.NOT_READY", "counter is not ready")
	events := &collectingAudit{}
	runtime := newDependencyCodeRuntime(t, descriptor, dependencyCodeHandler{executeErr: handlerErr}, Options{Audit: events})

	_, err := runtime.Execute(context.Background(), testRequest())
	if action.ErrorCode(err) != handlerErr.Code || action.ErrorKindOf(err) != action.ErrorKindPrecondition || !errors.Is(err, handlerErr) {
		t.Fatalf("custom handler error = %v, kind=%q", err, action.ErrorKindOf(err))
	}
	assertLastFailureAudit(t, events, "rejected", handlerErr.Code, action.ErrorKindPrecondition)
	events.mu.Lock()
	reason := events.events[len(events.events)-1].Reason
	events.mu.Unlock()
	if reason != handlerErr.Message {
		t.Fatalf("audit reason = %q, want %q", reason, handlerErr.Message)
	}
}

func TestRuntimeOwnsBoundedValidationMessagesAtEveryExit(t *testing.T) {
	fields := make(map[string]action.Field, 100)
	for index := range 100 {
		fields[fmt.Sprintf("required_%03d", index)] = action.RequiredField(action.String())
	}
	descriptor := testDescriptor()
	descriptor.InputSchema = action.Object(fields).JSON()
	registry := NewRegistry()
	if err := registry.Register("test", descriptor, &countingPlanHandler{}); err != nil {
		t.Fatal(err)
	}
	events := &collectingAudit{}
	runtime := mustRuntime(t, registry, Options{
		Authorizer: testAuthorizer{}, Audit: events, Plans: newMemoryPlanStore(),
		Idempotency: newMemoryIdempotencyStore(), Transactions: confirmedTransactionManager{},
	})

	for _, run := range []struct {
		name string
		call func(action.Request) error
	}{
		{name: "preview", call: func(request action.Request) error {
			_, err := runtime.Preview(context.Background(), request)
			return err
		}},
		{name: "execute", call: func(request action.Request) error {
			_, err := runtime.Execute(context.Background(), request)
			return err
		}},
	} {
		t.Run(run.name+" schema", func(t *testing.T) {
			request := testRequest()
			request.Input = json.RawMessage(`{}`)
			err := run.call(request)
			projected, ok := findActionError(err)
			if !ok || projected.Code != action.CodeValidationFailed || projected.Kind != action.ErrorKindValidation ||
				projected.Message != "input does not satisfy the Action schema" || !action.ValidErrorMessage(projected.Message) {
				t.Fatalf("schema failure = %#v / %v", projected, err)
			}
		})
		t.Run(run.name+" action id", func(t *testing.T) {
			request := testRequest()
			request.ActionID = strings.Repeat("x", 4096)
			err := run.call(request)
			projected, ok := findActionError(err)
			if !ok || projected.Code != action.CodeValidationFailed || projected.Kind != action.ErrorKindValidation ||
				projected.Message != "action id is invalid" || projected.ActionID != "" {
				t.Fatalf("identifier failure = %#v / %v", projected, err)
			}
		})
	}

	events.mu.Lock()
	defer events.mu.Unlock()
	for _, event := range events.events {
		if !action.ValidErrorMessage(event.Reason) {
			t.Fatalf("audit contains invalid public reason: %#v", event)
		}
	}
}

func TestRuntimeAuditClassifiesBusinessAndInternalFailures(t *testing.T) {
	tests := []struct {
		name    string
		handler action.Handler
		actor   string
		code    string
		kind    action.ErrorKind
		outcome string
	}{
		{name: "business rejection", handler: dependencyCodeHandler{planErr: action.NewError(action.CodePreconditionFailed, "not ready")}, code: action.CodePreconditionFailed, kind: action.ErrorKindPrecondition, outcome: "rejected"},
		{name: "authorization denial", handler: dependencyCodeHandler{}, actor: "denied", code: action.CodeAuthzDenied, kind: action.ErrorKindDenied, outcome: "denied"},
		{name: "internal handler failure", handler: dependencyCodeHandler{planErr: errors.New("private dependency detail")}, code: action.CodeInternal, kind: action.ErrorKindInternal, outcome: "failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
			runtime, events := newTestRuntime(t, test.handler, &clock)
			request := testRequest()
			if test.actor != "" {
				request.Actor.Type = test.actor
			}
			_, err := runtime.Preview(context.Background(), request)
			if action.ErrorCode(err) != test.code || action.ErrorKindOf(err) != test.kind {
				t.Fatalf("failure = %v, kind=%q", err, action.ErrorKindOf(err))
			}
			assertLastFailureAudit(t, events, test.outcome, test.code, test.kind)
		})
	}
}

func deniedDecision(code, reason string) authz.Decision {
	return authz.Decision{Code: code, Reason: reason, Fingerprint: "policy-v1"}
}

func assertLastFailureAudit(t *testing.T, events *collectingAudit, outcome, code string, kind action.ErrorKind) {
	t.Helper()
	events.mu.Lock()
	defer events.mu.Unlock()
	if len(events.events) == 0 {
		t.Fatal("failure audit was not recorded")
	}
	event := events.events[len(events.events)-1]
	if event.Decision != outcome || event.ErrorCode != code || event.ErrorKind != string(kind) {
		t.Fatalf("failure audit = %#v", event)
	}
}

var _ audit.Hook = (*collectingAudit)(nil)
