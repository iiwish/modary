package module

import (
	"fmt"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/actionruntime"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/task"
)

// Assembly is the safe result of finalizing a running Host. It contains only
// the governed Action Runtime and optional public identity facades; mutable
// service storage, raw Handlers, and transaction control remain Host-private.
type Assembly struct {
	runtime    action.Runtime
	identities identity.Resolver
	sessions   identity.Authenticator
	tokens     identity.TokenAuthenticator
	tasks      task.Service
}

// Runtime returns the governed Action execution surface.
func (assembly Assembly) Runtime() action.Runtime { return assembly.runtime }

// Identities returns the optional public actor resolver installed by Modules.
func (assembly Assembly) Identities() identity.Resolver { return assembly.identities }

// Sessions returns the optional public session authenticator installed by Modules.
func (assembly Assembly) Sessions() identity.Authenticator { return assembly.sessions }

// Tokens returns the optional public bearer-token authenticator installed by Modules.
func (assembly Assembly) Tokens() identity.TokenAuthenticator { return assembly.tokens }

// Tasks returns the optional durable task service installed by Modules.
func (assembly Assembly) Tasks() task.Service { return assembly.tasks }

// Assemble resolves canonical governance services inside a running Host. The
// first call constructs the Host-owned facade set or caches its failure; every
// later call while running returns the same immutable Assembly or error. All
// returned facades share the Host-owned lifecycle and governance domain.
// Callers can neither substitute policy or dependencies nor obtain underlying
// service values through the returned Assembly.
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
	authorizer, err := resolveRequiredHostServiceLocked(host, Authorizer())
	if err != nil {
		return Assembly{}, err
	}
	auditHook, err := resolveRequiredHostServiceLocked(host, AuditHook())
	if err != nil {
		return Assembly{}, err
	}
	persistence := host.persistence
	if isNilValue(persistence) {
		return Assembly{}, fmt.Errorf("resolve required service %s: complete Action persistence is not available", runtimecontrol.ServiceName)
	}
	plans := persistence.Plans()
	idempotency := persistence.Idempotency()
	transactions := persistence.Transactions()
	if isNilValue(plans) || isNilValue(idempotency) || isNilValue(transactions) {
		return Assembly{}, fmt.Errorf("resolve required service %s: complete Action persistence is not available", runtimecontrol.ServiceName)
	}
	identities, err := resolveOptionalHostServiceLocked(host, IdentityResolver())
	if err != nil {
		return Assembly{}, err
	}
	sessions, err := resolveOptionalHostServiceLocked(host, SessionAuthenticator())
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
	runtime, err := actionruntime.New(host.registry, actionruntime.Options{
		Authorizer: authorizer, Audit: auditHook, Plans: plans,
		Idempotency: idempotency, Transactions: transactions,
		Clock: host.runtimePolicy.Clock, PlanTTL: host.runtimePolicy.PlanTTL,
		AuditTimeout: host.runtimePolicy.AuditTimeout, AuditFailure: host.runtimePolicy.AuditFailure,
	})
	if err != nil {
		return Assembly{}, fmt.Errorf("create Action Runtime: %w", err)
	}
	assembly := Assembly{runtime: &assemblyRuntime{gate: host.facades, next: runtime}}
	if identities != nil {
		assembly.identities = &assemblyResolver{gate: host.facades, next: identities}
	}
	if sessions != nil {
		assembly.sessions = &assemblyAuthenticator{gate: host.facades, next: sessions}
	}
	if tokens != nil {
		assembly.tokens = &assemblyTokenAuthenticator{gate: host.facades, next: tokens}
	}
	if tasks != nil {
		assembly.tasks = &assemblyTaskService{gate: host.facades, next: tasks}
	}
	return assembly, nil
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
