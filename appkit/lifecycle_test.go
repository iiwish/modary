package appkit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/module"
)

func TestApplicationShutdownCancelsAndDrainsIdentityFacadesBeforeCleanup(t *testing.T) {
	service := newBlockingIdentity(true)
	var cleanupCalls atomic.Int32
	var cleanupRaced atomic.Bool
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
			withIdentity: true,
			identity:     service,
			cleanup: func(context.Context) error {
				if service.active.Load() {
					cleanupRaced.Store(true)
				}
				cleanupCalls.Add(1)
				return nil
			},
		})},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := application.Tokens()
	if err != nil {
		t.Fatal(err)
	}

	callResult := make(chan error, 1)
	go func() {
		_, callErr := tokens.AuthenticateToken(context.Background(), "token")
		callResult <- callErr
	}()
	<-service.entered
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if callErr := <-callResult; !errors.Is(callErr, context.Canceled) {
		t.Fatalf("identity call error = %v", callErr)
	}
	if cleanupCalls.Load() != 1 || cleanupRaced.Load() {
		t.Fatalf("cleanup calls=%d raced=%t", cleanupCalls.Load(), cleanupRaced.Load())
	}
	if service.calls.Load() != 1 {
		t.Fatalf("identity calls = %d", service.calls.Load())
	}
	if _, err := tokens.AuthenticateToken(context.Background(), "token"); !errors.Is(err, ErrApplicationUnavailable) {
		t.Fatalf("retained facade after shutdown = %v", err)
	}
	if service.calls.Load() != 1 {
		t.Fatalf("retained facade reached cleaned service: calls=%d", service.calls.Load())
	}
}

func TestApplicationShutdownContinuesCleanupAfterCallerStopsWaiting(t *testing.T) {
	service := newBlockingIdentity(false)
	var cleanupCalls atomic.Int32
	cleanupDone := make(chan struct{})
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
			withIdentity: true,
			identity:     service,
			cleanup: func(context.Context) error {
				cleanupCalls.Add(1)
				close(cleanupDone)
				return nil
			},
		})},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := application.Tokens()
	if err != nil {
		t.Fatal(err)
	}
	callResult := make(chan error, 1)
	go func() {
		_, callErr := tokens.AuthenticateToken(context.Background(), "token")
		callResult <- callErr
	}()
	<-service.entered

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := application.Shutdown(shutdownCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded Shutdown() = %v", err)
	}
	if cleanupCalls.Load() != 0 {
		t.Fatalf("cleanup raced active identity call: calls=%d", cleanupCalls.Load())
	}
	if application.Ready() {
		t.Fatal("application remained ready after shutdown began")
	}
	if _, err := application.Tokens(); !errors.Is(err, ErrApplicationUnavailable) {
		t.Fatalf("Tokens() after shutdown began = %v", err)
	}
	if _, err := tokens.AuthenticateToken(context.Background(), "token"); !errors.Is(err, ErrApplicationUnavailable) {
		t.Fatalf("retained facade during shutdown = %v", err)
	}
	if service.calls.Load() != 1 {
		t.Fatalf("rejected calls reached identity service: calls=%d", service.calls.Load())
	}

	close(service.release)
	if err := <-callResult; err != nil {
		t.Fatalf("released identity call = %v", err)
	}
	select {
	case <-cleanupDone:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not continue after the active lease drained")
	}
	if cleanupCalls.Load() != 1 {
		t.Fatalf("cleanup calls = %d", cleanupCalls.Load())
	}
}

func TestApplicationShutdownWithPreCanceledContextStillStartsCleanup(t *testing.T) {
	var cleanupCalls atomic.Int32
	cleanupDone := make(chan struct{})
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
			cleanup: func(context.Context) error {
				cleanupCalls.Add(1)
				close(cleanupDone)
				return nil
			},
		})},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}

	shutdownCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := application.Shutdown(shutdownCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown() = %v", err)
	}
	select {
	case <-cleanupDone:
	case <-time.After(time.Second):
		t.Fatal("pre-canceled caller prevented cleanup")
	}
	if cleanupCalls.Load() != 1 {
		t.Fatalf("cleanup calls = %d", cleanupCalls.Load())
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("completed Shutdown() = %v", err)
	}
}

