package module

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	executionscope "github.com/iiwish/modary/scope"
)

var testDatabaseService = MustKey[string]("test.database", "database")

type inertActionHandler struct{}

type allowAllAuthorizer struct{}

func (allowAllAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Allowed: true, Fingerprint: "test-policy"}, nil
}

type countingAuditHook struct{ calls atomic.Int32 }

func (hook *countingAuditHook) Record(context.Context, audit.Event) error {
	hook.calls.Add(1)
	return nil
}

type blockingPlanHandler struct {
	entered  chan struct{}
	finished chan struct{}
}

type delayedPlanHandler struct {
	entered  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (handler *delayedPlanHandler) Plan(ctx context.Context, _ action.Request) (action.PlanData, error) {
	close(handler.entered)
	<-handler.release
	close(handler.finished)
	return action.PlanData{}, ctx.Err()
}

func (*delayedPlanHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	return action.Result{}, nil
}

func (handler *blockingPlanHandler) Plan(ctx context.Context, _ action.Request) (action.PlanData, error) {
	close(handler.entered)
	<-ctx.Done()
	close(handler.finished)
	return action.PlanData{}, ctx.Err()
}

func (*blockingPlanHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	return action.Result{}, nil
}

type panicMigrationFS struct{}

func (panicMigrationFS) Open(string) (fs.File, error) {
	panic("migration files must not be opened while inspecting a Definition")
}

func (inertActionHandler) Plan(context.Context, action.Request) (action.PlanData, error) {
	return action.PlanData{}, nil
}

func (inertActionHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	return action.Result{}, nil
}

func TestScopeEnforcesTypedDeclaredCapabilities(t *testing.T) {
	host := NewHost()
	provider := Register(validManifest("database", "adapter", nil, []string{"database"}), func(_ context.Context, scope Scope) error {
		return Provide(scope, testDatabaseService, "database-value")
	})
	var resolved string
	consumer := Register(validManifest("feature", "feature", []string{"database"}, []string{"feature"}), func(_ context.Context, scope Scope) error {
		value, err := Resolve(scope, testDatabaseService)
		resolved = value
		return err
	})
	if err := host.Register(provider, consumer); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if resolved != "database-value" {
		t.Fatalf("resolved = %q", resolved)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestScopeRejectsUndeclaredResolveAndProvide(t *testing.T) {
	tests := []struct {
		name  string
		start StartFunc
	}{
		{name: "resolve", start: func(_ context.Context, scope Scope) error {
			_, err := Resolve(scope, testDatabaseService)
			return err
		}},
		{name: "provide", start: func(_ context.Context, scope Scope) error {
			return Provide(scope, testDatabaseService, "forbidden")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := NewHost()
			if err := host.Register(Register(validManifest("feature", "feature", nil, []string{"feature"}), test.start)); err != nil {
				t.Fatal(err)
			}
			err := host.Start(context.Background())
			if err == nil || !strings.Contains(err.Error(), "start callback failed") {
				t.Fatalf("Start() error = %v", err)
			}
		})
	}
}

func TestCatalogIsPureAndAssignsActionOwner(t *testing.T) {
	var started, built atomic.Int32
	host := NewHost()
	registration := Register(
		validManifest("counter", "feature", nil, []string{"counter"}),
		func(context.Context, Scope) error { started.Add(1); return nil },
		ActionBinding{
			Descriptor: testActionDescriptor("counter.increment"),
			NewHandler: func(context.Context, Resolver) (action.Handler, error) {
				built.Add(1)
				return inertActionHandler{}, nil
			},
		},
	)
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	catalog, err := host.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if started.Load() != 0 || built.Load() != 0 {
		t.Fatalf("pure catalog invoked Start=%d factory=%d", started.Load(), built.Load())
	}
	if len(catalog) != 1 || catalog[0].ModuleID != "counter" || catalog[0].Descriptor.ID != "counter.increment" {
		t.Fatalf("catalog = %#v", catalog)
	}
	if _, exposed := reflect.TypeOf(catalog[0]).FieldByName("Handler"); exposed {
		t.Fatal("catalog exposes Handler")
	}
}

func TestCatalogRejectsDuplicateActionAcrossModules(t *testing.T) {
	host := NewHost()
	binding := ActionBinding{
		Descriptor: testActionDescriptor("counter.increment"),
		NewHandler: func(context.Context, Resolver) (action.Handler, error) {
			return inertActionHandler{}, nil
		},
	}
	err := host.Register(
		Register(validManifest("one", "feature", nil, []string{"one"}), nil, binding),
		Register(validManifest("two", "feature", nil, []string{"two"}), nil, binding),
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate action") {
		t.Fatalf("Register() error = %v", err)
	}
	if len(host.Manifests()) != 0 {
		t.Fatalf("duplicate Action batch partially registered: %#v", host.Manifests())
	}
}

func TestShutdownUsesReverseDependencyAndLIFOOrder(t *testing.T) {
	var order []string
	registerCleanup := func(scope Scope, names ...string) error {
		for _, name := range names {
			name := name
			if err := OnStop(scope, func(context.Context) error { order = append(order, name); return nil }); err != nil {
				return err
			}
		}
		return nil
	}
	host := NewHost()
	provider := Register(validManifest("provider", "adapter", nil, []string{"database"}), func(_ context.Context, scope Scope) error {
		if err := Provide(scope, testDatabaseService, "value"); err != nil {
			return err
		}
		return registerCleanup(scope, "provider-1", "provider-2")
	})
	consumer := Register(validManifest("consumer", "feature", []string{"database"}, []string{"feature"}), func(_ context.Context, scope Scope) error {
		return registerCleanup(scope, "consumer-1", "consumer-2")
	})
	if err := host.Register(consumer, provider); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"consumer-2", "consumer-1", "provider-2", "provider-1"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("cleanup order = %v, want %v", order, want)
	}
	if _, err := resolveHostService(host, testDatabaseService); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Resolve after stop error = %v", err)
	}
}

func TestPartialStartFailureRollsBackCurrentAndStartedModules(t *testing.T) {
	var order []string
	host := NewHost()
	provider := Register(validManifest("provider", "adapter", nil, []string{"database"}), func(_ context.Context, scope Scope) error {
		if err := Provide(scope, testDatabaseService, "value"); err != nil {
			return err
		}
		return OnStop(scope, func(context.Context) error { order = append(order, "provider"); return nil })
	})
	consumer := Register(validManifest("consumer", "feature", []string{"database"}, []string{"feature"}), func(_ context.Context, scope Scope) error {
		if err := OnStop(scope, func(ctx context.Context) error {
			if ctx.Err() != nil {
				t.Errorf("cleanup inherited canceled context: %v", ctx.Err())
			}
			order = append(order, "consumer")
			return nil
		}); err != nil {
			return err
		}
		return errors.New("boom")
	})
	if err := host.Register(provider, consumer); err != nil {
		t.Fatal(err)
	}
	err := host.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start callback failed") || strings.Contains(err.Error(), "boom") {
		t.Fatalf("Start() error = %v", err)
	}
	if !reflect.DeepEqual(order, []string{"consumer", "provider"}) {
		t.Fatalf("cleanup order = %v", order)
	}
	if host.State() != StateFailed {
		t.Fatalf("state = %s", host.State())
	}
}

