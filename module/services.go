package module

import (
	"fmt"
	"reflect"
	"regexp"

	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/observe"
	"github.com/iiwish/modary/task"
)

var serviceKeyNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}(?:\.[a-z][a-z0-9-]{0,62})+$`)

type keySpec struct {
	identity *keyIdentity
}

type keyIdentity struct {
	name       string
	capability Capability
	valueType  reflect.Type
}

// Key binds a Go service type to the capability that owns it.
type Key[T any] struct {
	spec keySpec
}

// NewKey constructs an unforgeable typed service key after validating its
// public name and capability. Names require at least two dot-separated
// segments; capabilities use canonical dot or slash-separated segments.
func NewKey[T any](name string, capability Capability) (Key[T], error) {
	if len(name) > 127 || !serviceKeyNamePattern.MatchString(name) {
		return Key[T]{}, fmt.Errorf("module service key name %q must be a dot-namespaced identifier", name)
	}
	if len(capability) > 127 || !capabilityPattern.MatchString(string(capability)) {
		return Key[T]{}, fmt.Errorf("module service key capability %q is invalid", capability)
	}
	return Key[T]{spec: keySpec{identity: &keyIdentity{
		name:       name,
		capability: capability,
		valueType:  reflect.TypeOf((*T)(nil)).Elem(),
	}}}, nil
}

// MustKey constructs a service key for a package-level literal and panics if
// that programmer-owned declaration is invalid. Runtime input should use
// NewKey and handle its error.
func MustKey[T any](name string, capability Capability) Key[T] {
	key, err := NewKey[T](name, capability)
	if err != nil {
		panic(err)
	}
	return key
}

// Name returns the canonical namespaced service name.
func (key Key[T]) Name() string {
	if key.spec.identity == nil {
		return ""
	}
	return key.spec.identity.name
}

// Capability returns the capability governing this service key.
func (key Key[T]) Capability() Capability {
	if key.spec.identity == nil {
		return ""
	}
	return key.spec.identity.capability
}

// Resolver is a sealed, short-lived read-only service view. Only installation
// resolvers and scopes created by this package can implement it; Host
// deliberately cannot. A HandlerFactory may use its Resolver only synchronously
// during that factory call.
type Resolver interface {
	moduleResolver()
}

// Scope is the sealed capability-limited view supplied during Module startup.
// Its operations are safe for concurrent use while StartFunc is active. Every
// goroutine using it must finish before StartFunc returns.
type Scope interface {
	Resolver
	moduleScope()
}

type rawResolver interface {
	resolveService(keySpec) (any, error)
}

// Resolve returns the typed service associated with key from a sealed Resolver.
// It returns ErrInvalidResolver for a forged or nil Resolver and for a retained
// HandlerFactory Resolver. An expired startup Scope returns ErrInvalidScope.
func Resolve[T any](resolver Resolver, key Key[T]) (T, error) {
	var zero T
	if !key.spec.valid() {
		return zero, fmt.Errorf("invalid module service key")
	}
	if isNilValue(resolver) {
		return zero, ErrInvalidResolver
	}
	raw, ok := resolver.(rawResolver)
	if !ok {
		return zero, ErrInvalidResolver
	}
	value, err := raw.resolveService(key.spec)
	if err != nil {
		return zero, err
	}
	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("service %s has type %T, expected %s", key.Name(), value, key.spec.identity.valueType)
	}
	return typed, nil
}

type rawProvider interface {
	provideService(keySpec, any) error
}

// Provide publishes a typed service through an active, capability-owning Scope.
func Provide[T any](scope Scope, key Key[T], value T) error {
	if !key.spec.valid() {
		return fmt.Errorf("invalid module service key")
	}
	if isNilValue(value) {
		return fmt.Errorf("service %s cannot be nil", key.Name())
	}
	if isNilValue(scope) {
		return fmt.Errorf("invalid module installation scope")
	}
	raw, ok := scope.(rawProvider)
	if !ok {
		return fmt.Errorf("invalid module installation scope")
	}
	return raw.provideService(key.spec, value)
}

func (spec keySpec) valid() bool {
	return spec.identity != nil && spec.identity.name != "" && spec.identity.capability != "" && spec.identity.valueType != nil
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var (
	databaseStoreKey    = MustKey[database.Store]("database.store", CapabilityDatabase)
	actionDatabaseKey   = MustKey[database.Access]("database.governed-access", CapabilityDatabase)
	identityResolverKey = MustKey[identity.Resolver]("identity.resolver", CapabilityIdentity)
	passwordAuthKey     = MustKey[identity.PasswordAuthenticator]("identity.password-authenticator", CapabilityPasswords)
	browserAuthKey      = MustKey[identity.BrowserAuthenticator]("identity.browser-authenticator", CapabilityBrowserAuthentication)
	sessionManagerKey   = MustKey[identity.SessionManager]("identity.session-manager", CapabilitySessions)
	tokenAuthKey        = MustKey[identity.TokenAuthenticator]("identity.token-authenticator", CapabilityBearers)
	authorizerKey       = MustKey[authz.Authorizer]("authorization.authorizer", CapabilityAuthorization)
	auditHookKey        = MustKey[audit.Hook]("audit.hook", CapabilityAudit)
	auditReaderKey      = MustKey[audit.Reader]("audit.reader", CapabilityAuditInspection)
	taskServiceKey      = MustKey[task.Service]("tasks.service", CapabilityTasks)
	taskInspectorKey    = MustKey[task.Inspector]("tasks.inspector", CapabilityTaskInspection)
	observabilityKey    = MustKey[observe.Service]("observability.service", CapabilityObservability)
)

func canonicalServiceIdentity(name string) *keyIdentity {
	switch name {
	case databaseStoreKey.Name():
		return databaseStoreKey.spec.identity
	case actionDatabaseKey.Name():
		return actionDatabaseKey.spec.identity
	case identityResolverKey.Name():
		return identityResolverKey.spec.identity
	case passwordAuthKey.Name():
		return passwordAuthKey.spec.identity
	case browserAuthKey.Name():
		return browserAuthKey.spec.identity
	case sessionManagerKey.Name():
		return sessionManagerKey.spec.identity
	case tokenAuthKey.Name():
		return tokenAuthKey.spec.identity
	case authorizerKey.Name():
		return authorizerKey.spec.identity
	case auditHookKey.Name():
		return auditHookKey.spec.identity
	case auditReaderKey.Name():
		return auditReaderKey.spec.identity
	case taskServiceKey.Name():
		return taskServiceKey.spec.identity
	case taskInspectorKey.Name():
		return taskInspectorKey.spec.identity
	case observabilityKey.Name():
		return observabilityKey.spec.identity
	default:
		return nil
	}
}

// Database and the other standard key accessors return copies backed by a
// package-owned identity. Consumers can pass public keys between Modules but
// cannot replace the canonical key for the process. Privileged Action
// persistence has no public key.
func Database() Key[database.Store] { return databaseStoreKey }

// ActionDatabase returns the governed-operation database Access key. It can
// mutate only through a transaction-bound context supplied by the Action
// Runtime and cannot begin a transaction.
func ActionDatabase() Key[database.Access] { return actionDatabaseKey }

// IdentityResolver returns the canonical identity-resolver service key.
func IdentityResolver() Key[identity.Resolver] { return identityResolverKey }

// PasswordAuthenticator returns the canonical password-verifier service key.
func PasswordAuthenticator() Key[identity.PasswordAuthenticator] { return passwordAuthKey }

// BrowserAuthenticator returns the canonical redirect-login service key.
func BrowserAuthenticator() Key[identity.BrowserAuthenticator] { return browserAuthKey }

// SessionManager returns the canonical server-side session service key.
func SessionManager() Key[identity.SessionManager] { return sessionManagerKey }

// TokenAuthenticator returns the canonical bearer-token authenticator key.
func TokenAuthenticator() Key[identity.TokenAuthenticator] { return tokenAuthKey }

// Authorizer returns the canonical authorization service key.
func Authorizer() Key[authz.Authorizer] { return authorizerKey }

// AuditHook returns the canonical audit Hook service key.
func AuditHook() Key[audit.Hook] { return auditHookKey }

// AuditReader returns the canonical bounded audit Reader key.
func AuditReader() Key[audit.Reader] { return auditReaderKey }

// Tasks returns the canonical durable task service key.
func Tasks() Key[task.Service] { return taskServiceKey }

// TaskInspector returns the canonical bounded task Inspector key.
func TaskInspector() Key[task.Inspector] { return taskInspectorKey }

// Observability returns the canonical bounded HTTP observability key.
func Observability() Key[observe.Service] { return observabilityKey }