func TestConcurrentApplicationShutdownCallersWaitForOneResult(t *testing.T) {
	service := newBlockingIdentity(false)
	cleanupFailure := errors.New("cleanup failed")
	var cleanupCalls atomic.Int32
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
			withIdentity: true,
			identity:     service,
			cleanup: func(context.Context) error {
				cleanupCalls.Add(1)
				return cleanupFailure
			},
		})},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := application.Tokens()
	if err != nil {
		t.Fatal(err)
	}
	callResult := make(chan error, 1)
	go func() {
		_, callErr := tokens.AuthenticateToken(context.Background(), "token")
		callResult <- callErr
	}()
	<-service.entered

	var callersReady sync.WaitGroup
	callersReady.Add(2)
	start := make(chan struct{})
	shortResult := make(chan error, 1)
	longResult := make(chan error, 1)
	go func() {
		callersReady.Done()
		<-start
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		shortResult <- application.Shutdown(ctx)
	}()
	go func() {
		callersReady.Done()
		<-start
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		longResult <- application.Shutdown(ctx)
	}()
	callersReady.Wait()
	close(start)

	if shutdownErr := <-shortResult; !errors.Is(shutdownErr, context.DeadlineExceeded) {
		t.Fatalf("short Shutdown() = %v", shutdownErr)
	}
	if cleanupCalls.Load() != 0 {
		t.Fatalf("cleanup raced active identity call: calls=%d", cleanupCalls.Load())
	}
	close(service.release)
	if callErr := <-callResult; callErr != nil {
		t.Fatalf("released identity call = %v", callErr)
	}
	shutdownErr := <-longResult
	if !errors.Is(shutdownErr, cleanupFailure) {
		t.Fatalf("long Shutdown() = %v", shutdownErr)
	}
	if cleanupCalls.Load() != 1 {
		t.Fatalf("cleanup calls = %d", cleanupCalls.Load())
	}
	if cachedErr := application.Shutdown(context.Background()); cachedErr != shutdownErr {
		t.Fatalf("cached Shutdown() = %v, want identical result %v", cachedErr, shutdownErr)
	}
}

func TestPanickingCallerContextDoesNotRegisterApplicationLease(t *testing.T) {
	service := newBlockingIdentity(false)
	var cleanupCalls atomic.Int32
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
			withIdentity: true,
			identity:     service,
			cleanup: func(context.Context) error {
				cleanupCalls.Add(1)
				return nil
			},
		})},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := application.Tokens()
	if err != nil {
		t.Fatal(err)
	}

	const panicValue = "broken caller context"
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = tokens.AuthenticateToken(panicDoneContext{Context: context.Background(), value: panicValue}, "token")
	}()
	if recovered != panicValue {
		t.Fatalf("recovered panic = %#v", recovered)
	}
	if service.calls.Load() != 0 {
		t.Fatalf("panicking Context reached identity service: calls=%d", service.calls.Load())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := application.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() after Context panic = %v", err)
	}
	if cleanupCalls.Load() != 1 {
		t.Fatalf("cleanup calls = %d", cleanupCalls.Load())
	}
}