func TestCleanupAttemptsAllCallbacksAndJoinsErrorsExactlyOnce(t *testing.T) {
	var calls atomic.Int32
	errOne := errors.New("cleanup one")
	errTwo := errors.New("cleanup two")
	host := NewHost()
	registration := Register(validManifest("feature", "feature", nil, []string{"feature"}), func(_ context.Context, scope Scope) error {
		if err := OnStop(scope, func(context.Context) error { calls.Add(1); return errOne }); err != nil {
			return err
		}
		return OnStop(scope, func(context.Context) error { calls.Add(1); return errTwo })
	})
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := host.Shutdown(context.Background())
	if !errors.Is(err, errOne) || !errors.Is(err, errTwo) || calls.Load() != 2 {
		t.Fatalf("Shutdown() error = %v, calls = %d", err, calls.Load())
	}
	err = host.Shutdown(context.Background())
	if !errors.Is(err, errOne) || !errors.Is(err, errTwo) || calls.Load() != 2 {
		t.Fatalf("second Shutdown() error = %v, calls = %d", err, calls.Load())
	}
}

func TestHostRejectsInvalidStateTransitions(t *testing.T) {
	host := NewHost()
	registration := Register(validManifest("feature", "feature", nil, []string{"feature"}), nil)
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := host.Register(registration); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Register after start error = %v", err)
	}
	if err := host.Start(context.Background()); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second Start error = %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterIsAtomicAndCopiesDefinitionData(t *testing.T) {
	host := NewHost()
	first := Register(validManifest("first", "feature", nil, []string{"first"}), nil)
	duplicate := Register(validManifest("first", "feature", nil, []string{"other"}), nil)
	if err := host.Register(first, duplicate); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if len(host.Manifests()) != 0 {
		t.Fatalf("failed batch partially registered: %#v", host.Manifests())
	}

	mutable := Register(validManifest("mutable", "feature", nil, []string{"original"}), nil)
	if err := host.Register(mutable); err != nil {
		t.Fatal(err)
	}
	mutable.Definition.Manifest.Provides[0] = "changed"
	if got := host.Manifests()[0].Provides[0]; got != "original" {
		t.Fatalf("registered definition mutated through caller slice: %q", got)
	}
	returned := host.Manifests()
	returned[0].Provides[0] = "mutated-return"
	if got := host.Manifests()[0].Provides[0]; got != "original" {
		t.Fatalf("manifest mutated through returned slice: %q", got)
	}
}

