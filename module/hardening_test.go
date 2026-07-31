package module

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/internal/testsupport"
	"github.com/iiwish/modary/scope"
)

func TestStartPanicRollsBackResourcesAndFailsDeterministically(t *testing.T) {
	key := MustKey[string]("panic-start.value", "feature")
	var cleaned atomic.Int32
	host := NewHost()
	registration := Register(validManifest("feature", "feature", nil, []string{"feature"}), func(_ context.Context, scope Scope) error {
		if err := Provide(scope, key, "value"); err != nil {
			return err
		}
		if err := OnStop(scope, func(context.Context) error {
			cleaned.Add(1)
			return nil
		}); err != nil {
			return err
		}
		panic("start exploded")
	})
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	err := host.Start(context.Background())
	if !errors.Is(err, ErrCallbackPanic) || !strings.Contains(err.Error(), "start callback panicked") || strings.Contains(err.Error(), "exploded") {
		t.Fatalf("Start() error = %v", err)
	}
	if host.State() != StateFailed || cleaned.Load() != 1 {
		t.Fatalf("state=%s cleanup calls=%d", host.State(), cleaned.Load())
	}
	if len(host.services) != 0 {
		t.Fatalf("services remain after panic: %#v", host.services)
	}
}

func TestHandlerFactoryPanicRevokesBindingsAndRollsBack(t *testing.T) {
	var cleaned atomic.Int32
	host := NewHost()
	registration := Register(
		validManifest("feature", "feature", nil, []string{"feature"}),
		func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error {
				cleaned.Add(1)
				return nil
			})
		},
		ActionBinding{
			Descriptor: testActionDescriptor("feature.first"),
			NewHandler: func(context.Context, Resolver) (action.Handler, error) {
				return inertActionHandler{}, nil
			},
		},
		ActionBinding{
			Descriptor: testActionDescriptor("feature.panic"),
			NewHandler: func(context.Context, Resolver) (action.Handler, error) {
				panic("factory exploded")
			},
		},
	)
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	err := host.Start(context.Background())
	if !errors.Is(err, ErrCallbackPanic) || !strings.Contains(err.Error(), "handler factory callback panicked") || strings.Contains(err.Error(), "exploded") {
		t.Fatalf("Start() error = %v", err)
	}
	if host.State() != StateFailed || cleaned.Load() != 1 {
		t.Fatalf("state=%s cleanup calls=%d", host.State(), cleaned.Load())
	}
	if _, ok := host.registry.Resolve("feature.first"); ok {
		t.Fatal("binding remains after factory panic")
	}
}

