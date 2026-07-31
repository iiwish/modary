package adapters_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/adapters/localidentity"
	"github.com/iiwish/modary/adapters/rbac"
	"github.com/iiwish/modary/adapters/sqlaudit"
	modarysqlite "github.com/iiwish/modary/adapters/sqlite"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/scope"
	_ "modernc.org/sqlite"
)

const (
	conformanceActionID = "counter.increment"
	conformancePassword = "correct horse battery staple"
	conformanceToken    = "token_0123456789abcdef0123456789abcdef0123456789abcdef"
)

var conformanceMigrations fs.FS = fstest.MapFS{
	"0001_counter.sql": &fstest.MapFile{Data: []byte(`
CREATE TABLE counter_value (
    scope_kind TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    value INTEGER NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (scope_kind, scope_id)
) STRICT;
`)},
}

func TestOfficialAdaptersComposeForGovernedWrites(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "conformance.db")
	executionScope := scope.Must("tenant", "tenant-alpha")
	actor := identity.Actor{
		ID:          "person-one",
		Type:        "human",
		DisplayName: "Person One",
		Scope:       executionScope,
	}

	application := startConformanceApplication(t, databasePath, executionScope, 5)
	sessions, err := application.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	session, err := sessions.Login(ctx, "person@example.test", conformancePassword)
	if err != nil || session.Actor != actor || session.Token == "" || session.CSRFToken == "" {
		t.Fatalf("Login() = %#v, %v", session, err)
	}
	tokens, err := application.Tokens()
	if err != nil {
		t.Fatal(err)
	}
	tokenActor, err := tokens.AuthenticateToken(ctx, conformanceToken)
	if err != nil || tokenActor != actor {
		t.Fatalf("AuthenticateToken() = %#v, %v", tokenActor, err)
	}

	first := action.Request{
		RequestID:      "counter-first",
		Actor:          actor,
		Channel:        action.ChannelCLI,
		ActionID:       conformanceActionID,
		Scope:          executionScope,
		Input:          json.RawMessage(`{"delta":3}`),
		IdempotencyKey: "counter-first-key",
	}
	preview, err := application.Runtime().Preview(ctx, first)
	if err != nil || preview.PlanHash == "" || preview.Impact.Rows != 1 {
		t.Fatalf("Preview() = %#v, %v", preview, err)
	}
	first.PlanHash = preview.PlanHash
	result, err := application.Runtime().Execute(ctx, first)
	if err != nil || string(result.Data) != `{"value":3}` {
		t.Fatalf("Execute() = %#v, %v", result, err)
	}
	replayed, err := application.Runtime().Execute(ctx, first)
	if err != nil || string(replayed.Data) != string(result.Data) {
		t.Fatalf("in-process replay = %#v, %v", replayed, err)
	}

	staleCandidate := action.Request{
		RequestID:      "counter-stale",
		Actor:          actor,
		Channel:        action.ChannelCLI,
		ActionID:       conformanceActionID,
		Scope:          executionScope,
		Input:          json.RawMessage(`{"delta":1}`),
		IdempotencyKey: "counter-stale-key",
	}
	stalePreview, err := application.Runtime().Preview(ctx, staleCandidate)
	if err != nil {
		t.Fatal(err)
	}
	staleCandidate.PlanHash = stalePreview.PlanHash
	shutdownApplication(t, application)

	application = startConformanceApplication(t, databasePath, executionScope, 5)
	restartReplay := first
	restartReplay.RequestID = "counter-restart-replay"
	replayedResult, err := application.Runtime().Execute(ctx, restartReplay)
	if err != nil || string(replayedResult.Data) != string(result.Data) {
		t.Fatalf("restart replay = %#v, %v", replayedResult, err)
	}
	shutdownApplication(t, application)

	application = startConformanceApplication(t, databasePath, executionScope, 10)
	if _, err := application.Runtime().Execute(ctx, staleCandidate); !action.IsCode(err, action.CodePlanStale) {
		t.Fatalf("Execute() after policy change error = %v", err)
	}

	failing := action.Request{
		RequestID:      "counter-transaction-failure",
		Actor:          actor,
		Channel:        action.ChannelCLI,
		ActionID:       conformanceActionID,
		Scope:          executionScope,
		Input:          json.RawMessage(`{"delta":7,"fail":true}`),
		IdempotencyKey: "counter-failure-key",
	}
	failingPreview, err := application.Runtime().Preview(ctx, failing)
	if err != nil {
		t.Fatal(err)
	}
	failing.PlanHash = failingPreview.PlanHash
	if _, err := application.Runtime().Execute(ctx, failing); err == nil {
		t.Fatal("failing Action unexpectedly succeeded")
	}
	shutdownApplication(t, application)

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value, version int
	if err := db.QueryRowContext(ctx, `
		SELECT value, version FROM counter_value
		WHERE scope_kind = ? AND scope_id = ?`, executionScope.Kind, executionScope.ID).Scan(&value, &version); err != nil {
		t.Fatal(err)
	}
	if value != 3 || version != 1 {
		t.Fatalf("counter after rollback = value %d, version %d; want 3, 1", value, version)
	}
	assertRowCount(t, db, 1, `
		SELECT COUNT(*) FROM modary_action_idempotency
		WHERE idempotency_key = 'counter-first-key' AND status = 'completed'
		  AND actor_type = 'human' AND channel = 'cli'
		  AND scope_kind = 'tenant' AND scope_id = 'tenant-alpha'`)
	assertRowCount(t, db, 0, `
		SELECT COUNT(*) FROM modary_action_idempotency
		WHERE idempotency_key = 'counter-failure-key'`)
	assertRowCount(t, db, 1, `
		SELECT COUNT(*) FROM modary_audit_event
		WHERE request_id = 'counter-first' AND decision = 'allowed'
		  AND action_version = '0.1.0' AND length(contract_hash) = 71
		  AND actor_type = 'human' AND channel = 'cli'
		  AND scope_kind = 'tenant' AND scope_id = 'tenant-alpha'`)
	assertRowCount(t, db, 1, `
		SELECT COUNT(*) FROM modary_audit_event
		WHERE request_id = 'counter-transaction-failure' AND decision = 'failed'`)
	assertRowCount(t, db, 0, `
		SELECT COUNT(*) FROM modary_audit_event
		WHERE request_id = 'counter-transaction-failure' AND decision = 'allowed'`)
}