func TestEveryIdentityFacadeMethodHoldsApplicationLease(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Application) error
	}{
		{name: "resolver", invoke: func(application *Application) error {
			resolver, err := application.Identities()
			if err != nil {
				return err
			}
			_, err = resolver.ResolveByID(context.Background(), "actor")
			return err
		}},
		{name: "password", invoke: func(application *Application) error {
			authenticator, err := application.Passwords()
			if err != nil {
				return err
			}
			_, err = authenticator.AuthenticatePassword(context.Background(), "user", "password")
			return err
		}},
		{name: "create session", invoke: func(application *Application) error {
			authenticator, err := application.Sessions()
			if err != nil {
				return err
			}
			_, err = authenticator.CreateSession(context.Background(), identity.Authentication{Actor: lifecycleTestActor(), Method: identity.AuthenticationMethodOIDC})
			return err
		}},
		{name: "revoke session", invoke: func(application *Application) error {
			authenticator, err := application.Sessions()
			if err != nil {
				return err
			}
			return authenticator.RevokeSession(context.Background(), "session")
		}},
		{name: "session", invoke: func(application *Application) error {
			authenticator, err := application.Sessions()
			if err != nil {
				return err
			}
			_, err = authenticator.ResolveSession(context.Background(), "session")
			return err
		}},
		{name: "token", invoke: func(application *Application) error {
			authenticator, err := application.Tokens()
			if err != nil {
				return err
			}
			_, err = authenticator.AuthenticateToken(context.Background(), "token")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newBlockingIdentity(true)
			var cleanupCalls atomic.Int32
			var cleanupRaced atomic.Bool
			application, err := Start(context.Background(), Definition{
				Metadata: validMetadata(),
				Modules: []module.Registration{runtimeRegistration(runtimeRegistrationOptions{
					withIdentity: true,
					identity:     service,
					cleanup: func(context.Context) error {
						if service.active.Load() {
							cleanupRaced.Store(true)
						}
						cleanupCalls.Add(1)
						return nil
					},
				})},
			}, Options{})
			if err != nil {
				t.Fatal(err)
			}
			callResult := make(chan error, 1)
			go func() { callResult <- test.invoke(application) }()
			<-service.entered
			if err := application.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			if callErr := <-callResult; !errors.Is(callErr, context.Canceled) {
				t.Fatalf("facade call = %v", callErr)
			}
			if cleanupCalls.Load() != 1 || cleanupRaced.Load() || service.calls.Load() != 1 {
				t.Fatalf("cleanup=%d raced=%t calls=%d", cleanupCalls.Load(), cleanupRaced.Load(), service.calls.Load())
			}
		})
	}
}

type blockingIdentity struct {
	respectContext bool
	entered        chan struct{}
	release        chan struct{}
	enteredOnce    sync.Once
	calls          atomic.Int32
	active         atomic.Bool
}

type panicDoneContext struct {
	context.Context
	value string
}

func (ctx panicDoneContext) Done() <-chan struct{} {
	panic(ctx.value)
}

func newBlockingIdentity(respectContext bool) *blockingIdentity {
	return &blockingIdentity{
		respectContext: respectContext,
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
}

func (service *blockingIdentity) AuthenticateToken(ctx context.Context, _ string) (identity.Actor, error) {
	if err := service.wait(ctx); err != nil {
		return identity.Actor{}, err
	}
	return lifecycleTestActor(), nil
}

func (service *blockingIdentity) wait(ctx context.Context) error {
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

func (service *blockingIdentity) ResolveByID(ctx context.Context, _ string) (identity.Actor, error) {
	if err := service.wait(ctx); err != nil {
		return identity.Actor{}, err
	}
	return lifecycleTestActor(), nil
}

func (service *blockingIdentity) AuthenticatePassword(ctx context.Context, _, _ string) (identity.Authentication, error) {
	if err := service.wait(ctx); err != nil {
		return identity.Authentication{}, err
	}
	return identity.Authentication{Actor: lifecycleTestActor(), Method: identity.AuthenticationMethodPassword, CredentialVersion: "version"}, nil
}

func (service *blockingIdentity) CreateSession(ctx context.Context, _ identity.Authentication) (identity.Session, error) {
	if err := service.wait(ctx); err != nil {
		return identity.Session{}, err
	}
	return identity.Session{Actor: lifecycleTestActor()}, nil
}

func (service *blockingIdentity) RevokeSession(ctx context.Context, _ string) error {
	return service.wait(ctx)
}

func (service *blockingIdentity) ResolveSession(ctx context.Context, _ string) (identity.Session, error) {
	if err := service.wait(ctx); err != nil {
		return identity.Session{}, err
	}
	return identity.Session{Actor: lifecycleTestActor()}, nil
}

func lifecycleTestActor() identity.Actor {
	return identity.Actor{ID: "actor", Type: "user", DisplayName: "Actor"}
}
