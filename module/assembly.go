package module

import (
	"fmt"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/actionruntime"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/observe"
	"github.com/iiwish/modary/task"
)

// Assembly is the safe result of finalizing a running Host. It contains the
// optional governed Action Runtime and optional public component facades;
// mutable service storage, raw Handlers, and transaction control remain
// Host-private.
type Assembly struct {
	runtime       action.Runtime
	database      database.Store
	identities    identity.Resolver
	passwords     identity.PasswordAuthenticator
	browserAuth   identity.BrowserAuthenticator
	observability observe.Service
	sessions      identity.SessionManager
	tokens        identity.TokenAuthenticator
	tasks         task.Service
	taskInspector task.Inspector
	auditReader   audit.Reader
	authorizer    authz.Authorizer
}

// Runtime returns the governed Action execution surface, or nil when no Module
// declares an Action.
func (assembly Assembly) Runtime() action.Runtime { return assembly.runtime }

// Database returns the optional ordinary business-data Store installed by
// Modules. It owns callback-scoped transactions but exposes no raw connection.
func (assembly Assembly) Database() database.Store { return assembly.database }

// Identities returns the optional public actor resolver installed by Modules.
func (assembly Assembly) Identities() identity.Resolver { return assembly.identities }

// Passwords returns the optional public password verifier installed by Modules.
func (assembly Assembly) Passwords() identity.PasswordAuthenticator { return assembly.passwords }

// BrowserAuthentication returns the optional redirect-login service.
func (assembly Assembly) BrowserAuthentication() identity.BrowserAuthenticator {
	return assembly.browserAuth
}

// Observability returns the optional bounded telemetry facade.
func (assembly Assembly) Observability() observe.Service { return assembly.observability }

// Sessions returns the optional public session manager installed by Modules.
func (assembly Assembly) Sessions() identity.SessionManager { return assembly.sessions }

// Tokens returns the optional public bearer-token authenticator installed by Modules.
func (assembly Assembly) Tokens() identity.TokenAuthenticator { return assembly.tokens }

// Tasks returns the optional durable task service installed by Modules.
func (assembly Assembly) Tasks() task.Service { return assembly.tasks }

// TaskInspector returns optional read-only task metadata inspection.
func (assembly Assembly) TaskInspector() task.Inspector { return assembly.taskInspector }

// AuditReader returns optional scope-bound audit metadata inspection.
func (assembly Assembly) AuditReader() audit.Reader { return assembly.auditReader }

// Authorizer returns the optional policy evaluator installed by Modules.
func (assembly Assembly) Authorizer() authz.Authorizer { return assembly.authorizer }

