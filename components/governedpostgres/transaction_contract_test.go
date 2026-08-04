package governedpostgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/actionruntime"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/internal/testsupport"
	"github.com/iiwish/modary/scope"
)

func TestPostgresPreservesNestedRuntimeAndDatabaseTransactionMarkers(t *testing.T) {
	services := startTestServices(t)
	if _, err := services.db.Exec(`CREATE TABLE transaction_contract_probe (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	want := errors.New("business operation failed")
	handler := &transactionContractHandler{database: services.access, cause: want}
	descriptor := action.Descriptor{
		ID:          "probe.write",
		Version:     "1.0.0",
		Title:       "Write transaction probe",
		InputSchema: action.Object(nil).JSON(),
		OutputSchema: action.Object(map[string]action.Field{
			"ok": action.RequiredField(action.Boolean()),
		}).JSON(),
		Permission: "probe.write",
		Preview:    action.PreviewNone,
		AuditLevel: action.AuditMetadata,
		Channels:   []action.Channel{"test"},
	}
	registry := actionruntime.NewRegistry()
	if err := registry.Register("probe", descriptor, handler); err != nil {
		t.Fatal(err)
	}
	runtime, err := actionruntime.New(registry, actionruntime.Options{
		Authorizer:   transactionContractAuthorizer{},
		Audit:        testsupport.DiscardAudit{},
		Plans:        services.plans,
		Idempotency:  services.idempotency,
		Transactions: services.transactions,
		Clock:        func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) },
		AuditFailure: func(context.Context, error, audit.Event) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	executionScope := scope.Must("tenant", "transaction-contract")
	actor := identity.Actor{ID: "actor-1", Type: "user"}
	result, err := runtime.Execute(context.Background(), action.Request{
		RequestID: "transaction-contract",
		Actor:     actor,
		Channel:   "test",
		ActionID:  descriptor.ID,
		Scope:     executionScope,
		Input:     json.RawMessage(`{}`),
	})
	if action.ErrorCode(err) != action.CodeInternal || !errors.Is(err, want) || errors.Is(err, runtimecontrol.ErrTransactionManagerContract) {
		t.Fatalf("Execute() = %#v, %v", result, err)
	}
	var count int
	if err := services.db.QueryRow(`SELECT COUNT(*) FROM transaction_contract_probe`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 || handler.calls != 1 {
		t.Fatalf("probe rows = %d, Handler calls = %d", count, handler.calls)
	}
}

type transactionContractHandler struct {
	database database.Access
	cause    error
	calls    int
}

func (*transactionContractHandler) Plan(context.Context, action.Request) (action.PlanData, error) {
	return action.PlanData{Payload: json.RawMessage(`{}`)}, nil
}

func (handler *transactionContractHandler) Execute(ctx context.Context, _ action.Plan) (action.Result, error) {
	handler.calls++
	if _, err := handler.database.ExecContext(ctx, `INSERT INTO transaction_contract_probe (id) VALUES (1)`); err != nil {
		return action.Result{}, err
	}
	return action.Result{}, handler.cause
}

type transactionContractAuthorizer struct{}

func (transactionContractAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Allowed: true, Fingerprint: "transaction-contract-v1"}, nil
}
