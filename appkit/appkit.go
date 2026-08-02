package appkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/task"
)

// DefaultRollbackTimeout bounds Start's wait for cleanup when Runtime assembly
// fails after Modules have started.
const DefaultRollbackTimeout = 10 * time.Second

var (
	// ErrContextRequired reports a nil lifecycle context.
	ErrContextRequired = module.ErrContextRequired
	// ErrApplicationUnavailable reports use of a nil, zero, or shutting-down Application.
	ErrApplicationUnavailable = module.ErrApplicationUnavailable
	// ErrIdentitiesUnavailable reports that no identity resolver was installed.
	ErrIdentitiesUnavailable = errors.New("identity resolver is unavailable")
	// ErrSessionsUnavailable reports that no session authenticator was installed.
	ErrSessionsUnavailable = errors.New("session authenticator is unavailable")
	// ErrTokensUnavailable reports that no token authenticator was installed.
	ErrTokensUnavailable = errors.New("token authenticator is unavailable")
)

// Definition is the consumer-owned application composition source. Inspecting
// it never starts Modules or constructs Action handlers. Start retains Module
// startup callbacks, handler factories, and migration sources only for its
// startup attempt; the returned Application does not retain those references.
// The consumer remains responsible for the lifetime of its original Definition,
// including any credentials captured by Module startup callbacks.
type Definition struct {
	Metadata Metadata
	Modules  []module.Registration
}

// DefinitionProvider constructs a consumer-owned application composition.
// Invocation is synchronous and has no cancellation context, so providers must
// perform no runtime startup or unbounded work and must return promptly. Separate
// commands and project-tool operations may invoke the provider independently.
type DefinitionProvider func() (Definition, error)

// Runtime is the complete governed execution surface exposed by Application.
// It aliases action.Runtime and provides no Registry or Handler access.
type Runtime = action.Runtime

// RuntimeOptions controls Runtime policy without accepting governance
// dependencies. AppKit resolves those dependencies from the Module Host.
type RuntimeOptions = action.RuntimePolicy

// Options controls lifecycle and Runtime policy. RollbackTimeout bounds how
// long Start waits for cleanup after a post-start assembly failure; Host cleanup
// continues under its own callback policy if that wait expires.
type Options struct {
	Shutdown        module.ShutdownPolicy
	Runtime         RuntimeOptions
	RollbackTimeout time.Duration
}

// Application is an opaque, fully assembled governed application.
type Application struct {
	metadata   Metadata
	catalog    []action.CatalogEntry
	runtime    Runtime
	identities identity.Resolver
	sessions   identity.Authenticator
	tokens     identity.TokenAuthenticator
	tasks      task.Service
	ready      func() bool
	shutdown   func(context.Context) error
}