func startConformanceApplication(t *testing.T, path string, executionScope scope.Execution, maxRows int) *appkit.Application {
	t.Helper()
	sqliteRegistration, err := modarysqlite.Module(modarysqlite.Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	identityRegistration, err := localidentity.Module(localidentity.Options{
		Users: []localidentity.User{{
			ActorID:     "person-one",
			ActorType:   "human",
			DisplayName: "Person One",
			Scope:       executionScope,
			Username:    "person@example.test",
			Password:    conformancePassword,
		}},
		BearerTokens: []localidentity.BearerToken{{
			TokenID: "automation-one",
			ActorID: "person-one",
			Token:   conformanceToken,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rbacRegistration, err := rbac.Module(rbac.Options{
		Roles: []rbac.Role{{
			ID:          "counter-writer",
			Permissions: []string{conformanceActionID},
			MaxRows:     maxRows,
		}},
		Bindings: []rbac.Binding{{
			ActorID:   "person-one",
			ActorType: "human",
			Scope:     executionScope,
			RoleID:    "counter-writer",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := appkit.Start(context.Background(), appkit.Definition{
		Metadata: appkit.Metadata{ID: "adapter-conformance", Name: "Adapter Conformance", Version: "0.1.0"},
		Modules: []module.Registration{
			sqliteRegistration,
			identityRegistration,
			rbacRegistration,
			sqlaudit.Module(sqlaudit.Options{}),
			counterRegistration(),
		},
	}, appkit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return application
}

func shutdownApplication(t *testing.T, application *appkit.Application) {
	t.Helper()
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func counterRegistration() module.Registration {
	return module.Registration{
		Definition: module.Definition{
			Manifest: module.Manifest{
				SchemaVersion: module.SchemaVersion,
				ID:            "counter-conformance",
				Version:       "0.1.0",
				Type:          module.ModuleTypeFeature,
				Requires:      []module.Capability{module.CapabilityDatabase},
				Provides:      []module.Capability{"counter"},
			},
			Migrations: []module.MigrationSource{{Driver: "sqlite", Files: conformanceMigrations}},
			Actions: []module.ActionBinding{{
				Descriptor: action.Descriptor{
					ID:      conformanceActionID,
					Version: "0.1.0",
					Title:   "Increment counter",
					InputSchema: action.Object(map[string]action.Field{
						"delta": action.RequiredField(action.Integer(action.Minimum(1), action.Maximum(100))),
						"fail":  action.OptionalField(action.Boolean()),
					}).JSON(),
					PreviewSchema: action.Object(map[string]action.Field{
						"before": action.RequiredField(action.Integer()),
						"after":  action.RequiredField(action.Integer()),
					}).JSON(),
					OutputSchema: action.Object(map[string]action.Field{
						"value": action.RequiredField(action.Integer()),
					}).JSON(),
					Permission:          conformanceActionID,
					Preview:             action.PreviewRequired,
					AuditLevel:          action.AuditDetailed,
					Channels:            []action.Channel{action.ChannelCLI, action.ChannelHTTP, action.ChannelMCP},
					RequiresIdempotency: true,
				},
				NewHandler: func(_ context.Context, services module.Resolver) (action.Handler, error) {
					db, err := module.Resolve(services, module.Database())
					if err != nil {
						return nil, err
					}
					return &counterHandler{db: db}, nil
				},
			}},
		},
	}
}

type counterInput struct {
	Delta int  `json:"delta"`
	Fail  bool `json:"fail,omitempty"`
}

type counterPlan struct {
	ExpectedVersion int  `json:"expected_version"`
	NextValue       int  `json:"next_value"`
	Fail            bool `json:"fail"`
}

type counterHandler struct{ db database.Access }

func (handler *counterHandler) Plan(ctx context.Context, request action.Request) (action.PlanData, error) {
	if _, err := handler.db.ExecContext(ctx, `UPDATE counter_value SET value = value`); !errors.Is(err, database.ErrTransactionRequired) {
		return action.PlanData{}, fmt.Errorf("counter Plan write boundary error = %v", err)
	}
	var input counterInput
	if err := json.Unmarshal(request.Input, &input); err != nil {
		return action.PlanData{}, err
	}
	var current, version int
	err := handler.db.QueryRowContext(ctx, `
		SELECT value, version FROM counter_value
		WHERE scope_kind = ? AND scope_id = ?`, request.Scope.Kind, request.Scope.ID).Scan(&current, &version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return action.PlanData{}, err
	}
	payload, err := json.Marshal(counterPlan{ExpectedVersion: version, NextValue: current + input.Delta, Fail: input.Fail})
	if err != nil {
		return action.PlanData{}, err
	}
	summary, err := json.Marshal(struct {
		Before int `json:"before"`
		After  int `json:"after"`
	}{Before: current, After: current + input.Delta})
	if err != nil {
		return action.PlanData{}, err
	}
	return action.PlanData{
		Payload:      payload,
		Summary:      summary,
		Impact:       authz.Impact{Rows: 1, Resources: []string{"counter/" + request.Scope.ID}},
		SnapshotHash: counterSnapshot(current, version),
	}, nil
}

func (handler *counterHandler) Execute(ctx context.Context, plan action.Plan) (action.Result, error) {
	var payload counterPlan
	if err := json.Unmarshal(plan.Payload, &payload); err != nil {
		return action.Result{}, err
	}
	result, err := handler.db.ExecContext(ctx, `
		INSERT INTO counter_value (scope_kind, scope_id, value, version)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(scope_kind, scope_id) DO UPDATE SET
			value = excluded.value,
			version = counter_value.version + 1
		WHERE counter_value.version = ?`,
		plan.Scope.Kind, plan.Scope.ID, payload.NextValue, payload.ExpectedVersion)
	if err != nil {
		return action.Result{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return action.Result{}, err
	}
	if rows != 1 {
		return action.Result{}, fmt.Errorf("counter changed after preview")
	}
	if payload.Fail {
		return action.Result{}, fmt.Errorf("injected counter failure")
	}
	data, err := json.Marshal(struct {
		Value int `json:"value"`
	}{Value: payload.NextValue})
	if err != nil {
		return action.Result{}, err
	}
	return action.Result{
		Data:       data,
		Summary:    fmt.Sprintf("counter is %d", payload.NextValue),
		References: []audit.Reference{{Kind: "counter", ID: plan.Scope.ID}},
	}, nil
}

func counterSnapshot(value, version int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d", value, version)))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func assertRowCount(t *testing.T, db *sql.DB, want int, query string) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query row count = %d, want %d: %s", got, want, query)
	}
}
