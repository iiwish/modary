package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/components/postgres/internal/postgrestest"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/scope"
)

func TestEmptyProvisioningIsDefaultDenyAndCreatesNoPolicy(t *testing.T) {
	db, authorizer := openAuthorizer(t, Options{})
	for _, table := range []string{"modary_rbac_role", "modary_rbac_role_permission", "modary_rbac_binding"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s contains %d rows after empty provisioning", table, count)
		}
	}
	decision, err := authorizer.Authorize(context.Background(), policyRequest("counter.write"))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.RequiredPermission != "counter.write" || decision.Fingerprint == "" {
		t.Fatalf("default decision = %#v", decision)
	}
	if err := authorizer.provision(context.Background(), Options{}); err != nil {
		t.Fatalf("repeat empty provisioning: %v", err)
	}
}

func TestExactActorScopePermissionAndRowConstraints(t *testing.T) {
	options := Options{
		Roles: []Role{
			{ID: "reader", Permissions: []string{"counter.read"}, MaxRows: 5},
			{ID: "writer-small", Permissions: []string{"counter.write"}, MaxRows: 10},
			{ID: "writer-large", Permissions: []string{"counter.write"}, MaxRows: 50},
		},
		Bindings: []Binding{
			binding("reader"), binding("writer-small"), binding("writer-large"),
		},
	}
	_, authorizer := openAuthorizer(t, options)
	decision, err := authorizer.Authorize(context.Background(), policyRequest("counter.write"))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Constraints.MaxRows != 50 || decision.RequiredPermission != "counter.write" || decision.Fingerprint == "" {
		t.Fatalf("write decision = %#v", decision)
	}
	denied, err := authorizer.Authorize(context.Background(), policyRequest("counter.delete"))
	if err != nil || denied.Allowed || denied.Fingerprint != decision.Fingerprint {
		t.Fatalf("denied decision = %#v, %v", denied, err)
	}

	wrongActor := policyRequest("counter.write")
	wrongActor.Actor.ID = "person-two"
	if got, err := authorizer.Authorize(context.Background(), wrongActor); err != nil || got.Allowed {
		t.Fatalf("wrong actor decision = %#v, %v", got, err)
	}
	wrongType := policyRequest("counter.write")
	wrongType.Actor.Type = "service"
	if got, err := authorizer.Authorize(context.Background(), wrongType); err != nil || got.Allowed {
		t.Fatalf("wrong actor type decision = %#v, %v", got, err)
	}
	wrongScope := policyRequest("counter.write")
	wrongScope.Scope = scope.Must("account", "account-2")
	if got, err := authorizer.Authorize(context.Background(), wrongScope); err != nil || got.Allowed {
		t.Fatalf("mismatched actor scope decision = %#v, %v", got, err)
	}

	unbounded := Options{
		Roles:    []Role{{ID: "writer-unbounded", Permissions: []string{"counter.write"}}},
		Bindings: []Binding{{ActorID: "person-one", ActorType: "human", Scope: testScope(), RoleID: "writer-unbounded"}},
	}
	if err := authorizer.provision(context.Background(), unbounded); err != nil {
		t.Fatal(err)
	}
	decision, err = authorizer.Authorize(context.Background(), policyRequest("counter.write"))
	if err != nil || !decision.Allowed || decision.Constraints.MaxRows != 0 {
		t.Fatalf("unbounded decision = %#v, %v", decision, err)
	}
}

