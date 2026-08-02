package appkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/internal/testsupport"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/scope"
)

var errCleanup = errors.New("cleanup failed")

func TestStartValidatesBeforeModuleSideEffects(t *testing.T) {
	tests := []struct {
		name       string
		metadata   Metadata
		options    Options
		definition func(*atomic.Int32, *atomic.Int32) Definition
		want       string
	}{
		{
			name:     "missing metadata id",
			metadata: Metadata{Name: "Example", Version: "1.0.0"},
			want:     "metadata id",
		},
		{
			name:     "invalid metadata name",
			metadata: Metadata{ID: "example", Name: " Example", Version: "1.0.0"},
			want:     "metadata name",
		},
		{
			name:     "invalid metadata version",
			metadata: Metadata{ID: "example", Name: "Example", Version: "v1.0.0"},
			want:     "metadata version",
		},
		{
			name:     "negative cleanup timeout",
			metadata: validMetadata(),
			options:  Options{Shutdown: module.ShutdownPolicy{CallbackTimeout: -time.Second}},
			want:     "cleanup callback timeout",
		},
		{
			name:     "negative plan ttl",
			metadata: validMetadata(),
			options:  Options{Runtime: RuntimeOptions{PlanTTL: -time.Second}},
			want:     "plan TTL",
		},
		{
			name:     "negative audit timeout",
			metadata: validMetadata(),
			options:  Options{Runtime: RuntimeOptions{AuditTimeout: -time.Second}},
			want:     "audit timeout",
		},
		{
			name:     "negative rollback timeout",
			metadata: validMetadata(),
			options:  Options{RollbackTimeout: -time.Second},
			want:     "rollback timeout",
		},
		{
			name:     "no modules",
			metadata: validMetadata(),
			definition: func(_, _ *atomic.Int32) Definition {
				return Definition{Metadata: validMetadata()}
			},
			want: "at least one Module",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var starts atomic.Int32
			var factories atomic.Int32
			definition := Definition{
				Metadata: test.metadata,
				Modules:  []module.Registration{sideEffectRegistration(&starts, &factories)},
			}
			if test.definition != nil {
				definition = test.definition(&starts, &factories)
			}
			application, err := Start(context.Background(), definition, test.options)
			if err == nil || application != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Start() = %#v, %v; want error containing %q", application, err, test.want)
			}
			if starts.Load() != 0 || factories.Load() != 0 {
				t.Fatalf("preflight invoked start=%d factory=%d", starts.Load(), factories.Load())
			}
		})
	}
}

func TestStartRejectsNilContextBeforeModuleSideEffects(t *testing.T) {
	var starts atomic.Int32
	var factories atomic.Int32
	definition := Definition{
		Metadata: validMetadata(),
		Modules:  []module.Registration{sideEffectRegistration(&starts, &factories)},
	}
	application, err := Start(nil, definition, Options{})
	if application != nil || !errors.Is(err, ErrContextRequired) {
		t.Fatalf("Start(nil) = %#v, %v", application, err)
	}
	if starts.Load() != 0 || factories.Load() != 0 {
		t.Fatalf("nil context invoked start=%d factory=%d", starts.Load(), factories.Load())
	}
}

func TestStartCancellationDuringAssemblyRollsBackBeforePublishingReady(t *testing.T) {
	for _, cancelAt := range []int32{1, 2, 3} {
		t.Run(fmt.Sprintf("check-%d", cancelAt), func(t *testing.T) {
			var cleanup atomic.Int32
			ctx := &assemblyCancellationContext{cancelAt: cancelAt}
			application, err := Start(ctx, Definition{
				Metadata: validMetadata(),
				Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
					cleanup: func(context.Context) error {
						cleanup.Add(1)
						return nil
					},
				})},
			}, Options{})
			if application != nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("Start() = %#v, %v", application, err)
			}
			if cleanup.Load() != 1 {
				t.Fatalf("cleanup calls = %d, want 1", cleanup.Load())
			}
		})
	}
}