// Assemble resolves public component facades inside a running Host. Governance
// services and Action persistence are required only when a Module declares an
// Action. The first call constructs the Host-owned facade set or caches its
// failure; every later call while running returns the same immutable Assembly
// or error. All returned facades share the Host-owned lifecycle. Callers can
// neither substitute policy or dependencies nor obtain underlying service
// values through the returned Assembly.
func (host *Host) Assemble() (Assembly, error) {
	if !host.available() {
		return Assembly{}, ErrHostUnavailable
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.state != StateRunning {
		return Assembly{}, &StateError{Operation: "assemble application", State: host.state}
	}
	if host.assemblyAttempted {
		if host.assembly == nil {
			return Assembly{}, host.assemblyErr
		}
		return *host.assembly, nil
	}
	assembly, err := host.assembleLocked()
	host.assemblyAttempted = true
	if err != nil {
		host.assemblyErr = err
		return Assembly{}, err
	}
	host.assembly = &assembly
	return assembly, nil
}

func (host *Host) assembleLocked() (Assembly, error) {
	assembly := Assembly{}
	if len(host.prepared) > 0 {
		runtime, err := host.assembleActionRuntimeLocked()
		if err != nil {
			return Assembly{}, err
		}
		assembly.runtime = runtime
	}

	identities, err := resolveOptionalHostServiceLocked(host, IdentityResolver())
	if err != nil {
		return Assembly{}, err
	}
	passwords, err := resolveOptionalHostServiceLocked(host, PasswordAuthenticator())
	if err != nil {
		return Assembly{}, err
	}
	browserAuth, err := resolveOptionalHostServiceLocked(host, BrowserAuthenticator())
	if err != nil {
		return Assembly{}, err
	}
	observability, err := resolveOptionalHostServiceLocked(host, Observability())
	if err != nil {
		return Assembly{}, err
	}
	sessions, err := resolveOptionalHostServiceLocked(host, SessionManager())
	if err != nil {
		return Assembly{}, err
	}
	tokens, err := resolveOptionalHostServiceLocked(host, TokenAuthenticator())
	if err != nil {
		return Assembly{}, err
	}
	tasks, err := resolveOptionalHostServiceLocked(host, Tasks())
	if err != nil {
		return Assembly{}, err
	}
	taskInspector, err := resolveOptionalHostServiceLocked(host, TaskInspector())
	if err != nil {
		return Assembly{}, err
	}
	auditReader, err := resolveOptionalHostServiceLocked(host, AuditReader())
	if err != nil {
		return Assembly{}, err
	}
	if identities != nil {
		assembly.identities = &assemblyResolver{gate: host.facades, next: identities}
	}
	if passwords != nil {
		assembly.passwords = &assemblyPasswordAuthenticator{gate: host.facades, next: passwords}
	}
	if browserAuth != nil {
		assembly.browserAuth = &assemblyBrowserAuthenticator{gate: host.facades, next: browserAuth}
	}
	if observability != nil {
		assembly.observability = &assemblyObservability{gate: host.facades, next: observability}
	}
	store, err := resolveOptionalHostServiceLocked(host, Database())
	if err != nil {
		return Assembly{}, err
	}
	authorizer, err := resolveOptionalHostServiceLocked(host, Authorizer())
	if err != nil {
		return Assembly{}, err
	}
	if store != nil {
		assembly.database = &assemblyStore{gate: host.facades, next: store, observer: assembly.observability}
	}
	if authorizer != nil {
		assembly.authorizer = &assemblyAuthorizer{gate: host.facades, next: authorizer}
	}
	if sessions != nil {
		assembly.sessions = &assemblySessionManager{gate: host.facades, next: sessions}
	}
	if tokens != nil {
		assembly.tokens = &assemblyTokenAuthenticator{gate: host.facades, next: tokens}
	}
	if tasks != nil {
		assembly.tasks = &assemblyTaskService{gate: host.facades, next: tasks, observer: assembly.observability}
	}
	if taskInspector != nil {
		assembly.taskInspector = &assemblyTaskInspector{gate: host.facades, next: taskInspector, observer: assembly.observability}
	}
	if auditReader != nil {
		assembly.auditReader = &assemblyAuditReader{gate: host.facades, next: auditReader}
	}
	return assembly, nil
}

func (host *Host) assembleActionRuntimeLocked() (action.Runtime, error) {
	authorizer, err := resolveRequiredHostServiceLocked(host, Authorizer())
	if err != nil {
		return nil, err
	}
	auditHook, err := resolveRequiredHostServiceLocked(host, AuditHook())
	if err != nil {
		return nil, err
	}
	persistence := host.persistence
	if isNilValue(persistence) {
		return nil, fmt.Errorf("resolve required service %s: complete Action persistence is not available", runtimecontrol.ServiceName)
	}
	plans := persistence.Plans()
	idempotency := persistence.Idempotency()
	transactions := persistence.Transactions()
	if isNilValue(plans) || isNilValue(idempotency) || isNilValue(transactions) {
		return nil, fmt.Errorf("resolve required service %s: complete Action persistence is not available", runtimecontrol.ServiceName)
	}
	runtime, err := actionruntime.New(host.registry, actionruntime.Options{
		Authorizer: authorizer, Audit: auditHook, Plans: plans,
		Idempotency: idempotency, Transactions: transactions,
		Clock: host.runtimePolicy.Clock, PlanTTL: host.runtimePolicy.PlanTTL,
		AuditTimeout: host.runtimePolicy.AuditTimeout, AuditFailure: host.runtimePolicy.AuditFailure,
	})
	if err != nil {
		return nil, fmt.Errorf("create Action Runtime: %w", err)
	}
	return &assemblyRuntime{gate: host.facades, next: runtime}, nil
}

func resolveRequiredHostServiceLocked[T any](host *Host, key Key[T]) (T, error) {
	value, err := resolveHostServiceLocked(host, key)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("resolve required service %s: %w", key.Name(), err)
	}
	return value, nil
}

func resolveOptionalHostServiceLocked[T any](host *Host, key Key[T]) (T, error) {
	var zero T
	if !key.spec.valid() {
		return zero, fmt.Errorf("invalid module service key")
	}
	if _, exists := host.services[key.spec.identity.name]; !exists {
		return zero, nil
	}
	value, err := resolveHostServiceLocked(host, key)
	if err != nil {
		return zero, fmt.Errorf("resolve optional service %s: %w", key.Name(), err)
	}
	return value, nil
}

// resolveHostServiceLocked resolves one service while the caller holds the
// Host read lock, preserving a single lifecycle/service snapshot.
func resolveHostServiceLocked[T any](host *Host, key Key[T]) (T, error) {
	var zero T
	if host == nil {
		return zero, fmt.Errorf("module host is required")
	}
	if !key.spec.valid() {
		return zero, fmt.Errorf("invalid module service key")
	}
	value, err := host.resolveLocked(key.spec)
	if err != nil {
		return zero, err
	}
	typed, ok := value.(T)
	if !ok || isNilValue(typed) {
		return zero, fmt.Errorf("service %s has type %T, expected non-nil %s", key.Name(), value, key.spec.identity.valueType)
	}
	return typed, nil
}

func resolveHostService[T any](host *Host, key Key[T]) (T, error) {
	var zero T
	if host == nil {
		return zero, fmt.Errorf("module host is required")
	}
	if !key.spec.valid() {
		return zero, fmt.Errorf("invalid module service key")
	}
	host.mu.RLock()
	defer host.mu.RUnlock()
	if host.state != StateRunning {
		return zero, &StateError{Operation: "resolve services", State: host.state}
	}
	return resolveHostServiceLocked(host, key)
}