func TestRequestEnvelopeAndImpactLimitsFailClosed(t *testing.T) {
	_, authorizer := openAuthorizer(t, Options{
		Roles:    []Role{{ID: "writer", Permissions: []string{"counter.write"}, MaxRows: 10}},
		Bindings: []Binding{binding("writer")},
	})

	invalidRequests := []struct {
		name   string
		mutate func(*authz.Request)
	}{
		{name: "operation id", mutate: func(request *authz.Request) { request.OperationID = "counter/write" }},
		{name: "phase", mutate: func(request *authz.Request) { request.Phase = "unknown" }},
		{name: "negative rows", mutate: func(request *authz.Request) {
			request.Phase = authz.PhaseImpact
			request.Impact.Rows = -1
		}},
		{name: "intent impact", mutate: func(request *authz.Request) { request.Impact.Rows = 1 }},
	}
	for _, test := range invalidRequests {
		t.Run(test.name, func(t *testing.T) {
			request := policyRequest("counter.write")
			test.mutate(&request)
			decision, err := authorizer.Authorize(context.Background(), request)
			if err != nil || decision.Allowed || decision.Fingerprint == "" {
				t.Fatalf("invalid request decision = %#v, %v", decision, err)
			}
		})
	}

	request := policyRequest("counter.write")
	request.Phase = authz.PhaseImpact
	request.Impact = authz.Impact{Rows: 11, Resources: []string{"counter"}}
	decision, err := authorizer.Authorize(context.Background(), request)
	if err != nil || !decision.Allowed || decision.Code != "" || decision.Reason != "" ||
		decision.Constraints.MaxRows != 10 || decision.Fingerprint == "" {
		t.Fatalf("over-limit decision = %#v, %v", decision, err)
	}
	request.Impact.Rows = 10
	decision, err = authorizer.Authorize(context.Background(), request)
	if err != nil || !decision.Allowed {
		t.Fatalf("at-limit decision = %#v, %v", decision, err)
	}
}