func TestCatalogPreflightDoesNotInvokeStartOrHandlerFactory(t *testing.T) {
	var starts atomic.Int32
	var factories atomic.Int32
	registration := sideEffectRegistration(&starts, &factories)
	registration.Definition.Manifest.Requires = []module.Capability{"missing-capability"}
	registration.Definition.Actions[0].NewHandler = func(context.Context, module.Resolver) (action.Handler, error) {
		factories.Add(1)
		panic("preflight invoked handler factory")
	}

	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules:  []module.Registration{registration},
	}, Options{})
	if err == nil || application != nil || !strings.Contains(err.Error(), "missing-capability") {
		t.Fatalf("Start() = %#v, %v", application, err)
	}
	if starts.Load() != 0 || factories.Load() != 0 {
		t.Fatalf("catalog preflight invoked start=%d factory=%d", starts.Load(), factories.Load())
	}
}

func TestMissingGovernanceServiceCleansStartedModulesExactlyOnce(t *testing.T) {
	tests := []struct {
		missing string
		keyName string
	}{
		{missing: "authorizer", keyName: module.Authorizer().Name()},
		{missing: "audit", keyName: module.AuditHook().Name()},
		{missing: "persistence", keyName: runtimecontrol.ServiceName},
	}

	for _, test := range tests {
		t.Run(test.missing, func(t *testing.T) {
			var cleanup atomic.Int32
			application, err := Start(context.Background(), Definition{
				Metadata: validMetadata(),
				Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
					missing:    test.missing,
					withAction: true,
					cleanup: func(context.Context) error {
						cleanup.Add(1)
						return nil
					},
				})},
			}, Options{})
			if err == nil || application != nil || !strings.Contains(err.Error(), test.keyName) {
				t.Fatalf("Start() = %#v, %v; want missing %s", application, err, test.keyName)
			}
			if cleanup.Load() != 1 {
				t.Fatalf("cleanup calls = %d, want 1", cleanup.Load())
			}
		})
	}
}

func TestPostStartFailureJoinsCleanupError(t *testing.T) {
	var cleanup atomic.Int32
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
			missing:    "authorizer",
			withAction: true,
			cleanup: func(context.Context) error {
				cleanup.Add(1)
				return errCleanup
			},
		})},
	}, Options{})
	if application != nil || err == nil || !errors.Is(err, errCleanup) || !strings.Contains(err.Error(), module.Authorizer().Name()) {
		t.Fatalf("Start() = %#v, %v", application, err)
	}
	if cleanup.Load() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanup.Load())
	}
}

func TestPostStartFailureUsesIndependentBoundedShutdownContext(t *testing.T) {
	var cleanup atomic.Int32
	cleanupFinished := make(chan struct{})
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
			missing:    "authorizer",
			withAction: true,
			cleanup: func(ctx context.Context) error {
				cleanup.Add(1)
				<-ctx.Done()
				close(cleanupFinished)
				return ctx.Err()
			},
		})},
	}, Options{
		Shutdown:        module.ShutdownPolicy{CallbackTimeout: 50 * time.Millisecond},
		RollbackTimeout: 5 * time.Millisecond,
	})
	if application != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start() = %#v, %v; want bounded rollback deadline", application, err)
	}
	select {
	case <-cleanupFinished:
	case <-time.After(time.Second):
		t.Fatal("background Host cleanup did not finish")
	}
	if cleanup.Load() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanup.Load())
	}
}

func TestStartContainsLifecyclePanicsAndRejectsTypedNilServices(t *testing.T) {
	t.Run("start panic", func(t *testing.T) {
		var cleanup atomic.Int32
		registration := runtimeRegistration(runtimeRegistrationOptions{
			cleanup: func(context.Context) error {
				cleanup.Add(1)
				return nil
			},
			startPanic: true,
		})
		application, err := Start(context.Background(), Definition{Metadata: validMetadata(), Modules: []module.Registration{registration}}, Options{})
		if application != nil || !errors.Is(err, module.ErrCallbackPanic) || cleanup.Load() != 1 {
			t.Fatalf("Start() = %#v, %v; cleanup=%d", application, err, cleanup.Load())
		}
	})

	t.Run("handler factory panic", func(t *testing.T) {
		var cleanup atomic.Int32
		var factories atomic.Int32
		registration := runtimeRegistration(runtimeRegistrationOptions{
			cleanup: func(context.Context) error {
				cleanup.Add(1)
				return nil
			},
			factoryCalls: &factories,
			factoryPanic: true,
			withAction:   true,
		})
		application, err := Start(context.Background(), Definition{Metadata: validMetadata(), Modules: []module.Registration{registration}}, Options{})
		if application != nil || !errors.Is(err, module.ErrCallbackPanic) || cleanup.Load() != 1 || factories.Load() != 1 {
			t.Fatalf("Start() = %#v, %v; cleanup=%d factories=%d", application, err, cleanup.Load(), factories.Load())
		}
	})

	t.Run("typed nil service", func(t *testing.T) {
		var cleanup atomic.Int32
		manifest := validManifest("typed-nil")
		manifest.Provides = []module.Capability{module.CapabilityAuthorization}
		registration := module.Registration{
			Definition: module.Definition{Manifest: manifest},
			Start: func(_ context.Context, install module.Scope) error {
				if err := module.OnStop(install, func(context.Context) error {
					cleanup.Add(1)
					return nil
				}); err != nil {
					return err
				}
				var service *nilAuthorizer
				return module.Provide(install, module.Authorizer(), authz.Authorizer(service))
			},
		}
		application, err := Start(context.Background(), Definition{Metadata: validMetadata(), Modules: []module.Registration{registration}}, Options{})
		if application != nil || err == nil || !strings.Contains(err.Error(), "start callback failed") || cleanup.Load() != 1 {
			t.Fatalf("Start() = %#v, %v; cleanup=%d", application, err, cleanup.Load())
		}
	})
}