func TestDefinitionOwnsActionsAndMigrationsWithoutInspectingRuntimeValues(t *testing.T) {
	var built atomic.Int32
	actions := []ActionBinding{{
		Descriptor: testActionDescriptor("counter.increment"),
		NewHandler: func(context.Context, Resolver) (action.Handler, error) {
			built.Add(1)
			return inertActionHandler{}, nil
		},
	}}
	migrations := []MigrationSource{{Driver: "sqlite", Files: panicMigrationFS{}}}
	registration := Registration{
		Definition: Definition{
			Manifest:   validManifest("counter", "feature", nil, []string{"counter", "database"}),
			Actions:    actions,
			Migrations: migrations,
		},
	}
	if _, exposed := reflect.TypeOf(registration).FieldByName("Actions"); exposed {
		t.Fatal("Registration duplicates Definition.Actions")
	}
	host := NewHost()
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	actions[0].Descriptor.ID = "mutated.action"
	migrations[0].Driver = "INVALID"
	catalog, err := host.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if built.Load() != 0 {
		t.Fatalf("Definition inspection built %d handlers", built.Load())
	}
	if len(catalog) != 1 || catalog[0].Descriptor.ID != "counter.increment" {
		t.Fatalf("catalog = %#v", catalog)
	}
	encoded, err := json.Marshal(registration.Definition)
	if err != nil {
		t.Fatalf("marshal Definition: %v", err)
	}
	if strings.Contains(string(encoded), "NewHandler") || !strings.Contains(string(encoded), `"driver":"INVALID"`) {
		t.Fatalf("serialized Definition = %s", encoded)
	}
}

