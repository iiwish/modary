package module

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/database"
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

type assemblyStore struct {
	gate *assemblyGate
	next database.Store
}

func (store *assemblyStore) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	callCtx, release, err := store.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return store.next.ExecContext(callCtx, query, args...)
}

func (store *assemblyStore) QueryContext(ctx context.Context, query string, args ...any) (database.Rows, error) {
	callCtx, release, err := store.acquire(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := store.next.QueryContext(callCtx, query, args...)
	if err != nil {
		release()
		return nil, err
	}
	return &assemblyRows{next: rows, release: release}, nil
}

func (store *assemblyStore) QueryRowContext(ctx context.Context, query string, args ...any) database.Row {
	if ctx == nil {
		return assemblyErrorRow{err: ErrContextRequired}
	}
	return &assemblyLazyRow{store: store, ctx: ctx, query: query, args: append([]any(nil), args...)}
}

func (store *assemblyStore) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	callCtx, release, err := store.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return store.next.WithinTransaction(callCtx, operation)
}

func (store *assemblyStore) acquire(ctx context.Context) (context.Context, func(), error) {
	if store == nil || store.gate == nil || store.next == nil {
		return nil, nil, database.ErrAccessUnavailable
	}
	callCtx, release, err := store.gate.acquire(ctx)
	if err != nil {
		if errors.Is(err, ErrContextRequired) {
			return nil, nil, ErrContextRequired
		}
		return nil, nil, ErrApplicationUnavailable
	}
	return callCtx, release, nil
}

type assemblyErrorRow struct{ err error }

func (row assemblyErrorRow) Scan(...any) error { return row.err }

type assemblyLazyRow struct {
	store *assemblyStore
	ctx   context.Context
	query string
	args  []any
	once  sync.Once
	err   error
}

func (row *assemblyLazyRow) Scan(destinations ...any) error {
	if row == nil || row.store == nil {
		return database.ErrAccessUnavailable
	}
	row.once.Do(func() {
		callCtx, release, err := row.store.acquire(row.ctx)
		if err != nil {
			row.err = err
			return
		}
		defer release()
		row.err = row.store.next.QueryRowContext(callCtx, row.query, row.args...).Scan(destinations...)
		row.args = nil
	})
	return row.err
}

type assemblyRows struct {
	next    database.Rows
	release func()
	once    sync.Once
}

func (rows *assemblyRows) finish() { rows.once.Do(rows.release) }

func (rows *assemblyRows) Next() bool {
	if rows == nil || rows.next == nil {
		return false
	}
	next := rows.next.Next()
	if !next {
		rows.finish()
	}
	return next
}

func (rows *assemblyRows) Scan(destinations ...any) error {
	if rows == nil || rows.next == nil {
		return database.ErrAccessUnavailable
	}
	return rows.next.Scan(destinations...)
}

func (rows *assemblyRows) Err() error {
	if rows == nil || rows.next == nil {
		return database.ErrAccessUnavailable
	}
	return rows.next.Err()
}

func (rows *assemblyRows) Columns() ([]string, error) {
	if rows == nil || rows.next == nil {
		return nil, database.ErrAccessUnavailable
	}
	return rows.next.Columns()
}

func (rows *assemblyRows) Close() error {
	if rows == nil || rows.next == nil {
		return database.ErrAccessUnavailable
	}
	defer rows.finish()
	return rows.next.Close()
}

type assemblyAuthorizer struct {
	gate *assemblyGate
	next authz.Authorizer
}

func (authorizer *assemblyAuthorizer) Authorize(ctx context.Context, request authz.Request) (authz.Decision, error) {
	if authorizer == nil || authorizer.gate == nil || authorizer.next == nil {
		return authz.Decision{}, ErrApplicationUnavailable
	}
	callCtx, release, err := authorizer.gate.acquire(ctx)
	if err != nil {
		return authz.Decision{}, err
	}
	defer release()
	return authorizer.next.Authorize(callCtx, request)
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

type assemblyTaskInspector struct {
	gate *assemblyGate
	next task.Inspector
}

func (inspector *assemblyTaskInspector) List(ctx context.Context, options task.ListOptions) (task.Page, error) {
	if inspector == nil || inspector.gate == nil || inspector.next == nil {
		return task.Page{}, task.ErrUnavailable
	}
	callCtx, release, err := inspector.gate.acquire(ctx)
	if err != nil {
		return task.Page{}, task.ErrUnavailable
	}
	defer release()
	return inspector.next.List(callCtx, options)
}

type assemblyAuditReader struct {
	gate *assemblyGate
	next audit.Reader
}

func (reader *assemblyAuditReader) List(ctx context.Context, options audit.ListOptions) (audit.Page, error) {
	if reader == nil || reader.gate == nil || reader.next == nil {
		return audit.Page{}, ErrApplicationUnavailable
	}
	callCtx, release, err := reader.gate.acquire(ctx)
	if err != nil {
		return audit.Page{}, ErrApplicationUnavailable
	}
	defer release()
	return reader.next.List(callCtx, options)
}