func TestApplicationFacadeIsDefensiveAndRuntimeIsRevokedOnShutdown(t *testing.T) {
	var cleanup atomic.Int32
	var factories atomic.Int32
	services := newTestIdentity()
	registration := runtimeRegistration(runtimeRegistrationOptions{
		cleanup: func(context.Context) error {
			cleanup.Add(1)
			return nil
		},
		withAction:   true,
		withIdentity: true,
		identity:     services,
		factoryCalls: &factories,
	})
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules:  []module.Registration{registration},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if factories.Load() != 1 {
		t.Fatalf("handler factory calls = %d, want 1", factories.Load())
	}

	metadata := application.Metadata()
	metadata.Name = "mutated"
	if application.Metadata().Name != validMetadata().Name {
		t.Fatalf("metadata mutation escaped: %#v", application.Metadata())
	}

	catalog := application.Catalog()
	if len(catalog) != 1 || catalog[0].ContractHash == "" {
		t.Fatalf("Catalog() = %#v", catalog)
	}
	originalSchema := string(catalog[0].Descriptor.InputSchema)
	originalChannel := catalog[0].Descriptor.Channels[0]
	catalog[0].Descriptor.InputSchema[0] = 'x'
	catalog[0].Descriptor.Channels[0] = "mutated"
	catalog[0].ModuleID = "mutated"
	reloaded := application.Catalog()
	if string(reloaded[0].Descriptor.InputSchema) != originalSchema || reloaded[0].Descriptor.Channels[0] != originalChannel || reloaded[0].ModuleID == "mutated" {
		t.Fatalf("catalog mutation escaped: %#v", reloaded)
	}

	if got, accessErr := application.Identities(); accessErr != nil || got == nil || got == identity.Resolver(services) {
		t.Fatalf("Identities() = %#v, %v", got, accessErr)
	}
	if got, accessErr := application.Sessions(); accessErr != nil || got == nil || got == identity.Authenticator(services) {
		t.Fatalf("Sessions() = %#v, %v", got, accessErr)
	}
	if got, accessErr := application.Tokens(); accessErr != nil || got == nil || got == identity.TokenAuthenticator(services) {
		t.Fatalf("Tokens() = %#v, %v", got, accessErr)
	}

	executionScope := scope.Must("tenant", "acme")
	result, err := application.Runtime().Execute(context.Background(), action.Request{
		Actor:    identity.Actor{ID: "actor", Type: "user", Scope: executionScope},
		Channel:  action.ChannelCLI,
		ActionID: "example.echo",
		Scope:    executionScope,
		Input:    json.RawMessage(`{"message":"hello"}`),
	})
	if err != nil || string(result.Data) != `{"echo":"ok"}` {
		t.Fatalf("Execute() = %#v, %v", result, err)
	}

	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	if cleanup.Load() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanup.Load())
	}
	_, err = application.Runtime().Execute(context.Background(), action.Request{
		Actor: identity.Actor{ID: "actor", Type: "user", Scope: executionScope}, Channel: action.ChannelCLI,
		ActionID: "example.echo", Scope: executionScope, Input: json.RawMessage(`{"message":"hello"}`),
	})
	if !action.IsCode(err, action.CodeUnavailable) {
		t.Fatalf("Execute() after Shutdown error = %v", err)
	}
}