func TestDefinitionRejectsInvalidMigrationDeclarations(t *testing.T) {
	tests := []struct {
		name       string
		migrations []MigrationSource
		want       string
	}{
		{name: "driver", migrations: []MigrationSource{{Driver: "SQLite!", Files: panicMigrationFS{}}}, want: "invalid migration driver"},
		{name: "nil files", migrations: []MigrationSource{{Driver: "sqlite"}}, want: "has no files"},
		{name: "typed nil files", migrations: []MigrationSource{{Driver: "sqlite", Files: (*panicMigrationFS)(nil)}}, want: "has no files"},
		{name: "duplicate driver", migrations: []MigrationSource{{Driver: "sqlite", Files: panicMigrationFS{}}, {Driver: "sqlite", Files: panicMigrationFS{}}}, want: "more than once"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := NewHost()
			registration := Registration{Definition: Definition{
				Manifest:   validManifest("feature", "feature", []string{"database"}, []string{"feature"}),
				Migrations: test.migrations,
			}}
			if err := host.Register(registration); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Register() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRegisterRejectsMigrationsWithoutDatabaseCapabilityBeforeLifecycle(t *testing.T) {
	var starts atomic.Int32
	host := NewHost()
	registration := Register(
		validManifest("unbound-migration", "feature", nil, []string{"feature"}),
		func(context.Context, Scope) error {
			starts.Add(1)
			return nil
		},
	)
	registration.Definition.Migrations = []MigrationSource{{Driver: "sqlite", Files: panicMigrationFS{}}}
	if err := host.Register(registration); err == nil || !strings.Contains(err.Error(), "without requiring or providing capability \"database\"") {
		t.Fatalf("Register() error = %v", err)
	}
	if starts.Load() != 0 {
		t.Fatalf("registration invoked Start %d time(s)", starts.Load())
	}
	catalog, err := host.Catalog()
	if err != nil || len(catalog) != 0 {
		t.Fatalf("Catalog() = %#v, %v", catalog, err)
	}
}

func TestServiceKeyIdentityCannotBeRecreatedFromPublicStrings(t *testing.T) {
	providerKey := MustKey[string]("shared.name", "database")
	tests := []struct {
		name    string
		consume StartFunc
	}{
		{
			name: "same type",
			consume: func(_ context.Context, scope Scope) error {
				_, err := Resolve(scope, MustKey[string]("shared.name", "database"))
				return err
			},
		},
		{
			name: "different type",
			consume: func(_ context.Context, scope Scope) error {
				_, err := Resolve(scope, MustKey[int]("shared.name", "database"))
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := NewHost()
			provider := Register(validManifest("provider", "adapter", nil, []string{"database"}), func(_ context.Context, scope Scope) error {
				return Provide(scope, providerKey, "value")
			})
			consumer := Register(validManifest("consumer", "feature", []string{"database"}, []string{"feature"}), test.consume)
			if err := host.Register(provider, consumer); err != nil {
				t.Fatal(err)
			}
			if err := host.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "start callback failed") {
				t.Fatalf("Start() error = %v", err)
			}
		})
	}
}

func TestStandardServiceKeyAccessorsReturnStableUnforgeableIdentity(t *testing.T) {
	first := Authorizer()
	second := Authorizer()
	forged := MustKey[authz.Authorizer](first.Name(), first.Capability())
	if first.spec.identity != second.spec.identity {
		t.Fatal("standard key accessor returned different identities")
	}
	if first.spec.identity == forged.spec.identity {
		t.Fatal("standard key identity was recreated from its public strings")
	}
}

func TestProvideRejectsTypedNilService(t *testing.T) {
	type service struct{}
	key := MustKey[*service]("nil.service", "feature")
	host := NewHost()
	registration := Register(validManifest("feature", "feature", nil, []string{"feature"}), func(_ context.Context, scope Scope) error {
		var value *service
		return Provide(scope, key, value)
	})
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "start callback failed") {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestScopeSupportsConcurrentCleanupRegistrationAndExpiresAfterStart(t *testing.T) {
	const callbacks = 64
	key := MustKey[string]("scope-expiry.value", "feature")
	var retained Scope
	var cleaned atomic.Int32
	host := NewHost()
	registration := Register(validManifest("feature", "feature", nil, []string{"feature"}), func(_ context.Context, scope Scope) error {
		retained = scope
		var wait sync.WaitGroup
		errors := make(chan error, callbacks)
		for index := 0; index < callbacks; index++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				errors <- OnStop(scope, func(context.Context) error {
					cleaned.Add(1)
					return nil
				})
			}()
		}
		wait.Wait()
		close(errors)
		for err := range errors {
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(retained, key); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("Resolve with retained Scope error = %v, want ErrInvalidScope", err)
	}
	if err := Provide(retained, key, "late"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("Provide with retained Scope error = %v, want ErrInvalidScope", err)
	}
	if err := OnStop(retained, func(context.Context) error { return nil }); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("OnStop with retained Scope error = %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cleaned.Load() != callbacks {
		t.Fatalf("cleanup calls = %d, want %d", cleaned.Load(), callbacks)
	}
}

func TestShutdownDuringStartCancelsWaitsAndRollsBack(t *testing.T) {
	entered := make(chan struct{})
	var cleaned atomic.Int32
	host := NewHost()
	registration := Register(validManifest("feature", "feature", nil, []string{"feature"}), func(ctx context.Context, scope Scope) error {
		if err := OnStop(scope, func(context.Context) error {
			cleaned.Add(1)
			return nil
		}); err != nil {
			return err
		}
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	startResult := make(chan error, 1)
	go func() { startResult <- host.Start(context.Background()) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("module did not begin startup")
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := <-startResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v", err)
	}
	if state := host.State(); state != StateStopped {
		t.Fatalf("state = %s, want %s", state, StateStopped)
	}
	if cleaned.Load() != 1 {
		t.Fatalf("cleanup calls = %d", cleaned.Load())
	}
	if err := host.Shutdown(context.Background()); err != nil || cleaned.Load() != 1 {
		t.Fatalf("second Shutdown() error = %v, cleanup calls = %d", err, cleaned.Load())
	}
}

func TestShutdownAndPartialFailureRevokeActionBindings(t *testing.T) {
	binding := func(id string, factory HandlerFactory) ActionBinding {
		return ActionBinding{Descriptor: testActionDescriptor(id), NewHandler: factory}
	}
	goodFactory := func(context.Context, Resolver) (action.Handler, error) { return inertActionHandler{}, nil }

	t.Run("shutdown", func(t *testing.T) {
		host := NewHost()
		if err := host.Register(Register(validManifest("feature", "feature", nil, []string{"feature"}), nil,
			binding("feature.run", goodFactory))); err != nil {
			t.Fatal(err)
		}
		if err := host.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, ok := host.registry.Resolve("feature.run"); !ok {
			t.Fatal("binding missing after Start")
		}
		if err := host.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, ok := host.registry.Resolve("feature.run"); ok {
			t.Fatal("binding remains after Shutdown")
		}
	})

	t.Run("partial factory failure", func(t *testing.T) {
		host := NewHost()
		factoryErr := errors.New("factory failed")
		if err := host.Register(Register(validManifest("feature", "feature", nil, []string{"feature"}), nil,
			binding("feature.first", goodFactory),
			binding("feature.second", func(context.Context, Resolver) (action.Handler, error) { return nil, factoryErr }))); err != nil {
			t.Fatal(err)
		}
		if err := host.Start(context.Background()); !errors.Is(err, factoryErr) {
			t.Fatalf("Start() error = %v", err)
		}
		if _, ok := host.registry.Resolve("feature.first"); ok {
			t.Fatal("partial binding remains after failed Start")
		}
	})
}

func TestStartRejectsTypedNilHandlerAndRevokesBindings(t *testing.T) {
	host := NewHost()
	registration := Register(
		validManifest("feature", "feature", nil, []string{"feature"}),
		nil,
		ActionBinding{
			Descriptor: testActionDescriptor("feature.first"),
			NewHandler: func(context.Context, Resolver) (action.Handler, error) {
				return inertActionHandler{}, nil
			},
		},
		ActionBinding{
			Descriptor: testActionDescriptor("feature.typed-nil"),
			NewHandler: func(context.Context, Resolver) (action.Handler, error) {
				var handler *inertActionHandler
				return handler, nil
			},
		},
	)
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "has no handler") {
		t.Fatalf("Start() error = %v", err)
	}
	if host.State() != StateFailed {
		t.Fatalf("state = %s, want %s", host.State(), StateFailed)
	}
	if _, ok := host.registry.Resolve("feature.first"); ok {
		t.Fatal("binding remains after typed-nil Handler failure")
	}
	if _, ok := host.registry.Resolve("feature.typed-nil"); ok {
		t.Fatal("typed-nil Handler was bound")
	}
}

func TestShutdownRevokesHeldRuntimeBeforeCleanup(t *testing.T) {
	var runtime action.Runtime
	var executionErr error
	auditHook := &countingAuditHook{}
	host := NewHost()
	registration := Register(
		validManifest("feature", "feature", nil, []string{"feature"}),
		func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error {
				executionScope := executionscope.Must("tenant", "one")
				_, executionErr = runtime.Execute(context.Background(), action.Request{
					RequestID: "during-cleanup",
					Actor:     identity.Actor{ID: "user", Type: "user", Scope: executionScope},
					Channel:   action.ChannelHTTP,
					ActionID:  "feature.run",
					Scope:     executionScope,
					Input:     []byte(`{}`),
				})
				return nil
			})
		},
		ActionBinding{
			Descriptor: testActionDescriptor("feature.run"),
			NewHandler: func(context.Context, Resolver) (action.Handler, error) {
				return inertActionHandler{}, nil
			},
		},
	)
	if err := host.Register(registration, testRuntimeServicesRegistration(testRuntimeServices{audit: auditHook})); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	var err error
	assembly, err := host.Assemble()
	if err != nil {
		t.Fatal(err)
	}
	runtime = assembly.Runtime()
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !action.IsCode(executionErr, action.CodeUnavailable) {
		t.Fatalf("execution during cleanup error = %v", executionErr)
	}
	if auditHook.calls.Load() != 0 {
		t.Fatalf("revoked Runtime touched Audit %d times", auditHook.calls.Load())
	}
}

func TestShutdownCancelsAndDrainsInFlightRuntimeBeforeCleanup(t *testing.T) {
	handler := &blockingPlanHandler{entered: make(chan struct{}), finished: make(chan struct{})}
	var cleanupRan atomic.Bool
	host := NewHost()
	registration := Register(
		validManifest("feature", "feature", nil, []string{"feature"}),
		func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error {
				select {
				case <-handler.finished:
					cleanupRan.Store(true)
					return nil
				default:
					return errors.New("cleanup ran before in-flight Action drained")
				}
			})
		},
		ActionBinding{
			Descriptor: action.Descriptor{
				ID:            "feature.preview",
				Version:       "0.1.0",
				Title:         "Feature preview",
				InputSchema:   action.Object(nil).JSON(),
				PreviewSchema: action.Object(nil).JSON(),
				OutputSchema:  action.Object(nil).JSON(),
				Permission:    "feature.preview",
				Preview:       action.PreviewRequired,
				AuditLevel:    action.AuditMetadata,
				Channels:      []action.Channel{action.ChannelHTTP},
			},
			NewHandler: func(context.Context, Resolver) (action.Handler, error) { return handler, nil },
		},
	)
	if err := host.Register(registration, testRuntimeServicesRegistration(testRuntimeServices{})); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	assembly, err := host.Assemble()
	if err != nil {
		t.Fatal(err)
	}
	runtime := assembly.Runtime()
	executionScope := executionscope.Must("tenant", "one")
	previewResult := make(chan error, 1)
	go func() {
		_, previewErr := runtime.Preview(context.Background(), action.Request{
			RequestID: "in-flight",
			Actor:     identity.Actor{ID: "user", Type: "user", Scope: executionScope},
			Channel:   action.ChannelHTTP,
			ActionID:  "feature.preview",
			Scope:     executionScope,
			Input:     []byte(`{}`),
		})
		previewResult <- previewErr
	}()
	select {
	case <-handler.entered:
	case <-time.After(time.Second):
		t.Fatal("Action did not enter Handler.Plan")
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := <-previewResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("Preview() error = %v", err)
	}
	if !cleanupRan.Load() {
		t.Fatal("cleanup did not run after Action drain")
	}
}