func TestCleanupPanicIsJoinedAndRemainingCallbacksContinue(t *testing.T) {
	var order []string
	errFirst := errors.New("first failed")
	errLast := errors.New("last failed")
	host := NewHost()
	registration := Register(validManifest("feature", "feature", nil, []string{"feature"}), func(_ context.Context, scope Scope) error {
		callbacks := []Cleanup{
			func(context.Context) error { order = append(order, "first"); return errFirst },
			func(context.Context) error { order = append(order, "panic"); panic("cleanup exploded") },
			func(context.Context) error { order = append(order, "last"); return errLast },
		}
		for _, callback := range callbacks {
			if err := OnStop(scope, callback); err != nil {
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
	err := host.Shutdown(context.Background())
	if !errors.Is(err, errFirst) || !errors.Is(err, errLast) || !errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if want := []string{"last", "panic", "first"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("cleanup order = %v, want %v", order, want)
	}
	if host.State() != StateFailed {
		t.Fatalf("state = %s", host.State())
	}
	if second := host.Shutdown(context.Background()); !errors.Is(second, ErrCallbackPanic) || len(order) != 3 {
		t.Fatalf("second Shutdown() error=%v order=%v", second, order)
	}
}

func TestModuleLifecycleContainsNilPanics(t *testing.T) {
	t.Run("start rollback", func(t *testing.T) {
		key := MustKey[string]("nil-panic-start.value", "feature")
		var cleaned atomic.Int32
		host := NewHost()
		registration := Register(validManifest("nil-panic-start", "feature", nil, []string{"feature"}), func(_ context.Context, scope Scope) error {
			if err := Provide(scope, key, "value"); err != nil {
				return err
			}
			if err := OnStop(scope, func(context.Context) error {
				cleaned.Add(1)
				return nil
			}); err != nil {
				return err
			}
			panic(nil)
		})
		if err := host.Register(registration); err != nil {
			t.Fatal(err)
		}
		err := host.Start(context.Background())
		if !errors.Is(err, ErrCallbackPanic) || !strings.Contains(err.Error(), "start callback panicked") {
			t.Fatalf("Start() nil panic error = %v", err)
		}
		if host.State() != StateFailed || cleaned.Load() != 1 {
			t.Fatalf("state=%s cleanup calls=%d", host.State(), cleaned.Load())
		}
		if len(host.services) != 0 {
			t.Fatalf("services remain after panic(nil): %#v", host.services)
		}
	})

	t.Run("cleanup continues", func(t *testing.T) {
		var order []string
		host := NewHost()
		registration := Register(validManifest("nil-panic-cleanup", "feature", nil, []string{"feature"}), func(_ context.Context, scope Scope) error {
			callbacks := []Cleanup{
				func(context.Context) error { order = append(order, "first"); return nil },
				func(context.Context) error { order = append(order, "panic"); panic(nil) },
				func(context.Context) error { order = append(order, "last"); return nil },
			}
			for _, callback := range callbacks {
				if err := OnStop(scope, callback); err != nil {
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
		err := host.Shutdown(context.Background())
		if !errors.Is(err, ErrCallbackPanic) || !strings.Contains(err.Error(), "cleanup callback panicked") {
			t.Fatalf("Shutdown() nil panic error = %v", err)
		}
		if want := []string{"last", "panic", "first"}; !reflect.DeepEqual(order, want) {
			t.Fatalf("cleanup order = %v, want %v", order, want)
		}
		if host.State() != StateFailed {
			t.Fatalf("state = %s, want failed", host.State())
		}
	})
}

func TestHostShutdownPolicyValidationAndIndependentCallbackBudgets(t *testing.T) {
	if _, err := NewHostWithOptions(HostOptions{Shutdown: ShutdownPolicy{CallbackTimeout: -time.Second}}); err == nil {
		t.Fatal("negative callback timeout was accepted")
	}
	if _, err := NewHostWithOptions(HostOptions{Runtime: action.RuntimePolicy{PlanTTL: -time.Second}}); err == nil {
		t.Fatal("negative Runtime plan TTL was accepted")
	}
	if _, err := NewHostWithOptions(HostOptions{Runtime: action.RuntimePolicy{AuditTimeout: -time.Second}}); err == nil {
		t.Fatal("negative Runtime audit timeout was accepted")
	}
	if got := NewHost().shutdownPolicy.CallbackTimeout; got != DefaultCleanupCallbackTimeout {
		t.Fatalf("default callback timeout = %s", got)
	}
	host, err := NewHostWithOptions(HostOptions{Shutdown: ShutdownPolicy{CallbackTimeout: 25 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	var entriesMu sync.Mutex
	var entryErrors []error
	entries := make(chan struct{}, 2)
	registration := Register(validManifest("feature", "feature", nil, []string{"feature"}), func(_ context.Context, scope Scope) error {
		for index := 0; index < 2; index++ {
			if err := OnStop(scope, func(ctx context.Context) error {
				entriesMu.Lock()
				entryErrors = append(entryErrors, ctx.Err())
				entriesMu.Unlock()
				entries <- struct{}{}
				<-ctx.Done()
				return ctx.Err()
			}); err != nil {
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
	if err := host.Shutdown(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v", err)
	}
	for range 2 {
		select {
		case <-entries:
		case <-time.After(time.Second):
			t.Fatal("cleanup callback did not enter")
		}
	}
	entriesMu.Lock()
	defer entriesMu.Unlock()
	if len(entryErrors) != 2 || entryErrors[0] != nil || entryErrors[1] != nil {
		t.Fatalf("callback entry contexts = %v; each callback needs a fresh budget", entryErrors)
	}
}

func TestShutdownContinuesInBackgroundAfterCallerDeadline(t *testing.T) {
	host, err := NewHostWithOptions(HostOptions{Shutdown: ShutdownPolicy{CallbackTimeout: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	registration := Register(validManifest("feature", "feature", nil, []string{"feature"}), func(_ context.Context, scope Scope) error {
		return OnStop(scope, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	})
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err = host.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v", err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("background cleanup did not start")
	}
	if host.State() != StateStopping {
		t.Fatalf("state = %s, want stopping", host.State())
	}
	releaseOnce.Do(func() { close(release) })
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAssembleAndShutdownAreRaceSafeAndLinearizable(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		host := NewHost()
		if err := host.Register(
			Register(validManifest("feature", "feature", nil, []string{"feature"}), nil),
			testRuntimeServicesRegistration(testRuntimeServices{}),
		); err != nil {
			t.Fatal(err)
		}
		if err := host.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		ready := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		var runtime action.Runtime
		var runtimeErr, shutdownErr error
		go func() {
			defer wait.Done()
			<-ready
			var assembly Assembly
			assembly, runtimeErr = host.Assemble()
			if runtimeErr == nil {
				runtime = assembly.Runtime()
			}
		}()
		go func() {
			defer wait.Done()
			<-ready
			shutdownErr = host.Shutdown(context.Background())
		}()
		close(ready)
		wait.Wait()
		if shutdownErr != nil {
			t.Fatalf("iteration %d Shutdown() error = %v", iteration, shutdownErr)
		}
		if runtimeErr != nil && !errors.Is(runtimeErr, ErrInvalidState) {
			t.Fatalf("iteration %d Assemble() error = %v", iteration, runtimeErr)
		}
		if runtimeErr == nil && runtime == nil {
			t.Fatalf("iteration %d Assemble() returned nil Runtime without error", iteration)
		}
	}
}

func TestAssembleRejectsCorruptOptionalServiceInsteadOfTreatingItAsAbsent(t *testing.T) {
	host := NewHost()
	if err := host.Register(testRuntimeServicesRegistration(testRuntimeServices{})); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	canonical := IdentityResolver()
	forged := MustKey[string](canonical.Name(), canonical.Capability())
	host.mu.Lock()
	host.services[canonical.Name()] = serviceRecord{key: forged.spec, value: "corrupt", owner: "corrupt"}
	host.mu.Unlock()
	assembly, err := host.Assemble()
	if err == nil || assembly.Runtime() != nil || !strings.Contains(err.Error(), "resolve optional service "+canonical.Name()) {
		t.Fatalf("Assemble() = %#v, %v", assembly, err)
	}
	host.mu.Lock()
	delete(host.services, canonical.Name())
	host.mu.Unlock()
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestServiceKeyConstructionIsValidatedAndExplicit(t *testing.T) {
	valid := []struct {
		name       string
		capability Capability
	}{
		{name: "example.database", capability: "database"},
		{name: "example.read-model", capability: "storage/read-model"},
		{name: "example.deep.value", capability: "storage.read-model"},
	}
	for _, item := range valid {
		key, err := NewKey[string](item.name, item.capability)
		if err != nil {
			t.Fatalf("NewKey(%q, %q): %v", item.name, item.capability, err)
		}
		if key.Name() != item.name || key.Capability() != item.capability {
			t.Fatalf("key = %q/%q", key.Name(), key.Capability())
		}
	}
	invalid := []struct {
		name       string
		capability Capability
	}{
		{name: "unnamespaced", capability: "database"},
		{name: "Example.value", capability: "database"},
		{name: "example..value", capability: "database"},
		{name: "example.value", capability: "storage//read"},
		{name: "example.value", capability: "storage_legacy"},
		{name: " example.value", capability: "database"},
	}
	for _, item := range invalid {
		if key, err := NewKey[string](item.name, item.capability); err == nil || key.Name() != "" {
			t.Fatalf("NewKey(%q, %q) = %#v, %v", item.name, item.capability, key, err)
		}
	}
}

func TestManifestCapabilityGrammarMatchesServiceKeys(t *testing.T) {
	for _, capability := range []string{"storage//read", "storage_legacy", "storage.", "/storage"} {
		manifest := validManifest("feature", "feature", nil, []string{capability})
		if err := ValidateManifest(manifest); err == nil {
			t.Errorf("capability %q was accepted", capability)
		}
	}
}

func TestMustKeyMakesIntentionalLiteralFailureExplicit(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("MustKey did not panic for invalid literal")
		}
	}()
	_ = MustKey[string]("invalid", "database")
}

func TestTypedNilResolverAndScopeAreRejectedWithoutPanic(t *testing.T) {
	key := MustKey[string]("typed-nil.value", "feature")
	var nilScope *installScope
	var resolver Resolver = nilScope
	if _, err := Resolve(resolver, key); !errors.Is(err, ErrInvalidResolver) {
		t.Fatalf("Resolve() error = %v", err)
	}
	var scope Scope = nilScope
	if err := Provide(scope, key, "value"); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("Provide() error = %v", err)
	}
	if err := OnStop(scope, func(context.Context) error { return nil }); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("OnStop() error = %v", err)
	}
}

func TestFrameworkServiceNamesRejectForgedKeysWithoutStatePollution(t *testing.T) {
	reserved := []struct {
		name       string
		capability Capability
	}{
		{name: Database().Name(), capability: Database().Capability()},
		{name: IdentityResolver().Name(), capability: IdentityResolver().Capability()},
		{name: SessionAuthenticator().Name(), capability: SessionAuthenticator().Capability()},
		{name: TokenAuthenticator().Name(), capability: TokenAuthenticator().Capability()},
		{name: Authorizer().Name(), capability: Authorizer().Capability()},
		{name: AuditHook().Name(), capability: AuditHook().Capability()},
		{name: "modary.database-control", capability: "database"},
		{name: runtimecontrol.ServiceName, capability: "database"},
	}
	for _, item := range reserved {
		t.Run(item.name, func(t *testing.T) {
			forged := MustKey[string](item.name, item.capability)
			host := NewHost()
			registration := Register(validManifest("forged-service", "adapter", nil, []string{string(item.capability)}), func(_ context.Context, scope Scope) error {
				return Provide(scope, forged, "forged")
			})
			if err := host.Register(registration); err != nil {
				t.Fatal(err)
			}
			if err := host.Start(context.Background()); !errors.Is(err, ErrReservedServiceName) {
				t.Fatalf("Start() forged service error = %v", err)
			}
			if len(host.services) != 0 || host.database != nil || host.databaseOwner != "" {
				t.Fatalf("forged service polluted Host: services=%v database=%#v owner=%q", host.services, host.database, host.databaseOwner)
			}
		})
	}
}

func TestCanonicalDatabaseAccessNameRejectsSameTypedForgedKey(t *testing.T) {
	canonical := Database()
	forged := MustKey[database.Access](canonical.Name(), canonical.Capability())
	access := database.Access(inertDatabaseAccess{})
	host := NewHost()
	registration := Register(validManifest("forged-database", "adapter", nil, []string{"database"}), func(_ context.Context, scope Scope) error {
		return Provide(scope, forged, access)
	})
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); !errors.Is(err, ErrReservedServiceName) {
		t.Fatalf("Start() forged database service error = %v", err)
	}
	if len(host.services) != 0 || host.database != nil || host.databaseOwner != "" {
		t.Fatalf("forged database service polluted Host: services=%v database=%#v owner=%q", host.services, host.database, host.databaseOwner)
	}
}

func TestConsumerCannotInstallCanonicalDatabaseAccess(t *testing.T) {
	host := NewHost()
	registration := Register(validManifest("consumer-database", "adapter", nil, []string{"database"}), func(_ context.Context, scope Scope) error {
		return Provide(scope, Database(), database.Access(inertDatabaseAccess{}))
	})
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	err := host.Start(context.Background())
	if err == nil {
		t.Fatalf("Start() canonical public database error = %v", err)
	}
	if host.State() != StateFailed || len(host.services) != 0 || host.database != nil || host.databaseOwner != "" {
		t.Fatalf("consumer database state=%s services=%v database=%#v owner=%q", host.State(), host.services, host.database, host.databaseOwner)
	}
}

func TestInstallationViewsExposeNoPrivilegedReflectionMethods(t *testing.T) {
	assertHidden := func(value any) error {
		viewType := reflect.TypeOf(value)
		if viewType.NumMethod() != 0 {
			methods := make([]string, 0, viewType.NumMethod())
			for index := 0; index < viewType.NumMethod(); index++ {
				methods = append(methods, viewType.Method(index).Name)
			}
			return fmt.Errorf("%s exposes reflected methods %v", viewType, methods)
		}
		return nil
	}
	host := NewHost()
	registration := Register(
		validManifest("reflection-boundary", "feature", nil, []string{"feature"}),
		func(_ context.Context, scope Scope) error { return assertHidden(scope) },
		ActionBinding{
			Descriptor: testActionDescriptor("reflection.boundary"),
			NewHandler: func(_ context.Context, resolver Resolver) (action.Handler, error) {
				if err := assertHidden(resolver); err != nil {
					return nil, err
				}
				return inertActionHandler{}, nil
			},
		},
	)
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHostRejectsNilLifecycleContextsBeforeStateChanges(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		host := NewHost()
		if err := host.Start(nil); !errors.Is(err, ErrContextRequired) {
			t.Fatalf("Start(nil) error = %v", err)
		}
		if state := host.State(); state != StateNew {
			t.Fatalf("state = %s, want %s", state, StateNew)
		}
		if err := host.Start(context.Background()); err != nil {
			t.Fatalf("valid Start() after rejection: %v", err)
		}
		if err := host.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("shutdown", func(t *testing.T) {
		host := NewHost()
		if err := host.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := host.Shutdown(nil); !errors.Is(err, ErrContextRequired) {
			t.Fatalf("Shutdown(nil) error = %v", err)
		}
		if state := host.State(); state != StateRunning {
			t.Fatalf("state = %s, want %s", state, StateRunning)
		}
		if err := host.Shutdown(context.Background()); err != nil {
			t.Fatalf("valid Shutdown() after rejection: %v", err)
		}
		if state := host.State(); state != StateStopped {
			t.Fatalf("state = %s, want %s", state, StateStopped)
		}
	})
}

func TestHandlerFactoryReceivesShortLivedReadOnlyResolverAfterStartScopeExpires(t *testing.T) {
	key := MustKey[string]("factory-read.value", "feature")
	other := MustKey[string]("factory-read.other", "feature")
	var retained Scope
	var retainedResolver Resolver
	host := NewHost()
	registration := Register(
		validManifest("factory-read", "feature", nil, []string{"feature"}),
		func(_ context.Context, scope Scope) error {
			retained = scope
			return Provide(scope, key, "ready")
		},
		ActionBinding{
			Descriptor: testActionDescriptor("factory.read"),
			NewHandler: func(_ context.Context, resolver Resolver) (action.Handler, error) {
				retainedResolver = resolver
				if _, mutable := resolver.(Scope); mutable {
					return nil, errors.New("handler factory received mutable Scope")
				}
				if value, err := Resolve(resolver, key); err != nil || value != "ready" {
					return nil, fmt.Errorf("resolve factory service: %q, %w", value, err)
				}
				if _, err := Resolve(retained, key); !errors.Is(err, ErrInvalidScope) {
					return nil, fmt.Errorf("retained Start Scope Resolve error = %v, want ErrInvalidScope", err)
				}
				if err := Provide(retained, other, "forbidden"); !errors.Is(err, ErrInvalidScope) {
					return nil, fmt.Errorf("retained Start Scope Provide error = %v, want ErrInvalidScope", err)
				}
				return inertActionHandler{}, nil
			},
		},
	)
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(retainedResolver, key); !errors.Is(err, ErrInvalidResolver) || !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("Resolve with retained HandlerFactory Resolver error = %v, want ErrInvalidResolver with legacy ErrInvalidScope compatibility", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestModuleResolverCannotForgePrivateActionPersistenceKey(t *testing.T) {
	host := NewHost()
	forged := MustKey[runtimecontrol.Persistence](runtimecontrol.ServiceName, "database")
	provider := Register(validManifest("database-provider", "adapter", nil, []string{"database"}), func(_ context.Context, scope Scope) error {
		persistence, err := runtimecontrol.New(testsupport.NewMemoryPlanStore(), testsupport.NewMemoryIdempotencyStore(), testsupport.DirectTransactions{})
		if err != nil {
			return err
		}
		return Provide(scope, testActionPersistenceKey, persistence)
	})
	consumer := Register(
		validManifest("database-consumer", "feature", []string{"database"}, nil),
		nil,
		ActionBinding{Descriptor: testActionDescriptor("database.consume"), NewHandler: func(_ context.Context, resolver Resolver) (action.Handler, error) {
			if _, err := Resolve(resolver, forged); err == nil || !strings.Contains(err.Error(), "does not match the registered key") {
				return nil, fmt.Errorf("private persistence resolution error = %v", err)
			}
			return inertActionHandler{}, nil
		}},
	)
	if err := host.Register(provider, consumer); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNonCooperativeCleanupTimesOutAndRemainingCallbacksRunExactlyOnce(t *testing.T) {
	release := make(chan struct{})
	var calls [3]atomic.Int32
	var orderMu sync.Mutex
	order := make([]string, 0, 3)
	appendOrder := func(value string) {
		orderMu.Lock()
		order = append(order, value)
		orderMu.Unlock()
	}
	host, err := NewHostWithOptions(HostOptions{Shutdown: ShutdownPolicy{CallbackTimeout: 15 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	registration := Register(validManifest("cleanup-timeout", "feature", nil, []string{"feature"}), func(_ context.Context, scope Scope) error {
		callbacks := []Cleanup{
			func(context.Context) error { calls[0].Add(1); appendOrder("first"); return nil },
			func(context.Context) error { calls[1].Add(1); appendOrder("second"); return nil },
			func(context.Context) error {
				calls[2].Add(1)
				appendOrder("blocking")
				<-release
				return nil
			},
		}
		for _, callback := range callbacks {
			if err := OnStop(scope, callback); err != nil {
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
	shutdownErr := host.Shutdown(context.Background())
	if !errors.Is(shutdownErr, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v", shutdownErr)
	}
	if host.State() != StateFailed {
		t.Fatalf("state = %s, want failed", host.State())
	}
	orderMu.Lock()
	got := strings.Join(order, ",")
	orderMu.Unlock()
	if got != "blocking,second,first" {
		t.Fatalf("cleanup order = %q", got)
	}
	for index := range calls {
		if calls[index].Load() != 1 {
			t.Fatalf("cleanup %d calls = %d", index, calls[index].Load())
		}
	}
	if err := host.Shutdown(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	close(release)
}

func TestTimedOutConsumerCleanupMayOverlapProviderCleanup(t *testing.T) {
	consumerEntered := make(chan struct{})
	consumerRelease := make(chan struct{})
	consumerDone := make(chan struct{})
	providerEntered := make(chan struct{})
	host, err := NewHostWithOptions(HostOptions{Shutdown: ShutdownPolicy{CallbackTimeout: 20 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	provider := Register(
		validManifest("overlap-provider", "adapter", nil, []string{"overlap.service"}),
		func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error {
				close(providerEntered)
				return nil
			})
		},
	)
	consumer := Register(
		validManifest("overlap-consumer", "feature", []string{"overlap.service"}, nil),
		func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error {
				defer close(consumerDone)
				close(consumerEntered)
				<-consumerRelease
				return nil
			})
		},
	)
	if err := host.Register(provider, consumer); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	shutdownResult := make(chan error, 1)
	go func() { shutdownResult <- host.Shutdown(context.Background()) }()
	select {
	case <-consumerEntered:
	case <-time.After(time.Second):
		t.Fatal("consumer cleanup did not start")
	}
	select {
	case <-providerEntered:
		// The consumer has not been released: provider cleanup overlaps it.
	case <-time.After(time.Second):
		t.Fatal("provider cleanup did not start after consumer timeout")
	}
	if err := <-shutdownResult; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want DeadlineExceeded", err)
	}
	close(consumerRelease)
	select {
	case <-consumerDone:
	case <-time.After(time.Second):
		t.Fatal("timed-out consumer cleanup did not exit after release")
	}
}

func TestNonCooperativeStartEventuallyRollsBackAfterShutdownCancellation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var cleaned atomic.Int32
	host := NewHost()
	registration := Register(validManifest("blocking-start", "feature", nil, nil), func(_ context.Context, scope Scope) error {
		if err := OnStop(scope, func(context.Context) error {
			cleaned.Add(1)
			return nil
		}); err != nil {
			return err
		}
		close(entered)
		<-release
		return nil
	})
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	startResult := make(chan error, 1)
	go func() { startResult <- host.Start(context.Background()) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Start callback did not begin")
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := host.Shutdown(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want DeadlineExceeded", err)
	}
	close(release)
	if err := <-startResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want Canceled", err)
	}
	if state := host.State(); state != StateStopped {
		t.Fatalf("state = %s, want stopped", state)
	}
	if got := cleaned.Load(); got != 1 {
		t.Fatalf("cleanup calls = %d, want 1", got)
	}
}

func TestNonCooperativeHandlerFactoryEventuallyRollsBackAfterShutdownCancellation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var cleaned atomic.Int32
	host := NewHost()
	registration := Register(
		validManifest("blocking-factory", "feature", nil, nil),
		func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error {
				cleaned.Add(1)
				return nil
			})
		},
		ActionBinding{
			Descriptor: testActionDescriptor("blocking.factory"),
			NewHandler: func(context.Context, Resolver) (action.Handler, error) {
				close(entered)
				<-release
				return inertActionHandler{}, nil
			},
		},
	)
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	startResult := make(chan error, 1)
	go func() { startResult <- host.Start(context.Background()) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("HandlerFactory did not begin")
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := host.Shutdown(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want DeadlineExceeded", err)
	}
	close(release)
	if err := <-startResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want Canceled", err)
	}
	if state := host.State(); state != StateStopped {
		t.Fatalf("state = %s, want stopped", state)
	}
	if got := cleaned.Load(); got != 1 {
		t.Fatalf("cleanup calls = %d, want 1", got)
	}
	if _, ok := host.registry.Resolve("blocking.factory"); ok {
		t.Fatal("HandlerFactory binding survived canceled startup")
	}
}

func TestLifecyclePanicValueIsNeverFormattedOrLeaked(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		host := NewHost()
		if err := host.Register(Register(validManifest("panic-value-start", "feature", nil, nil), func(context.Context, Scope) error {
			panic(hostileCallbackPanic{})
		})); err != nil {
			t.Fatal(err)
		}
		err := host.Start(context.Background())
		assertContainedCallbackPanic(t, err, "start")
	})

	t.Run("cleanup", func(t *testing.T) {
		host := NewHost()
		registration := Register(validManifest("panic-value-cleanup", "feature", nil, nil), func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error { panic(hostileCallbackPanic{}) })
		})
		if err := host.Register(registration); err != nil {
			t.Fatal(err)
		}
		if err := host.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertContainedCallbackPanic(t, host.Shutdown(context.Background()), "cleanup")
	})
}

func TestLifecycleErrorsAreTypedNilSafe(t *testing.T) {
	var panicErr *CallbackPanicError
	if got := panicErr.Error(); got != "module callback panicked" || !errors.Is(panicErr, ErrCallbackPanic) {
		t.Fatalf("typed-nil CallbackPanicError = %q, unwrap=%v", got, errors.Unwrap(panicErr))
	}
	if got := (&CallbackPanicError{}).Error(); got != "module callback panicked" {
		t.Fatalf("zero CallbackPanicError = %q", got)
	}
	var stateErr *StateError
	if got := stateErr.Error(); got != "module host state is invalid" || !errors.Is(stateErr, ErrInvalidState) {
		t.Fatalf("typed-nil StateError = %q, unwrap=%v", got, errors.Unwrap(stateErr))
	}
	if got := (&StateError{}).Error(); got != "module host state is invalid" {
		t.Fatalf("zero StateError = %q", got)
	}
}

func TestLifecycleReturnedErrorsAreStableAndTypedNilFailsClosed(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		hostile := &hostileCallbackError{secret: "start-secret"}
		host := NewHost()
		if err := host.Register(Register(validManifest("hostile-start-error", "feature", nil, nil), func(context.Context, Scope) error {
			return hostile
		})); err != nil {
			t.Fatal(err)
		}
		err := host.Start(context.Background())
		assertStableDependencyError(t, err, hostile, "start callback failed")

		var typedNil *hostileCallbackError
		var cleaned atomic.Int32
		typedNilHost := NewHost()
		if err := typedNilHost.Register(Register(validManifest("typed-nil-start-error", "feature", nil, nil), func(_ context.Context, scope Scope) error {
			if err := OnStop(scope, func(context.Context) error { cleaned.Add(1); return nil }); err != nil {
				return err
			}
			return typedNil
		})); err != nil {
			t.Fatal(err)
		}
		startErr := typedNilHost.Start(context.Background())
		assertStableDependencyError(t, startErr, typedNil, "start callback failed")
		if typedNilHost.State() != StateFailed || cleaned.Load() != 1 {
			t.Fatalf("typed-nil Start state=%s cleanup calls=%d", typedNilHost.State(), cleaned.Load())
		}
	})

	t.Run("handler factory", func(t *testing.T) {
		hostile := &hostileCallbackError{secret: "factory-secret"}
		host := NewHost()
		registration := Register(validManifest("hostile-factory-error", "feature", nil, nil), nil, ActionBinding{
			Descriptor: testActionDescriptor("hostile.factory"),
			NewHandler: func(context.Context, Resolver) (action.Handler, error) { return nil, hostile },
		})
		if err := host.Register(registration); err != nil {
			t.Fatal(err)
		}
		assertStableDependencyError(t, host.Start(context.Background()), hostile, "handler factory callback failed")

		var typedNil *hostileCallbackError
		var cleaned atomic.Int32
		typedNilHost := NewHost()
		typedNilRegistration := Register(validManifest("typed-nil-factory-error", "feature", nil, nil), func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error { cleaned.Add(1); return nil })
		}, ActionBinding{
			Descriptor: testActionDescriptor("typed-nil.factory"),
			NewHandler: func(context.Context, Resolver) (action.Handler, error) {
				return inertActionHandler{}, typedNil
			},
		})
		if err := typedNilHost.Register(typedNilRegistration); err != nil {
			t.Fatal(err)
		}
		factoryErr := typedNilHost.Start(context.Background())
		assertStableDependencyError(t, factoryErr, typedNil, "handler factory callback failed")
		if typedNilHost.State() != StateFailed || cleaned.Load() != 1 {
			t.Fatalf("typed-nil HandlerFactory state=%s cleanup calls=%d", typedNilHost.State(), cleaned.Load())
		}
		if _, exists := typedNilHost.registry.Resolve("typed-nil.factory"); exists {
			t.Fatal("typed-nil HandlerFactory bound a Handler")
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		hostile := &hostileCallbackError{secret: "cleanup-secret"}
		host := NewHost()
		registration := Register(validManifest("hostile-cleanup-error", "feature", nil, nil), func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error { return hostile })
		})
		if err := host.Register(registration); err != nil {
			t.Fatal(err)
		}
		if err := host.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertStableDependencyError(t, host.Shutdown(context.Background()), hostile, "cleanup callback failed")

		var typedNil *hostileCallbackError
		laterFailure := errors.New("later cleanup failed")
		var cleanupOrder []string
		typedNilHost := NewHost()
		typedNilRegistration := Register(validManifest("typed-nil-cleanup-error", "feature", nil, nil), func(_ context.Context, scope Scope) error {
			if err := OnStop(scope, func(context.Context) error {
				cleanupOrder = append(cleanupOrder, "later")
				return laterFailure
			}); err != nil {
				return err
			}
			return OnStop(scope, func(context.Context) error {
				cleanupOrder = append(cleanupOrder, "typed-nil")
				return typedNil
			})
		})
		if err := typedNilHost.Register(typedNilRegistration); err != nil {
			t.Fatal(err)
		}
		if err := typedNilHost.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		shutdownErr := typedNilHost.Shutdown(context.Background())
		assertStableDependencyError(t, shutdownErr, typedNil, "cleanup callback failed")
		if !errors.Is(shutdownErr, laterFailure) {
			t.Fatal("later cleanup failure was not aggregated")
		}
		if want := []string{"typed-nil", "later"}; !reflect.DeepEqual(cleanupOrder, want) {
			t.Fatalf("cleanup order = %v, want %v", cleanupOrder, want)
		}
	})
}

func TestCanceledShutdownRevokesRuntimeBeforeReturning(t *testing.T) {
	release := make(chan struct{})
	host, err := NewHostWithOptions(HostOptions{Shutdown: ShutdownPolicy{CallbackTimeout: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	registration := Register(
		validManifest("revoke-before-return", "feature", nil, []string{"feature"}),
		func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error { <-release; return nil })
		},
		ActionBinding{Descriptor: testActionDescriptor("revoke.run"), NewHandler: func(context.Context, Resolver) (action.Handler, error) {
			return inertActionHandler{}, nil
		}},
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
	shutdownCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := host.Shutdown(shutdownCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown(canceled) error = %v", err)
	}
	executionScope := scope.Must("tenant", "revoke")
	_, executionErr := runtime.Execute(context.Background(), action.Request{
		Actor: identity.Actor{ID: "user", Type: "user", Scope: executionScope}, Channel: action.ChannelHTTP,
		ActionID: "revoke.run", Scope: executionScope, Input: []byte(`{}`),
	})
	if !action.IsCode(executionErr, action.CodeUnavailable) {
		t.Fatalf("Execute() after Shutdown returned = %v", executionErr)
	}
	close(release)
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type hostileCallbackPanic struct{}

type inertDatabaseAccess struct{ database.Executor }

func (hostileCallbackPanic) Error() string  { panic("callback-secret-error") }
func (hostileCallbackPanic) String() string { panic("callback-secret-string") }

type hostileCallbackError struct{ secret string }

func (*hostileCallbackError) Error() string { panic("hostile callback Error invoked") }
func (*hostileCallbackError) Unwrap() error { panic("hostile callback Unwrap invoked") }

func assertStableDependencyError(t *testing.T, err error, cause error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("callback returned no error")
	}
	if got := err.Error(); !strings.Contains(got, want) || strings.Contains(got, "secret") {
		t.Fatalf("stable callback error = %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("callback cause was not preserved")
	}
}

func assertContainedCallbackPanic(t *testing.T, err error, callback string) {
	t.Helper()
	if !errors.Is(err, ErrCallbackPanic) || !strings.Contains(err.Error(), callback+" callback panicked") {
		t.Fatalf("callback error = %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("callback panic leaked secret: %v", err)
	}
}
