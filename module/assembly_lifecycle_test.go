package module

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/identity"
)

func TestHostAssembleIsIdempotentAcrossConcurrentCallers(t *testing.T) {
	service := newHostBlockingIdentity(false)
	host := NewHost()
	if err := host.Register(
		testRuntimeServicesRegistration(testRuntimeServices{}),
		hostIdentityRegistration(service, nil),
		Register(
			validManifest("action-feature", ModuleTypeFeature, nil, []string{"action-feature"}),
			nil,
			ActionBinding{
				Descriptor: testActionDescriptor("action-feature.run"),
				NewHandler: func(context.Context, Resolver) (action.Handler, error) {
					return inertActionHandler{}, nil
				},
			},
		),
	); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	const callers = 32
	type result struct {
		assembly Assembly
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			assembly, err := host.Assemble()
			results <- result{assembly: assembly, err: err}
		}()
	}
	ready.Wait()
	close(start)

	first := <-results
	if first.err != nil {
		t.Fatal(first.err)
	}
	if first.assembly.Runtime() == nil || first.assembly.Identities() == nil ||
		first.assembly.Sessions() == nil || first.assembly.Tokens() == nil {
		t.Fatalf("incomplete Assembly = %#v", first.assembly)
	}
	for index := 1; index < callers; index++ {
		next := <-results
		if next.err != nil {
			t.Fatalf("caller %d Assemble() error = %v", index, next.err)
		}
		if next.assembly.Runtime() != first.assembly.Runtime() ||
			next.assembly.Identities() != first.assembly.Identities() ||
			next.assembly.Sessions() != first.assembly.Sessions() ||
			next.assembly.Tokens() != first.assembly.Tokens() {
			t.Fatalf("caller %d received a distinct facade set", index)
		}
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHostAssembleCachesFirstFailure(t *testing.T) {
	host := NewHost()
	if err := host.Register(Register(
		validManifest("incomplete", ModuleTypeFeature, nil, []string{"incomplete"}),
		nil,
		ActionBinding{
			Descriptor: testActionDescriptor("incomplete.run"),
			NewHandler: func(context.Context, Resolver) (action.Handler, error) {
				return inertActionHandler{}, nil
			},
		},
	)); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	_, firstErr := host.Assemble()
	if firstErr == nil {
		t.Fatal("first Assemble() unexpectedly succeeded")
	}
	_, secondErr := host.Assemble()
	if secondErr != firstErr {
		t.Fatalf("second Assemble() error = %v, want cached first error %v", secondErr, firstErr)
	}
	if !host.assemblyAttempted || host.assemblyErr != firstErr {
		t.Fatal("Host did not retain the first assembly attempt")
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHostShutdownDrainsDirectAssemblyIdentityFacadeBeforeCleanup(t *testing.T) {
	service := newHostBlockingIdentity(false)
	var cleanupCalls atomic.Int32
	var cleanupRaced atomic.Bool
	cleanupDone := make(chan struct{})
	host := NewHost()
	if err := host.Register(
		testRuntimeServicesRegistration(testRuntimeServices{}),
		hostIdentityRegistration(service, func(context.Context) error {
			if service.active.Load() {
				cleanupRaced.Store(true)
			}
			cleanupCalls.Add(1)
			close(cleanupDone)
			return nil
		}),
	); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	assembly, err := host.Assemble()
	if err != nil {
		t.Fatal(err)
	}
	tokens := assembly.Tokens()

	callResult := make(chan error, 1)
	go func() {
		_, callErr := tokens.AuthenticateToken(context.Background(), "token")
		callResult <- callErr
	}()
	<-service.entered

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := host.Shutdown(shutdownCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded Shutdown() = %v", err)
	}
	if cleanupCalls.Load() != 0 {
		t.Fatalf("cleanup raced active identity facade: calls=%d", cleanupCalls.Load())
	}

	close(service.release)
	if callErr := <-callResult; callErr != nil {
		t.Fatalf("released identity call = %v", callErr)
	}
	select {
	case <-cleanupDone:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not continue after the identity facade drained")
	}
	select {
	case <-host.shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("Host did not publish shutdown completion")
	}
	if cleanupCalls.Load() != 1 || cleanupRaced.Load() || host.State() != StateStopped {
		t.Fatalf("cleanup=%d raced=%t state=%s", cleanupCalls.Load(), cleanupRaced.Load(), host.State())
	}
	if _, err := tokens.AuthenticateToken(context.Background(), "token"); !errors.Is(err, ErrApplicationUnavailable) {
		t.Fatalf("retained identity facade = %v", err)
	}
	if service.calls.Load() != 1 {
		t.Fatalf("retained identity facade reached service: calls=%d", service.calls.Load())
	}
}

func TestHostPreCanceledShutdownStillCompletesSharedCleanup(t *testing.T) {
	var cleanupCalls atomic.Int32
	cleanupDone := make(chan struct{})
	host := NewHost()
	if err := host.Register(
		testRuntimeServicesRegistration(testRuntimeServices{}),
		Register(validManifest("cleanup-owner", "feature", nil, []string{"cleanup"}), func(_ context.Context, scope Scope) error {
			return OnStop(scope, func(context.Context) error {
				cleanupCalls.Add(1)
				close(cleanupDone)
				return nil
			})
		}),
	); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Assemble(); err != nil {
		t.Fatal(err)
	}

	shutdownCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := host.Shutdown(shutdownCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown() = %v", err)
	}
	select {
	case <-cleanupDone:
	case <-time.After(time.Second):
		t.Fatal("pre-canceled caller prevented cleanup")
	}
	select {
	case <-host.shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("Host did not publish shutdown completion")
	}
	if cleanupCalls.Load() != 1 || host.State() != StateStopped {
		t.Fatalf("cleanup=%d state=%s", cleanupCalls.Load(), host.State())
	}
}

func TestPanickingContextCannotPolluteHostFacadeDrain(t *testing.T) {
	service := newHostBlockingIdentity(false)
	var cleanupCalls atomic.Int32
	host := NewHost()
	if err := host.Register(
		testRuntimeServicesRegistration(testRuntimeServices{}),
		hostIdentityRegistration(service, func(context.Context) error {
			cleanupCalls.Add(1)
			return nil
		}),
	); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	assembly, err := host.Assemble()
	if err != nil {
		t.Fatal(err)
	}

	const panicValue = "broken host facade context"
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = assembly.Tokens().AuthenticateToken(
			hostPanicDoneContext{Context: context.Background(), value: panicValue},
			"token",
		)
	}()
	if recovered != panicValue {
		t.Fatalf("recovered panic = %#v", recovered)
	}
	if service.calls.Load() != 0 {
		t.Fatalf("panicking Context reached identity service: calls=%d", service.calls.Load())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := host.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() after Context panic = %v", err)
	}
	if cleanupCalls.Load() != 1 {
		t.Fatalf("cleanup calls = %d", cleanupCalls.Load())
	}
}

func hostIdentityRegistration(service *hostBlockingIdentity, cleanup Cleanup) Registration {
	return Register(
		validManifest("identity-service", "adapter", nil, []string{"identity", "identity.bearers", "identity.passwords", "identity.sessions"}),
		func(_ context.Context, scope Scope) error {
			if cleanup != nil {
				if err := OnStop(scope, cleanup); err != nil {
					return err
				}
			}
			if err := Provide(scope, IdentityResolver(), identity.Resolver(service)); err != nil {
				return err
			}
			if err := Provide(scope, PasswordAuthenticator(), identity.PasswordAuthenticator(service)); err != nil {
				return err
			}
			if err := Provide(scope, SessionManager(), identity.SessionManager(service)); err != nil {
				return err
			}
			return Provide(scope, TokenAuthenticator(), identity.TokenAuthenticator(service))
		},
	)
}

type hostBlockingIdentity struct {
	respectContext bool
	entered        chan struct{}
	release        chan struct{}
	enteredOnce    sync.Once
	calls          atomic.Int32
	active         atomic.Bool
}

func newHostBlockingIdentity(respectContext bool) *hostBlockingIdentity {
	return &hostBlockingIdentity{
		respectContext: respectContext,
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
}

func (service *hostBlockingIdentity) wait(ctx context.Context) error {
	service.calls.Add(1)
	service.active.Store(true)
	defer service.active.Store(false)
	service.enteredOnce.Do(func() { close(service.entered) })
	if service.respectContext {
		<-ctx.Done()
		return ctx.Err()
	}
	<-service.release
	return nil
}

func (service *hostBlockingIdentity) ResolveByID(ctx context.Context, _ string) (identity.Actor, error) {
	return identity.Actor{}, service.wait(ctx)
}

func (service *hostBlockingIdentity) AuthenticatePassword(ctx context.Context, _, _ string) (identity.Authentication, error) {
	return identity.Authentication{}, service.wait(ctx)
}

func (service *hostBlockingIdentity) CreateSession(ctx context.Context, _ identity.Authentication) (identity.Session, error) {
	return identity.Session{}, service.wait(ctx)
}

func (service *hostBlockingIdentity) RevokeSession(ctx context.Context, _ string) error {
	return service.wait(ctx)
}

func (service *hostBlockingIdentity) ResolveSession(ctx context.Context, _ string) (identity.Session, error) {
	return identity.Session{}, service.wait(ctx)
}

func (service *hostBlockingIdentity) AuthenticateToken(ctx context.Context, _ string) (identity.Actor, error) {
	return identity.Actor{}, service.wait(ctx)
}

type hostPanicDoneContext struct {
	context.Context
	value string
}

func (ctx hostPanicDoneContext) Done() <-chan struct{} {
	panic(ctx.value)
}
