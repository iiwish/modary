package postgres

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/scope"
)

func TestPlanValidationRejectsInvalidProvenance(t *testing.T) {
	tests := map[string]func(*action.Plan){
		"plan hash":            func(plan *action.Plan) { plan.Hash = "a" },
		"action id":            func(plan *action.Plan) { plan.ActionID = "Document.Write" },
		"action id path":       func(plan *action.Plan) { plan.ActionID = "document/write" },
		"action version":       func(plan *action.Plan) { plan.ActionVersion = "01.2.3" },
		"contract hash":        func(plan *action.Plan) { plan.ContractHash = digest('z') },
		"actor id":             func(plan *action.Plan) { plan.ActorID = " person" },
		"actor type":           func(plan *action.Plan) { plan.ActorType = "" },
		"channel":              func(plan *action.Plan) { plan.Channel = "mcp\n" },
		"scope kind":           func(plan *action.Plan) { plan.Scope = scope.Execution{Kind: "Account", ID: "one"} },
		"scope id":             func(plan *action.Plan) { plan.Scope = scope.Execution{Kind: "account", ID: ""} },
		"input hash":           func(plan *action.Plan) { plan.InputHash = digest('g') },
		"payload":              func(plan *action.Plan) { plan.Payload = json.RawMessage(`{} {}`) },
		"negative impact":      func(plan *action.Plan) { plan.Impact.Rows = -1 },
		"duplicate resource":   func(plan *action.Plan) { plan.Impact.Resources = []string{"one", "one"} },
		"snapshot hash":        func(plan *action.Plan) { plan.SnapshotHash = "sha256:short" },
		"fingerprint empty":    func(plan *action.Plan) { plan.DecisionFingerprint = "" },
		"fingerprint control":  func(plan *action.Plan) { plan.DecisionFingerprint = "bad\nvalue" },
		"fingerprint too long": func(plan *action.Plan) { plan.DecisionFingerprint = strings.Repeat("x", authz.MaxFingerprintRunes+1) },
		"creation time":        func(plan *action.Plan) { plan.CreatedAt = time.Time{} },
		"expiry time":          func(plan *action.Plan) { plan.ExpiresAt = time.Time{} },
		"expiry ordering":      func(plan *action.Plan) { plan.ExpiresAt = plan.CreatedAt },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			plan := clonePlanForTest(validPlan())
			mutate(&plan)
			if _, err := normalizePlan(plan); err == nil {
				t.Fatalf("normalizePlan accepted %#v", plan)
			}
		})
	}
	if _, err := normalizePlan(validPlan()); err != nil {
		t.Fatalf("valid plan: %v", err)
	}
	maximumFingerprint := validPlan()
	maximumFingerprint.DecisionFingerprint = strings.Repeat("界", authz.MaxFingerprintRunes)
	if _, err := normalizePlan(maximumFingerprint); err != nil {
		t.Fatalf("maximum-length plan fingerprint: %v", err)
	}
}

func TestIdempotencyValidationRejectsInvalidBindingsAndResults(t *testing.T) {
	reservationTests := map[string]func(*actionpersistence.IdempotencyRecord){
		"scope":                func(record *actionpersistence.IdempotencyRecord) { record.Scope = scope.Execution{} },
		"actor id":             func(record *actionpersistence.IdempotencyRecord) { record.ActorID = "" },
		"actor type":           func(record *actionpersistence.IdempotencyRecord) { record.ActorType = " service" },
		"action id":            func(record *actionpersistence.IdempotencyRecord) { record.ActionID = "Write" },
		"action id path":       func(record *actionpersistence.IdempotencyRecord) { record.ActionID = "document/write" },
		"action version":       func(record *actionpersistence.IdempotencyRecord) { record.ActionVersion = "1" },
		"contract hash":        func(record *actionpersistence.IdempotencyRecord) { record.ContractHash = "" },
		"channel":              func(record *actionpersistence.IdempotencyRecord) { record.Channel = "" },
		"key":                  func(record *actionpersistence.IdempotencyRecord) { record.Key = " retry" },
		"input hash":           func(record *actionpersistence.IdempotencyRecord) { record.InputHash = "bad" },
		"plan hash":            func(record *actionpersistence.IdempotencyRecord) { record.PlanHash = "" },
		"impact":               func(record *actionpersistence.IdempotencyRecord) { record.Impact.Rows = -1 },
		"decision fingerprint": func(record *actionpersistence.IdempotencyRecord) { record.DecisionFingerprint = "bad\rvalue" },
		"fingerprint too long": func(record *actionpersistence.IdempotencyRecord) {
			record.DecisionFingerprint = strings.Repeat("x", authz.MaxFingerprintRunes+1)
		},
		"status": func(record *actionpersistence.IdempotencyRecord) {
			record.Status = actionpersistence.IdempotencyCompleted
		},
		"running result": func(record *actionpersistence.IdempotencyRecord) {
			record.Result = action.Result{Data: json.RawMessage(`{"ok":true}`)}
		},
	}
	for name, mutate := range reservationTests {
		t.Run("reservation "+name, func(t *testing.T) {
			record := cloneRecordForTest(validReservation())
			mutate(&record)
			if _, err := normalizeReservation(record); err == nil {
				t.Fatalf("normalizeReservation accepted %#v", record)
			}
		})
	}
	if _, err := normalizeReservation(validReservation()); err != nil {
		t.Fatalf("valid reservation: %v", err)
	}
	maximumFingerprintRecord := validReservation()
	maximumFingerprintRecord.DecisionFingerprint = strings.Repeat("界", authz.MaxFingerprintRunes)
	if _, err := normalizeReservation(maximumFingerprintRecord); err != nil {
		t.Fatalf("maximum-length idempotency fingerprint: %v", err)
	}

	validCompletion := validReservation()
	validCompletion.Status = actionpersistence.IdempotencyCompleted
	validCompletion.Result = action.Result{
		Data:       json.RawMessage(`{"ok":true}`),
		Summary:    "complete",
		References: []audit.Reference{{Kind: "write", ID: "one"}},
	}
	completionTests := map[string]func(*actionpersistence.IdempotencyRecord){
		"status": func(record *actionpersistence.IdempotencyRecord) {
			record.Status = actionpersistence.IdempotencyRunning
		},
		"result JSON": func(record *actionpersistence.IdempotencyRecord) {
			record.Result.Data = json.RawMessage(`{"unterminated"`)
		},
		"summary": func(record *actionpersistence.IdempotencyRecord) { record.Result.Summary = "bad\nsummary" },
		"reference kind": func(record *actionpersistence.IdempotencyRecord) {
			record.Result.References = []audit.Reference{{Kind: "", ID: "one"}}
		},
		"reference id": func(record *actionpersistence.IdempotencyRecord) {
			record.Result.References = []audit.Reference{{Kind: "write", ID: " one"}}
		},
		"duplicate reference": func(record *actionpersistence.IdempotencyRecord) {
			record.Result.References = []audit.Reference{{Kind: "write", ID: "one"}, {Kind: "write", ID: "one"}}
		},
	}
	for name, mutate := range completionTests {
		t.Run("completion "+name, func(t *testing.T) {
			record := cloneRecordForTest(validCompletion)
			mutate(&record)
			if _, err := normalizeCompletion(record); err == nil {
				t.Fatalf("normalizeCompletion accepted %#v", record)
			}
		})
	}
	if _, err := normalizeCompletion(validCompletion); err != nil {
		t.Fatalf("valid completion: %v", err)
	}
}