func TestOpaqueActorIdentifiersFollowTheKernelContract(t *testing.T) {
	executionScope := scope.Must("tenant", "tenant-one")
	actor := identity.Actor{
		ID:          "01JABCDEF|user@example.test",
		Type:        "外部身份/service",
		DisplayName: "External User",
		Scope:       executionScope,
	}
	_, authorizer := openAuthorizer(t, Options{
		Roles: []Role{{ID: "writer", Permissions: []string{"counter.write"}}},
		Bindings: []Binding{{
			ActorID: actor.ID, ActorType: actor.Type, Scope: executionScope, RoleID: "writer",
		}},
	})
	decision, err := authorizer.Authorize(context.Background(), authz.Request{
		Actor: actor, OperationID: "counter.write", Permission: "counter.write", Scope: executionScope, Phase: authz.PhaseIntent,
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("opaque actor decision = %#v, %v", decision, err)
	}
}

func TestFingerprintIsStableAndChangesWithEffectivePolicy(t *testing.T) {
	options := Options{
		Roles:    []Role{{ID: "writer", Permissions: []string{"counter.write", "counter.read"}, MaxRows: 10}},
		Bindings: []Binding{binding("writer")},
	}
	_, evaluator := openAuthorizer(t, options)
	request := policyRequest("counter.write")
	intent, err := evaluator.Authorize(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.Phase = authz.PhaseImpact
	request.Impact = authz.Impact{Rows: 9, Resources: []string{"counter"}}
	impact, err := evaluator.Authorize(context.Background(), request)
	if err != nil || impact.Fingerprint != intent.Fingerprint {
		t.Fatalf("impact decision = %#v, %v; intent = %#v", impact, err, intent)
	}

	restarted := &authorizer{control: evaluator.control}
	repeated, err := restarted.Authorize(context.Background(), request)
	if err != nil || repeated.Fingerprint != intent.Fingerprint {
		t.Fatalf("restart fingerprint = %#v, %v", repeated, err)
	}
	changed := Options{Roles: []Role{{ID: "writer", Permissions: []string{"counter.write", "counter.read"}, MaxRows: 9}}}
	if err := restarted.provision(context.Background(), changed); err != nil {
		t.Fatal(err)
	}
	after, err := restarted.Authorize(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if after.Fingerprint == intent.Fingerprint || after.Constraints.MaxRows != 9 {
		t.Fatalf("changed decision = %#v, before = %#v", after, intent)
	}
	changed.Roles[0].Permissions = []string{"counter.read"}
	if err := restarted.provision(context.Background(), changed); err != nil {
		t.Fatal(err)
	}
	denied, err := restarted.Authorize(context.Background(), request)
	if err != nil || denied.Allowed || denied.Fingerprint == after.Fingerprint {
		t.Fatalf("permission mutation decision = %#v, %v", denied, err)
	}
}

func TestProvisioningAndRevocationAreIdempotent(t *testing.T) {
	options := Options{
		Roles:    []Role{{ID: "writer", Permissions: []string{"counter.write"}, MaxRows: 10}},
		Bindings: []Binding{binding("writer")},
	}
	_, authorizer := openAuthorizer(t, options)
	if err := authorizer.provision(context.Background(), options); err != nil {
		t.Fatalf("repeat provision: %v", err)
	}
	if decision, _ := authorizer.Authorize(context.Background(), policyRequest("counter.write")); !decision.Allowed {
		t.Fatalf("provisioned decision = %#v", decision)
	}
	revokeBinding := Options{RevokedBindings: []Binding{binding("writer")}}
	if err := authorizer.provision(context.Background(), revokeBinding); err != nil {
		t.Fatal(err)
	}
	if err := authorizer.provision(context.Background(), revokeBinding); err != nil {
		t.Fatalf("repeat binding revocation: %v", err)
	}
	if decision, _ := authorizer.Authorize(context.Background(), policyRequest("counter.write")); decision.Allowed {
		t.Fatalf("revoked binding decision = %#v", decision)
	}
	if err := authorizer.provision(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	revokeRole := Options{RevokedRoleIDs: []string{"writer"}}
	if err := authorizer.provision(context.Background(), revokeRole); err != nil {
		t.Fatal(err)
	}
	if decision, _ := authorizer.Authorize(context.Background(), policyRequest("counter.write")); decision.Allowed {
		t.Fatalf("revoked role decision = %#v", decision)
	}
	if err := authorizer.provision(context.Background(), Options{
		Roles: []Role{{ID: "writer", Permissions: []string{"counter.write"}, MaxRows: 10}},
	}); err != nil {
		t.Fatal(err)
	}
	if decision, _ := authorizer.Authorize(context.Background(), policyRequest("counter.write")); decision.Allowed {
		t.Fatalf("role reactivation resurrected binding: %#v", decision)
	}
	if err := authorizer.provision(context.Background(), Options{Bindings: []Binding{binding("writer")}}); err != nil {
		t.Fatal(err)
	}
	if decision, _ := authorizer.Authorize(context.Background(), policyRequest("counter.write")); !decision.Allowed {
		t.Fatalf("explicitly reactivated binding decision = %#v", decision)
	}
}

func TestProvisioningRollsBackOnMissingRole(t *testing.T) {
	db := openRBACDatabase(t)
	control, err := postgrestest.NewControl(db)
	if err != nil {
		t.Fatal(err)
	}
	authorizer := &authorizer{control: control}
	options := Options{
		Roles:    []Role{{ID: "reader", Permissions: []string{"counter.read"}}},
		Bindings: []Binding{binding("missing-role")},
	}
	if err := authorizer.provision(context.Background(), options); err == nil {
		t.Fatal("provisioning with missing role succeeded")
	}
	for _, table := range []string{"modary_rbac_role", "modary_rbac_role_permission", "modary_rbac_binding"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count = %d, %v", table, count, err)
		}
	}
}

func TestCorruptStoredPolicyFailsClosed(t *testing.T) {
	db, _ := openAuthorizer(t, Options{
		Roles: []Role{{ID: "writer", Permissions: []string{"counter.write"}}}, Bindings: []Binding{binding("writer")},
	})
	if _, err := db.Exec(`UPDATE modary_rbac_role SET max_rows = -1 WHERE role_id = 'writer'`); err == nil {
		t.Fatal("negative policy limit passed the PostgreSQL constraint")
	}
}

func TestPostgresSchemaRejectsOversizedPolicyFields(t *testing.T) {
	t.Run("permission", func(t *testing.T) {
		db, _ := openAuthorizer(t, Options{
			Roles: []Role{{ID: "writer", Permissions: []string{"counter.write"}}}, Bindings: []Binding{binding("writer")},
		})
		oversized := strings.Repeat("p", maxStoredPolicyTokenBytes+1)
		if _, err := db.Exec(`UPDATE modary_rbac_role_permission SET permission = $1`, oversized); err == nil {
			t.Fatal("oversized permission passed the PostgreSQL constraint")
		}
	})

	t.Run("role id", func(t *testing.T) {
		db, _ := openAuthorizer(t, Options{})
		oversized := strings.Repeat("r", maxStoredPolicyTokenBytes+1)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := db.Exec(`INSERT INTO modary_rbac_role (role_id, max_rows, active, created_at, updated_at) VALUES ($1, 0, TRUE, $2, $3)`, oversized, now, now); err == nil {
			t.Fatal("oversized role id passed the PostgreSQL constraint")
		}
	})
}

func TestStoredPolicyRejectsExcessiveEffectiveRows(t *testing.T) {
	db, authorizer := openAuthorizer(t, Options{
		Roles: []Role{{ID: "writer", Permissions: []string{"counter.write"}}}, Bindings: []Binding{binding("writer")},
	})
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO modary_rbac_role_permission (role_id, permission) VALUES ('writer', $1)`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	for index := 0; index < maxEffectivePolicyRows; index++ {
		if _, err := statement.Exec(fmt.Sprintf("counter.extra_%04d", index)); err != nil {
			_ = statement.Close()
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := statement.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	decision, err := authorizer.Authorize(context.Background(), policyRequest("counter.write"))
	if err == nil || decision.Allowed || !strings.Contains(err.Error(), "effective rows") {
		t.Fatalf("oversized effective policy decision = %#v, %v", decision, err)
	}
}

func TestOptionsFailClosedAndOwnInput(t *testing.T) {
	base := Options{
		Roles:    []Role{{ID: "writer", Permissions: []string{"counter.write"}}},
		Bindings: []Binding{binding("writer")},
	}
	tests := []struct {
		name   string
		mutate func(*Options)
	}{
		{name: "invalid role", mutate: func(options *Options) { options.Roles[0].ID = "Invalid" }},
		{name: "negative limit", mutate: func(options *Options) { options.Roles[0].MaxRows = -1 }},
		{name: "duplicate role", mutate: func(options *Options) { options.Roles = append(options.Roles, options.Roles[0]) }},
		{name: "invalid permission", mutate: func(options *Options) { options.Roles[0].Permissions = []string{"Invalid Permission"} }},
		{name: "path permission", mutate: func(options *Options) { options.Roles[0].Permissions = []string{"counter/write"} }},
		{name: "duplicate permission", mutate: func(options *Options) { options.Roles[0].Permissions = []string{"counter.write", "counter.write"} }},
		{name: "duplicate binding", mutate: func(options *Options) { options.Bindings = append(options.Bindings, options.Bindings[0]) }},
		{name: "invalid binding scope", mutate: func(options *Options) { options.Bindings[0].Scope = scope.Execution{} }},
		{name: "role provision and revoke", mutate: func(options *Options) { options.RevokedRoleIDs = []string{"writer"} }},
		{name: "binding to revoked role", mutate: func(options *Options) {
			options.Roles = nil
			options.RevokedRoleIDs = []string{"writer"}
		}},
		{name: "binding provision and revoke", mutate: func(options *Options) { options.RevokedBindings = []Binding{binding("writer")} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := cloneTestOptions(base)
			test.mutate(&options)
			registration, err := Module(options)
			if err == nil || registration.Definition.Manifest.ID != "" {
				t.Fatalf("Module() = %#v, %v", registration, err)
			}
		})
	}

	options := cloneTestOptions(base)
	registration, err := Module(options)
	if err != nil {
		t.Fatal(err)
	}
	options.Roles[0].Permissions[0] = "mutated.permission"
	options.Bindings[0].ActorID = "mutated-actor"
	if registration.Definition.Manifest.ID != ModuleID || registration.Start == nil || len(registration.Definition.Migrations) != 1 {
		t.Fatalf("registration = %#v", registration.Definition)
	}
}

func TestAuthorizeRejectsNilContextAndUnavailableService(t *testing.T) {
	request := policyRequest("counter.write")
	if _, err := (&authorizer{}).Authorize(nil, request); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("Authorize(nil) error = %v", err)
	}
	if _, err := (&authorizer{}).Authorize(context.Background(), request); err == nil {
		t.Fatal("unavailable Authorizer succeeded")
	}
}

func TestConcurrentReadsAndPolicyRefresh(t *testing.T) {
	options := Options{Roles: []Role{{ID: "writer", Permissions: []string{"counter.write"}, MaxRows: 10}}, Bindings: []Binding{binding("writer")}}
	_, authorizer := openAuthorizer(t, options)
	const workers = 8
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 20 {
				decision, err := authorizer.Authorize(context.Background(), policyRequest("counter.write"))
				if err != nil || !decision.Allowed {
					t.Errorf("Authorize() = %#v, %v", decision, err)
					return
				}
			}
		}()
	}
	group.Wait()
}

func TestAuthorizeUsesTheCallerTransaction(t *testing.T) {
	options := Options{
		Roles:    []Role{{ID: "writer", Permissions: []string{"counter.write"}, MaxRows: 10}},
		Bindings: []Binding{binding("writer")},
	}
	db, authorizer := openAuthorizer(t, options)
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	rollback := errors.New("rollback policy")
	err := authorizer.control.WithinTransaction(ctx, func(txContext context.Context) error {
		executor, err := authorizer.control.Executor(txContext)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(txContext, `UPDATE modary_rbac_role SET max_rows = 25 WHERE role_id = 'writer'`); err != nil {
			return err
		}
		decision, err := authorizer.Authorize(txContext, policyRequest("counter.write"))
		if err != nil {
			return fmt.Errorf("Authorize() in caller transaction: %w", err)
		}
		if !decision.Allowed || decision.Constraints.MaxRows != 25 {
			t.Fatalf("transaction policy decision = %#v", decision)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction error = %v", err)
	}
	outside, err := authorizer.Authorize(context.Background(), policyRequest("counter.write"))
	if err != nil || !outside.Allowed || outside.Constraints.MaxRows != 10 {
		t.Fatalf("rolled-back policy decision = %#v, %v", outside, err)
	}
}

func openAuthorizer(t *testing.T, raw Options) (*sql.DB, *authorizer) {
	t.Helper()
	options, err := normalizeOptions(raw)
	if err != nil {
		t.Fatal(err)
	}
	db := openRBACDatabase(t)
	control, err := postgrestest.NewControl(db)
	if err != nil {
		t.Fatal(err)
	}
	authorizer := &authorizer{control: control}
	if err := authorizer.provision(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	return db, authorizer
}

func openRBACDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, control := postgrestest.Open(t)
	if err := control.ApplyMigrations(context.Background(), ModuleID, postgresMigrations); err != nil {
		t.Fatal(err)
	}
	return db
}

func testScope() scope.Execution { return scope.Must("account", "account-1") }

func binding(roleID string) Binding {
	return Binding{ActorID: "person-one", ActorType: "human", Scope: testScope(), RoleID: roleID}
}

func policyRequest(permission string) authz.Request {
	execution := testScope()
	return authz.Request{
		Actor:       identity.Actor{ID: "person-one", Type: "human", DisplayName: "Person One", Scope: execution},
		OperationID: "counter.write", Permission: permission, Scope: execution, Phase: authz.PhaseIntent,
	}
}

func cloneTestOptions(options Options) Options {
	clone := options
	clone.Roles = append([]Role(nil), options.Roles...)
	for index := range clone.Roles {
		clone.Roles[index].Permissions = append([]string(nil), clone.Roles[index].Permissions...)
	}
	clone.Bindings = append([]Binding(nil), options.Bindings...)
	clone.RevokedRoleIDs = append([]string(nil), options.RevokedRoleIDs...)
	clone.RevokedBindings = append([]Binding(nil), options.RevokedBindings...)
	return clone
}