// Start validates the complete static application contract before Module side
// effects, then starts Modules and assembles the governed Runtime.
func Start(ctx context.Context, definition Definition, options Options) (*Application, error) {
	if ctx == nil {
		return nil, ErrContextRequired
	}
	rollbackTimeout, err := validateStartOptions(options)
	if err != nil {
		return nil, err
	}
	if err := ValidateMetadata(definition.Metadata); err != nil {
		return nil, err
	}
	if len(definition.Modules) == 0 {
		return nil, fmt.Errorf("application Definition must contain at least one Module")
	}

	host, err := module.NewHostWithOptions(module.HostOptions{
		Shutdown: options.Shutdown,
		Runtime:  options.Runtime,
	})
	if err != nil {
		return nil, fmt.Errorf("create Module Host: %w", err)
	}
	registrations := append([]module.Registration(nil), definition.Modules...)
	if err := host.Register(registrations...); err != nil {
		return nil, fmt.Errorf("register Modules: %w", err)
	}
	catalog, err := host.Catalog()
	if err != nil {
		return nil, fmt.Errorf("preflight Module catalog: %w", err)
	}
	if err := host.Start(ctx); err != nil {
		return nil, fmt.Errorf("start Modules: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, rollbackAssembly(host, rollbackTimeout, fmt.Errorf("assemble application: %w", err))
	}

	assembly, err := host.Assemble()
	if err != nil {
		return nil, rollbackAssembly(host, rollbackTimeout, fmt.Errorf("assemble application services: %w", err))
	}
	if err := ctx.Err(); err != nil {
		return nil, rollbackAssembly(host, rollbackTimeout, fmt.Errorf("assemble application: %w", err))
	}
	runtime := assembly.Runtime()
	identities := assembly.Identities()
	sessions := assembly.Sessions()
	tokens := assembly.Tokens()
	tasks := assembly.Tasks()
	if err := ctx.Err(); err != nil {
		return nil, rollbackAssembly(host, rollbackTimeout, fmt.Errorf("assemble application: %w", err))
	}

	application := &Application{
		metadata:   definition.Metadata,
		catalog:    cloneCatalog(catalog),
		runtime:    runtime,
		identities: identities,
		sessions:   sessions,
		tokens:     tokens,
		tasks:      tasks,
		ready:      func() bool { return host.State() == module.StateRunning },
		shutdown:   host.Shutdown,
	}
	return application, nil
}

func validateStartOptions(options Options) (time.Duration, error) {
	if options.Shutdown.CallbackTimeout < 0 {
		return 0, fmt.Errorf("cleanup callback timeout cannot be negative")
	}
	if options.Runtime.PlanTTL < 0 {
		return 0, fmt.Errorf("plan TTL cannot be negative")
	}
	if options.Runtime.AuditTimeout < 0 {
		return 0, fmt.Errorf("audit timeout cannot be negative")
	}
	if options.RollbackTimeout < 0 {
		return 0, fmt.Errorf("rollback timeout cannot be negative")
	}
	if options.RollbackTimeout == 0 {
		return DefaultRollbackTimeout, nil
	}
	return options.RollbackTimeout, nil
}

func rollbackAssembly(host *module.Host, timeout time.Duration, primary error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := host.Shutdown(ctx); err != nil {
		return errors.Join(primary, fmt.Errorf("shutdown after application assembly failure: %w", err))
	}
	return primary
}

// Metadata returns the immutable consumer-owned application identity.
func (application *Application) Metadata() Metadata {
	if application == nil {
		return Metadata{}
	}
	return application.metadata
}

// Catalog returns a defensive copy of the read-only Action catalog.
func (application *Application) Catalog() []action.CatalogEntry {
	if application == nil {
		return nil
	}
	return cloneCatalog(application.catalog)
}

// Runtime returns the governed Action execution facade.
func (application *Application) Runtime() Runtime {
	if application == nil {
		return nil
	}
	return application.runtime
}

// Tasks returns the optional durable task service installed by the application.
func (application *Application) Tasks() task.Service {
	if application == nil {
		return nil
	}
	return application.tasks
}

// Ready reports whether startup completed and shutdown has not begun.
func (application *Application) Ready() bool {
	return application != nil && application.ready != nil && application.ready()
}

// Shutdown delegates to the Host-owned exactly-once shutdown sequence and waits
// within ctx. Host cleanup continues independently if this caller stops waiting.
func (application *Application) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return ErrContextRequired
	}
	if application == nil || application.shutdown == nil {
		return ErrApplicationUnavailable
	}
	return application.shutdown(ctx)
}

func cloneCatalog(catalog []action.CatalogEntry) []action.CatalogEntry {
	if catalog == nil {
		return nil
	}
	clone := make([]action.CatalogEntry, len(catalog))
	for index, entry := range catalog {
		clone[index] = entry
		clone[index].Descriptor.InputSchema = append([]byte(nil), entry.Descriptor.InputSchema...)
		clone[index].Descriptor.PreviewSchema = append([]byte(nil), entry.Descriptor.PreviewSchema...)
		clone[index].Descriptor.OutputSchema = append([]byte(nil), entry.Descriptor.OutputSchema...)
		clone[index].Descriptor.Channels = append([]action.Channel(nil), entry.Descriptor.Channels...)
		clone[index].Descriptor.Errors = append([]action.ErrorSpec(nil), entry.Descriptor.Errors...)
	}
	return clone
}