func TestRuntimeOptionsAreForwardedAfterPreflight(t *testing.T) {
	fixed := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	registration := runtimeRegistration(runtimeRegistrationOptions{withAction: true})
	descriptor := registration.Definition.Actions[0].Descriptor
	descriptor.Preview = action.PreviewOptional
	descriptor.PreviewSchema = action.Object(map[string]action.Field{
		"message": action.RequiredField(action.String()),
	}).JSON()
	registration.Definition.Actions[0] = module.ActionBinding{
		Descriptor: descriptor,
		NewHandler: func(context.Context, module.Resolver) (action.Handler, error) {
			return previewEchoHandler{}, nil
		},
	}
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules:  []module.Registration{registration},
	}, Options{Runtime: RuntimeOptions{
		Clock:   func() time.Time { return fixed },
		PlanTTL: 3 * time.Minute,
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(context.Background())

	executionScope := scope.Must("tenant", "acme")
	preview, err := application.Runtime().Preview(context.Background(), action.Request{
		Actor:    identity.Actor{ID: "actor", Type: "user", Scope: executionScope},
		Channel:  action.ChannelCLI,
		ActionID: "example.echo",
		Scope:    executionScope,
		Input:    json.RawMessage(`{"message":"hello"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.ExpiresAt.Equal(fixed.Add(3 * time.Minute)) {
		t.Fatalf("Preview ExpiresAt = %s, want %s", preview.ExpiresAt, fixed.Add(3*time.Minute))
	}
}

func TestApplicationConcurrentShutdownIsExactlyOnce(t *testing.T) {
	var cleanup atomic.Int32
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
			cleanup: func(context.Context) error {
				cleanup.Add(1)
				return nil
			},
		})},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}

	const callers = 32
	var wait sync.WaitGroup
	errorsByCaller := make(chan error, callers)
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsByCaller <- application.Shutdown(context.Background())
		}()
	}
	wait.Wait()
	close(errorsByCaller)
	for shutdownErr := range errorsByCaller {
		if shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	}
	if cleanup.Load() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanup.Load())
	}
}

func TestApplicationReadyTurnsFalseWhenShutdownBegins(t *testing.T) {
	cleanupEntered := make(chan struct{})
	releaseCleanup := make(chan struct{})
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
			cleanup: func(context.Context) error {
				close(cleanupEntered)
				<-releaseCleanup
				return nil
			},
		})},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !application.Ready() {
		t.Fatal("application is not ready after Start")
	}
	if err := application.Shutdown(nil); !errors.Is(err, ErrContextRequired) || !application.Ready() {
		t.Fatalf("Shutdown(nil) = %v, ready=%t", err, application.Ready())
	}

	stopReaders := make(chan struct{})
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
					_ = application.Ready()
				}
			}
		}()
	}
	shutdownResult := make(chan error, 1)
	go func() { shutdownResult <- application.Shutdown(context.Background()) }()
	select {
	case <-cleanupEntered:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not enter cleanup")
	}
	if application.Ready() {
		t.Fatal("application remained ready after shutdown began")
	}
	close(releaseCleanup)
	if err := <-shutdownResult; err != nil {
		t.Fatal(err)
	}
	close(stopReaders)
	readers.Wait()
	if application.Ready() {
		t.Fatal("application became ready after shutdown")
	}
	var nilApplication *Application
	if nilApplication.Ready() {
		t.Fatal("nil Application reports ready")
	}
}

func TestShutdownTimeoutCanBeWaitedAgain(t *testing.T) {
	var cleanup atomic.Int32
	release := make(chan struct{})
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
			cleanup: func(context.Context) error {
				cleanup.Add(1)
				<-release
				return nil
			},
		})},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if shutdownErr := application.Shutdown(ctx); !errors.Is(shutdownErr, context.DeadlineExceeded) {
		t.Fatalf("bounded Shutdown() error = %v", shutdownErr)
	}
	close(release)
	if shutdownErr := application.Shutdown(context.Background()); shutdownErr != nil {
		t.Fatalf("second Shutdown() error = %v", shutdownErr)
	}
	if cleanup.Load() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanup.Load())
	}
}

func TestOptionalIdentityCapabilitiesAndNilReceiversFailClearly(t *testing.T) {
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules:  []module.Registration{runtimeRegistration(runtimeRegistrationOptions{})},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(context.Background())

	if value, accessErr := application.Identities(); value != nil || !errors.Is(accessErr, ErrIdentitiesUnavailable) {
		t.Fatalf("Identities() = %#v, %v", value, accessErr)
	}
	if value, accessErr := application.Sessions(); value != nil || !errors.Is(accessErr, ErrSessionsUnavailable) {
		t.Fatalf("Sessions() = %#v, %v", value, accessErr)
	}
	if value, accessErr := application.Tokens(); value != nil || !errors.Is(accessErr, ErrTokensUnavailable) {
		t.Fatalf("Tokens() = %#v, %v", value, accessErr)
	}

	var unavailable *Application
	if unavailable.Metadata() != (Metadata{}) || unavailable.Catalog() != nil || unavailable.Runtime() != nil {
		t.Fatal("nil Application accessors did not return zero values")
	}
	if value, accessErr := unavailable.Identities(); value != nil || !errors.Is(accessErr, ErrApplicationUnavailable) {
		t.Fatalf("nil Identities() = %#v, %v", value, accessErr)
	}
	if value, accessErr := unavailable.Sessions(); value != nil || !errors.Is(accessErr, ErrApplicationUnavailable) {
		t.Fatalf("nil Sessions() = %#v, %v", value, accessErr)
	}
	if value, accessErr := unavailable.Tokens(); value != nil || !errors.Is(accessErr, ErrApplicationUnavailable) {
		t.Fatalf("nil Tokens() = %#v, %v", value, accessErr)
	}
	if shutdownErr := unavailable.Shutdown(context.Background()); !errors.Is(shutdownErr, ErrApplicationUnavailable) {
		t.Fatalf("nil Shutdown() error = %v", shutdownErr)
	}
	if shutdownErr := unavailable.Shutdown(nil); !errors.Is(shutdownErr, ErrContextRequired) {
		t.Fatalf("nil Shutdown(nil) error = %v", shutdownErr)
	}
	if shutdownErr := application.Shutdown(nil); !errors.Is(shutdownErr, ErrContextRequired) {
		t.Fatalf("Shutdown(nil) error = %v", shutdownErr)
	}
}

func validMetadata() Metadata {
	return Metadata{ID: "example-app", Name: "Example App", Version: "1.2.3"}
}

func validManifest(id string) module.Manifest {
	return module.Manifest{
		SchemaVersion: module.SchemaVersion,
		ID:            id,
		Version:       "1.0.0",
		Type:          module.ModuleTypeAdapter,
	}
}

func sideEffectRegistration(starts, factories *atomic.Int32) module.Registration {
	manifest := validManifest("side-effects")
	return module.Registration{
		Definition: module.Definition{
			Manifest: manifest,
			Actions: []module.ActionBinding{{
				Descriptor: echoDescriptor(),
				NewHandler: func(context.Context, module.Resolver) (action.Handler, error) {
					factories.Add(1)
					return echoHandler{}, nil
				},
			}},
		},
		Start: func(context.Context, module.Scope) error {
			starts.Add(1)
			return nil
		},
	}
}

type runtimeRegistrationOptions struct {
	missing      string
	cleanup      module.Cleanup
	withAction   bool
	withIdentity bool
	identity     completeIdentity
	startPanic   bool
	factoryPanic bool
	factoryCalls *atomic.Int32
}

type completeIdentity interface {
	identity.Authenticator
	identity.TokenAuthenticator
}

type assemblyCancellationContext struct {
	calls    atomic.Int32
	cancelAt int32
}

func (*assemblyCancellationContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*assemblyCancellationContext) Done() <-chan struct{}       { return nil }
func (*assemblyCancellationContext) Value(any) any               { return nil }
func (ctx *assemblyCancellationContext) Err() error {
	if ctx.calls.Add(1) >= ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func runtimeRegistration(options runtimeRegistrationOptions) module.Registration {
	manifest := validManifest("runtime-services")
	manifest.Provides = []module.Capability{
		module.CapabilityAudit,
		module.CapabilityAuthorization,
		module.CapabilityDatabase,
	}
	if options.withIdentity {
		manifest.Provides = append(manifest.Provides, "identity")
	}
	definition := module.Definition{Manifest: manifest}
	if options.withAction {
		definition.Actions = []module.ActionBinding{{
			Descriptor: echoDescriptor(),
			NewHandler: func(context.Context, module.Resolver) (action.Handler, error) {
				if options.factoryCalls != nil {
					options.factoryCalls.Add(1)
				}
				if options.factoryPanic {
					panic("factory panic")
				}
				return echoHandler{}, nil
			},
		}}
	}
	return module.Registration{
		Definition: definition,
		Start: func(_ context.Context, install module.Scope) error {
			if options.cleanup != nil {
				if err := module.OnStop(install, options.cleanup); err != nil {
					return err
				}
			}
			if options.startPanic {
				panic("start panic")
			}
			if options.missing != "authorizer" {
				if err := module.Provide(install, module.Authorizer(), authz.Authorizer(allowAuthorizer{})); err != nil {
					return err
				}
			}
			if options.missing != "audit" {
				if err := module.Provide(install, module.AuditHook(), audit.Hook(testsupport.DiscardAudit{})); err != nil {
					return err
				}
			}
			if options.missing != "persistence" {
				if err := moduleassembly.ProvideActionPersistence(
					install,
					testsupport.NewMemoryPlanStore(),
					testsupport.NewMemoryIdempotencyStore(),
					testsupport.DirectTransactions{},
				); err != nil {
					return err
				}
			}
			if options.withIdentity {
				service := options.identity
				if service == nil {
					service = newTestIdentity()
				}
				if err := module.Provide(install, module.IdentityResolver(), identity.Resolver(service)); err != nil {
					return err
				}
				if err := module.Provide(install, module.SessionAuthenticator(), identity.Authenticator(service)); err != nil {
					return err
				}
				if err := module.Provide(install, module.TokenAuthenticator(), identity.TokenAuthenticator(service)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func echoDescriptor() action.Descriptor {
	return action.Descriptor{
		ID:      "example.echo",
		Version: "1.0.0",
		Title:   "Echo",
		InputSchema: action.Object(map[string]action.Field{
			"message": action.RequiredField(action.String()),
		}).JSON(),
		OutputSchema: action.Object(map[string]action.Field{
			"echo": action.RequiredField(action.String()),
		}).JSON(),
		Permission: "example.echo",
		Preview:    action.PreviewNone,
		AuditLevel: action.AuditMetadata,
		Channels:   []action.Channel{action.ChannelCLI, action.ChannelHTTP, action.ChannelMCP},
	}
}

type echoHandler struct{}

func (echoHandler) Plan(_ context.Context, request action.Request) (action.PlanData, error) {
	return action.PlanData{Payload: request.Input}, nil
}

func (echoHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	return action.Result{Data: json.RawMessage(`{"echo":"ok"}`)}, nil
}

type previewEchoHandler struct{}

func (previewEchoHandler) Plan(_ context.Context, request action.Request) (action.PlanData, error) {
	return action.PlanData{
		Payload: request.Input,
		Summary: json.RawMessage(`{"message":"planned"}`),
	}, nil
}

func (previewEchoHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	return action.Result{Data: json.RawMessage(`{"echo":"ok"}`)}, nil
}

type allowAuthorizer struct{}

func (allowAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Allowed: true, Fingerprint: "allow"}, nil
}

type nilAuthorizer struct{}

func (*nilAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Allowed: true}, nil
}

type testIdentity struct {
	actor identity.Actor
}

func newTestIdentity() *testIdentity {
	return &testIdentity{actor: identity.Actor{ID: "actor", Type: "user", Scope: scope.Must("tenant", "acme")}}
}

func (service *testIdentity) ResolveByID(context.Context, string) (identity.Actor, error) {
	return service.actor, nil
}

func (service *testIdentity) Login(context.Context, string, string) (identity.Session, error) {
	return identity.Session{Token: "session", CSRFToken: "csrf", Actor: service.actor}, nil
}

func (*testIdentity) Logout(context.Context, string) error { return nil }

func (service *testIdentity) Session(context.Context, string) (identity.Session, error) {
	return identity.Session{Token: "session", CSRFToken: "csrf", Actor: service.actor}, nil
}

func (service *testIdentity) AuthenticateToken(context.Context, string) (identity.Actor, error) {
	return service.actor, nil
}