func TestShutdownDeadlineLeavesBackgroundDrainSafeAndObservable(t *testing.T) {
	handler := &delayedPlanHandler{entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	var cleanupRan atomic.Bool
	host := NewHost()
	registration := Register(
		validManifest("feature", "feature", nil, []string{"feature"}),
		func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error {
				cleanupRan.Store(true)
				return nil
			})
		},
		ActionBinding{
			Descriptor: action.Descriptor{
				ID: "feature.delayed", Version: "0.1.0", Title: "Delayed preview",
				InputSchema: action.Object(nil).JSON(), PreviewSchema: action.Object(nil).JSON(), OutputSchema: action.Object(nil).JSON(),
				Permission: "feature.delayed", Preview: action.PreviewRequired, AuditLevel: action.AuditMetadata, Channels: []action.Channel{action.ChannelHTTP},
			},
			NewHandler: func(context.Context, Resolver) (action.Handler, error) { return handler, nil },
		},
	)
	if err := host.Register(registration, testRuntimeServicesRegistration(testRuntimeServices{})); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	assembly, err := host.Assemble()
	if err != nil {
		t.Fatal(err)
	}
	runtime := assembly.Runtime()
	executionScope := executionscope.Must("tenant", "one")
	previewResult := make(chan error, 1)
	go func() {
		_, previewErr := runtime.Preview(context.Background(), action.Request{
			RequestID: "delayed", Actor: identity.Actor{ID: "user", Type: "user", Scope: executionScope},
			Channel: action.ChannelHTTP, ActionID: "feature.delayed", Scope: executionScope, Input: []byte(`{}`),
		})
		previewResult <- previewErr
	}()
	select {
	case <-handler.entered:
	case <-time.After(time.Second):
		t.Fatal("Action did not enter delayed Handler.Plan")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := host.Shutdown(shutdownCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded Shutdown() error = %v", err)
	}
	if host.State() != StateStopping || cleanupRan.Load() {
		t.Fatalf("state=%s cleanup=%v before drain", host.State(), cleanupRan.Load())
	}
	close(handler.release)
	if err := <-previewResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("Preview() error = %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("final Shutdown() error = %v", err)
	}
	if host.State() != StateStopped || !cleanupRan.Load() {
		t.Fatalf("state=%s cleanup=%v after drain", host.State(), cleanupRan.Load())
	}
}

func testActionDescriptor(id string) action.Descriptor {
	return action.Descriptor{
		ID:           id,
		Version:      "0.1.0",
		Title:        "Test Action",
		InputSchema:  action.Object(nil).JSON(),
		OutputSchema: action.Object(nil).JSON(),
		Permission:   id,
		Preview:      action.PreviewNone,
		AuditLevel:   action.AuditMetadata,
		Channels:     []action.Channel{action.ChannelHTTP},
	}
}
