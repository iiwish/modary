package module

import (
	"context"
	"errors"
	"sync"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/task"
)

var (
	// ErrContextRequired reports a nil lifecycle or assembled-facade context.
	ErrContextRequired = errors.New("context is required")
	// ErrApplicationUnavailable reports an assembled facade whose Host has
	// begun shutdown or is otherwise unavailable.
	ErrApplicationUnavailable = errors.New("application is unavailable")
)

// assemblyGate is the single lease boundary around every Module-backed facade.
// Revocation rejects new calls, cancels active call contexts, and closes drained
// only after every facade method has returned.
type assemblyGate struct {
	mu       sync.Mutex
	open     bool
	active   int
	drained  chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	drainOne sync.Once
}

func newAssemblyGate() *assemblyGate {
	ctx, cancel := context.WithCancel(context.Background())
	return &assemblyGate{
		open:    true,
		drained: make(chan struct{}),
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (gate *assemblyGate) acquire(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		return nil, nil, ErrContextRequired
	}
	if gate == nil {
		return nil, nil, ErrApplicationUnavailable
	}

	// Caller Context implementations are process code. Derive before recording
	// shared state so a panic cannot leave a phantom lease behind.
	callCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(gate.ctx, cancel)

	gate.mu.Lock()
	if !gate.open {
		gate.mu.Unlock()
		stop()
		cancel()
		return nil, nil, ErrApplicationUnavailable
	}
	gate.active++
	gate.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			stop()
			cancel()
			gate.release()
		})
	}
	return callCtx, release, nil
}

func (gate *assemblyGate) release() {
	gate.mu.Lock()
	gate.active--
	if !gate.open && gate.active == 0 {
		gate.drainOne.Do(func() { close(gate.drained) })
	}
	gate.mu.Unlock()
}

func (gate *assemblyGate) revoke() <-chan struct{} {
	gate.mu.Lock()
	if gate.open {
		gate.open = false
		gate.cancel()
		if gate.active == 0 {
			gate.drainOne.Do(func() { close(gate.drained) })
		}
	}
	drained := gate.drained
	gate.mu.Unlock()
	return drained
}

type assemblyRuntime struct {
	gate *assemblyGate
	next action.Runtime
}

func (runtime *assemblyRuntime) Preview(ctx context.Context, request action.Request) (action.Preview, error) {
	callCtx, release, err := runtime.acquire(ctx)
	if err != nil {
		return action.Preview{}, err
	}
	defer release()
	return runtime.next.Preview(callCtx, request)
}

func (runtime *assemblyRuntime) Execute(ctx context.Context, request action.Request) (action.Result, error) {
	callCtx, release, err := runtime.acquire(ctx)
	if err != nil {
		return action.Result{}, err
	}
	defer release()
	return runtime.next.Execute(callCtx, request)
}

func (runtime *assemblyRuntime) CleanupExpiredPlans(ctx context.Context) (int64, error) {
	callCtx, release, err := runtime.acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer release()
	return runtime.next.CleanupExpiredPlans(callCtx)
}

func (runtime *assemblyRuntime) acquire(ctx context.Context) (context.Context, func(), error) {
	if runtime == nil || runtime.gate == nil || runtime.next == nil {
		return nil, nil, action.NewError(action.CodeUnavailable, "action runtime is unavailable")
	}
	callCtx, release, err := runtime.gate.acquire(ctx)
	if err == nil {
		return callCtx, release, nil
	}
	if errors.Is(err, ErrContextRequired) {
		return nil, nil, action.NewError(action.CodeValidationFailed, "context is required")
	}
	return nil, nil, action.NewError(action.CodeUnavailable, "action runtime is shutting down")
}

type assemblyResolver struct {
	gate *assemblyGate
	next identity.Resolver
}

func (resolver *assemblyResolver) ResolveByID(ctx context.Context, actorID string) (identity.Actor, error) {
	if resolver == nil || resolver.gate == nil || resolver.next == nil {
		return identity.Actor{}, ErrApplicationUnavailable
	}
	callCtx, release, err := resolver.gate.acquire(ctx)
	if err != nil {
		return identity.Actor{}, err
	}
	defer release()
	return resolver.next.ResolveByID(callCtx, actorID)
}

type assemblyAuthenticator struct {
	gate *assemblyGate
	next identity.Authenticator
}

func (authenticator *assemblyAuthenticator) ResolveByID(ctx context.Context, actorID string) (identity.Actor, error) {
	if authenticator == nil || authenticator.gate == nil || authenticator.next == nil {
		return identity.Actor{}, ErrApplicationUnavailable
	}
	callCtx, release, err := authenticator.gate.acquire(ctx)
	if err != nil {
		return identity.Actor{}, err
	}
	defer release()
	return authenticator.next.ResolveByID(callCtx, actorID)
}

func (authenticator *assemblyAuthenticator) Login(ctx context.Context, username, password string) (identity.Session, error) {
	if authenticator == nil || authenticator.gate == nil || authenticator.next == nil {
		return identity.Session{}, ErrApplicationUnavailable
	}
	callCtx, release, err := authenticator.gate.acquire(ctx)
	if err != nil {
		return identity.Session{}, err
	}
	defer release()
	return authenticator.next.Login(callCtx, username, password)
}

func (authenticator *assemblyAuthenticator) Logout(ctx context.Context, token string) error {
	if authenticator == nil || authenticator.gate == nil || authenticator.next == nil {
		return ErrApplicationUnavailable
	}
	callCtx, release, err := authenticator.gate.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return authenticator.next.Logout(callCtx, token)
}

func (authenticator *assemblyAuthenticator) Session(ctx context.Context, token string) (identity.Session, error) {
	if authenticator == nil || authenticator.gate == nil || authenticator.next == nil {
		return identity.Session{}, ErrApplicationUnavailable
	}
	callCtx, release, err := authenticator.gate.acquire(ctx)
	if err != nil {
		return identity.Session{}, err
	}
	defer release()
	return authenticator.next.Session(callCtx, token)
}

type assemblyTokenAuthenticator struct {
	gate *assemblyGate
	next identity.TokenAuthenticator
}

func (authenticator *assemblyTokenAuthenticator) AuthenticateToken(ctx context.Context, token string) (identity.Actor, error) {
	if authenticator == nil || authenticator.gate == nil || authenticator.next == nil {
		return identity.Actor{}, ErrApplicationUnavailable
	}
	callCtx, release, err := authenticator.gate.acquire(ctx)
	if err != nil {
		return identity.Actor{}, err
	}
	defer release()
	return authenticator.next.AuthenticateToken(callCtx, token)
}

type assemblyTaskService struct {
	gate *assemblyGate
	next task.Service
}

func (service *assemblyTaskService) Enqueue(ctx context.Context, request task.Request) (task.Receipt, error) {
	if service == nil || service.gate == nil || service.next == nil {
		return task.Receipt{}, task.ErrUnavailable
	}
	callCtx, release, err := service.gate.acquire(ctx)
	if err != nil {
		return task.Receipt{}, task.ErrUnavailable
	}
	defer release()
	return service.next.Enqueue(callCtx, request)
}

func (service *assemblyTaskService) NewRunner(handler task.Handler, options task.RunnerOptions) (task.Runner, error) {
	if service == nil || service.gate == nil || service.next == nil {
		return nil, task.ErrUnavailable
	}
	_, release, err := service.gate.acquire(context.Background())
	if err != nil {
		return nil, task.ErrUnavailable
	}
	defer release()
	return service.next.NewRunner(handler, options)
}
