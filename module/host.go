package module

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/internal/actionruntime"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/internal/safeerr"
)

// State identifies the current Module Host lifecycle phase.
type State string

// DefaultCleanupCallbackTimeout bounds each process-resource cleanup callback
// when no policy is configured.
const DefaultCleanupCallbackTimeout = 5 * time.Second

const (
	// StateUnavailable identifies a nil or zero-value Host. Such a Host cannot
	// enter the lifecycle and must be constructed with NewHost.
	StateUnavailable State = "unavailable"
	// StateNew accepts Module registrations and has not started resources.
	StateNew State = "new"
	// StateStarting is validating or starting registered Modules.
	StateStarting State = "starting"
	// StateRunning exposes assembled component facades and, when declared,
	// permits governed Action execution.
	StateRunning State = "running"
	// StateStopping is draining execution and cleaning Module resources.
	StateStopping State = "stopping"
	// StateStopped has completed an orderly shutdown.
	StateStopped State = "stopped"
	// StateFailed records an unsuccessful startup or cleanup outcome.
	StateFailed State = "failed"
)

var (
	// ErrHostUnavailable reports use of a nil or zero-value Host.
	ErrHostUnavailable = errors.New("module host is unavailable")
	// ErrInvalidState identifies an operation rejected by the Host lifecycle.
	ErrInvalidState = errors.New("invalid module host state")
	// ErrInvalidScope identifies a forged, expired, or otherwise unusable Scope.
	// An expired HandlerFactory Resolver also matches it for F0 compatibility;
	// new Resolver-specific code should match ErrInvalidResolver.
	ErrInvalidScope = errors.New("invalid module installation scope")
	// ErrInvalidResolver identifies a forged, expired, retained, or otherwise
	// unusable HandlerFactory Resolver.
	ErrInvalidResolver = errors.New("invalid module service resolver")
	// ErrNilCleanup identifies an attempt to register a nil cleanup callback.
	ErrNilCleanup = errors.New("module cleanup is nil")
	// ErrCallbackPanic identifies a recovered Module lifecycle callback panic.
	ErrCallbackPanic = errors.New("module callback panicked")
	// ErrReservedServiceName identifies an attempt to publish a framework-owned
	// service name with a recreated or otherwise noncanonical key.
	ErrReservedServiceName = errors.New("reserved module service name")
)

// ShutdownPolicy controls bounded process-resource cleanup. Each callback gets
// its own timeout. Invocation starts LIFO within a Module and in reverse
// dependency order across Modules, but completion is not serialized after a
// timeout: a noncooperative callback may overlap later callbacks and provider
// cleanup. Trusted callbacks must honor cancellation and stop using dependencies
// promptly. A zero timeout selects the safe default.
type ShutdownPolicy struct {
	CallbackTimeout time.Duration
}

// HostOptions configures lifecycle and optional governed Runtime behavior
// without changing Module contracts.
type HostOptions struct {
	Shutdown ShutdownPolicy
	Runtime  action.RuntimePolicy
	// SkipMigrations disables the default apply-before-start behavior. It is
	// intended for serving processes paired with an explicit Migrate invocation.
	SkipMigrations bool
}

// CallbackPanicError reports a recovered lifecycle callback panic without a
// process-dependent stack trace. It unwraps to ErrCallbackPanic.
type CallbackPanicError struct {
	Callback string
}

// Error returns a stable diagnostic that never formats the recovered value.
func (err *CallbackPanicError) Error() string {
	if err == nil || err.Callback == "" {
		return "module callback panicked"
	}
	return fmt.Sprintf("%s callback panicked", err.Callback)
}

// Unwrap returns ErrCallbackPanic, including for a typed-nil receiver.
func (err *CallbackPanicError) Unwrap() error { return ErrCallbackPanic }

type dependencyError struct {
	operation string
	cause     error
}

// Error returns stable context without formatting the dependency cause.
func (err *dependencyError) Error() string {
	if err == nil || err.operation == "" {
		return "module dependency failed"
	}
	return err.operation + " failed"
}

// Unwrap exposes the dependency cause through a safe opaque boundary.
func (err *dependencyError) Unwrap() error {
	if err == nil {
		return nil
	}
	return safeerr.Opaque(err.cause)
}

// StateError reports an operation that is invalid in the observed Host state.
type StateError struct {
	Operation string
	State     State
}

// Error describes the rejected lifecycle operation.
func (err *StateError) Error() string {
	if err == nil || err.Operation == "" {
		return "module host state is invalid"
	}
	return fmt.Sprintf("cannot %s module host in %s state", err.Operation, err.State)
}

// Unwrap returns ErrInvalidState, including for a typed-nil receiver.
func (err *StateError) Unwrap() error { return ErrInvalidState }

type serviceRecord struct {
	key   keySpec
	value any
	owner string
}

type moduleRuntime struct {
	mu       sync.Mutex
	id       string
	services []string
	cleanups []Cleanup
	active   bool
	mutable  bool
	cleaned  bool
}

type hostInitialization struct {
	owner *Host
}

// Host validates, starts, and stops one static set of Module registrations.
// A Host must be constructed with NewHost or NewHostWithOptions and must not be
// copied; nil, zero-value, partially initialized, and copied Hosts are
// unavailable.
type Host struct {
	mu                sync.RWMutex
	initialization    *hostInitialization
	state             State
	registrations     map[string]Registration
	prepared          map[string]action.PreparedDescriptor
	services          map[string]serviceRecord
	started           []string
	runtimes          []*moduleRuntime
	graph             Graph
	registry          *actionruntime.Registry
	database          databasecontrol.Control
	databaseOwner     string
	persistence       runtimecontrol.Persistence
	persistenceOwner  string
	shutdownPolicy    ShutdownPolicy
	runtimePolicy     action.RuntimePolicy
	skipMigrations    bool
	facades           *assemblyGate
	assembly          *Assembly
	assemblyErr       error
	assemblyAttempted bool
	startCancel       context.CancelFunc
	startDone         chan struct{}
	stopRequested     bool
	shutdownDone      chan struct{}
	shutdownErr       error
}

// NewHost constructs a Host with the default cooperative shutdown policy.
func NewHost() *Host {
	host, err := NewHostWithOptions(HostOptions{})
	if err != nil {
		panic(fmt.Sprintf("create Module Host: %v", err))
	}
	return host
}

// NewHostWithOptions validates options and constructs a Host. NewHost is the
// simple default for applications that do not need a custom shutdown policy.
func NewHostWithOptions(options HostOptions) (*Host, error) {
	if options.Shutdown.CallbackTimeout < 0 {
		return nil, fmt.Errorf("shutdown callback timeout cannot be negative")
	}
	if options.Runtime.PlanTTL < 0 {
		return nil, fmt.Errorf("plan TTL cannot be negative")
	}
	if options.Runtime.AuditTimeout < 0 {
		return nil, fmt.Errorf("audit timeout cannot be negative")
	}
	if options.Shutdown.CallbackTimeout == 0 {
		options.Shutdown.CallbackTimeout = DefaultCleanupCallbackTimeout
	}
	registry := actionruntime.NewRegistry()
	host := &Host{
		state:          StateNew,
		registrations:  make(map[string]Registration),
		prepared:       make(map[string]action.PreparedDescriptor),
		services:       make(map[string]serviceRecord),
		registry:       registry,
		shutdownPolicy: options.Shutdown,
		runtimePolicy:  options.Runtime,
		skipMigrations: options.SkipMigrations,
		facades:        newAssemblyGate(),
		shutdownDone:   make(chan struct{}),
	}
	host.initialization = &hostInitialization{owner: host}
	return host, nil
}

// State returns the current lifecycle state.
func (host *Host) State() State {
	if !host.available() {
		return StateUnavailable
	}
	host.mu.RLock()
	defer host.mu.RUnlock()
	return host.state
}

// Register adds pure Module registrations before the Host starts.
func (host *Host) Register(registrations ...Registration) error {
	if !host.available() {
		return ErrHostUnavailable
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.state != StateNew {
		return &StateError{Operation: "register modules", State: host.state}
	}
	pending := make(map[string]Registration, len(registrations))
	pendingPrepared := make(map[string]action.PreparedDescriptor)
	actionOwners := make(map[string]string, len(host.prepared))
	for moduleID, registration := range host.registrations {
		for _, binding := range registration.Definition.Actions {
			actionOwners[binding.Descriptor.ID] = moduleID
		}
	}
	for _, registration := range registrations {
		registration = cloneRegistration(registration)
		manifest := registration.Definition.Manifest
		if _, exists := host.registrations[manifest.ID]; exists {
			return fmt.Errorf("module %s is already registered", manifest.ID)
		}
		if _, exists := pending[manifest.ID]; exists {
			return fmt.Errorf("module %s is already registered", manifest.ID)
		}
		prepared, err := prepareRegistration(registration)
		if err != nil {
			return err
		}
		for actionID, contract := range prepared {
			if owner, exists := actionOwners[actionID]; exists {
				return fmt.Errorf("duplicate action id %q in modules %s and %s", actionID, owner, manifest.ID)
			}
			actionOwners[actionID] = manifest.ID
			pendingPrepared[actionID] = contract
		}
		pending[manifest.ID] = registration
	}
	for id, registration := range pending {
		host.registrations[id] = registration
	}
	for actionID, contract := range pendingPrepared {
		host.prepared[actionID] = contract
	}
	return nil
}

func prepareRegistration(registration Registration) (map[string]action.PreparedDescriptor, error) {
	manifest := registration.Definition.Manifest
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	if len(registration.Definition.Migrations) > 0 &&
		!contains(manifest.Requires, CapabilityDatabase) &&
		!contains(manifest.Provides, CapabilityDatabase) {
		return nil, fmt.Errorf("module %s declares migrations without requiring or providing capability %q", manifest.ID, CapabilityDatabase)
	}
	seenMigrationDrivers := make(map[string]struct{}, len(registration.Definition.Migrations))
	for _, migration := range registration.Definition.Migrations {
		if !migrationDriverPattern.MatchString(migration.Driver) {
			return nil, fmt.Errorf("module %s has invalid migration driver %q", manifest.ID, migration.Driver)
		}
		if isNilValue(migration.Files) {
			return nil, fmt.Errorf("module %s migration driver %s has no files", manifest.ID, migration.Driver)
		}
		if _, exists := seenMigrationDrivers[migration.Driver]; exists {
			return nil, fmt.Errorf("module %s declares migration driver %s more than once", manifest.ID, migration.Driver)
		}
		seenMigrationDrivers[migration.Driver] = struct{}{}
	}
	prepared := make(map[string]action.PreparedDescriptor, len(registration.Definition.Actions))
	for _, binding := range registration.Definition.Actions {
		if binding.NewHandler == nil {
			return nil, fmt.Errorf("action %s in module %s has no handler factory", binding.Descriptor.ID, manifest.ID)
		}
		contract, err := action.PrepareDescriptor(binding.Descriptor)
		if err != nil {
			return nil, fmt.Errorf("module %s: %w", manifest.ID, err)
		}
		if _, exists := prepared[binding.Descriptor.ID]; exists {
			return nil, fmt.Errorf("duplicate action id %q in module %s", binding.Descriptor.ID, manifest.ID)
		}
		prepared[binding.Descriptor.ID] = contract
	}
	return prepared, nil
}

func (host *Host) applyMigrations(ctx context.Context, definition Definition) (err error) {
	returned := false
	defer recoverCallback("migration", &returned, &err)
	err = host.applyMigrationsUnchecked(ctx, definition)
	returned = true
	return err
}

func (host *Host) applyMigrationsUnchecked(ctx context.Context, definition Definition) error {
	if len(definition.Migrations) == 0 {
		return nil
	}
	host.mu.RLock()
	control := host.database
	host.mu.RUnlock()
	if control == nil {
		return fmt.Errorf("database control is not available")
	}
	driver := control.Driver()
	for _, source := range definition.Migrations {
		if source.Driver != driver {
			continue
		}
		if applyErr := control.ApplyMigrations(ctx, definition.Manifest.ID, source.Files); applyErr != nil {
			if databasecontrol.ContainsDependencyPanic(applyErr) {
				return &CallbackPanicError{Callback: "migration"}
			}
			return &dependencyError{operation: "apply " + driver + " migrations", cause: applyErr}
		}
		return nil
	}
	return fmt.Errorf("module %s has no migrations for database driver %s", definition.Manifest.ID, driver)
}

// Migrate applies every selected forward migration without starting feature
// Modules or binding Action handlers. It starts only the database provider and
// its transitive dependencies, then cleans those resources before returning.
// Like Start, it is a one-shot Host lifecycle operation.
func (host *Host) Migrate(ctx context.Context) error {
	if ctx == nil {
		return ErrContextRequired
	}
	if !host.available() {
		return ErrHostUnavailable
	}
	host.mu.Lock()
	if host.state != StateNew {
		state := host.state
		host.mu.Unlock()
		return &StateError{Operation: "migrate", State: state}
	}
	host.state = StateStarting
	migrateCtx, cancel := context.WithCancel(ctx)
	host.startCancel = cancel
	host.startDone = make(chan struct{})
	host.stopRequested = false
	registrations := make(map[string]Registration, len(host.registrations))
	for id, registration := range host.registrations {
		registrations[id] = registration
	}
	prepared := make(map[string]action.PreparedDescriptor, len(host.prepared))
	for actionID, contract := range host.prepared {
		prepared[actionID] = contract
	}
	host.mu.Unlock()
	defer host.finishStart()

	graph, _, err := validateRegistrations(registrations, prepared)
	if err != nil {
		host.completeStartFailure(nil)
		return err
	}
	host.mu.Lock()
	host.graph = graph
	host.mu.Unlock()

	hasMigrations := false
	for _, registration := range registrations {
		if len(registration.Definition.Migrations) > 0 {
			hasMigrations = true
			break
		}
	}
	if !hasMigrations {
		host.completeMigration(nil)
		return nil
	}
	databaseOwner, ok := graph.Provides[CapabilityDatabase]
	if !ok {
		host.completeStartFailure(nil)
		return fmt.Errorf("migration graph has no database provider")
	}
	startupSet := map[string]bool{databaseOwner: true}
	for changed := true; changed; {
		changed = false
		for _, edge := range graph.Edges {
			if startupSet[edge.From] && !startupSet[edge.To] {
				startupSet[edge.To] = true
				changed = true
			}
		}
	}

	for _, id := range graph.Order {
		if !startupSet[id] {
			continue
		}
		if err := migrateCtx.Err(); err != nil {
			return host.startFailure(migrateCtx, fmt.Errorf("migrate modules: %w", err), nil)
		}
		registration := registrations[id]
		runtime := &moduleRuntime{id: id, active: true, mutable: true}
		install := &installScope{host: host, manifest: registration.Definition.Manifest, runtime: runtime, active: true}
		var startErr error
		if registration.Start != nil {
			startErr = invokeStart(registration.Start, migrateCtx, install)
		}
		install.expire()
		if startErr != nil {
			return host.startFailure(migrateCtx, fmt.Errorf("start migration dependency %s: %w", id, startErr), runtime)
		}
		runtime.deactivate()
		host.mu.Lock()
		host.runtimes = append(host.runtimes, runtime)
		host.started = append(host.started, id)
		host.mu.Unlock()
	}
	for _, id := range graph.Order {
		definition := registrations[id].Definition
		if len(definition.Migrations) == 0 {
			continue
		}
		if err := host.applyMigrations(migrateCtx, definition); err != nil {
			return host.startFailure(migrateCtx, fmt.Errorf("apply migrations for module %s: %w", id, err), nil)
		}
	}
	host.mu.RLock()
	started := append([]*moduleRuntime(nil), host.runtimes...)
	host.mu.RUnlock()
	cleanupErr := host.cleanup(migrateCtx, started)
	host.completeMigration(cleanupErr)
	return cleanupErr
}

func (host *Host) completeMigration(cleanupErr error) {
	host.mu.Lock()
	if cleanupErr != nil {
		host.state = StateFailed
	} else {
		host.state = StateStopped
	}
	host.shutdownErr = cleanupErr
	host.releaseStartupReferencesLocked()
	select {
	case <-host.shutdownDone:
	default:
		close(host.shutdownDone)
	}
	host.mu.Unlock()
}

// Start validates, migrates, and starts Modules in dependency order.
func (host *Host) Start(ctx context.Context) error {
	if ctx == nil {
		return ErrContextRequired
	}
	if !host.available() {
		return ErrHostUnavailable
	}
	host.mu.Lock()
	if host.state != StateNew {
		state := host.state
		host.mu.Unlock()
		return &StateError{Operation: "start", State: state}
	}
	host.state = StateStarting
	startCtx, cancel := context.WithCancel(ctx)
	host.startCancel = cancel
	host.startDone = make(chan struct{})
	host.stopRequested = false
	registrations := make(map[string]Registration, len(host.registrations))
	for id, registration := range host.registrations {
		registrations[id] = registration
	}
	prepared := make(map[string]action.PreparedDescriptor, len(host.prepared))
	for actionID, contract := range host.prepared {
		prepared[actionID] = contract
	}
	host.mu.Unlock()
	defer host.finishStart()

	graph, _, err := validateRegistrations(registrations, prepared)
	if err != nil {
		host.completeStartFailure(nil)
		return err
	}
	host.mu.Lock()
	host.graph = graph
	host.mu.Unlock()

	if err := startCtx.Err(); err != nil {
		return host.startFailure(startCtx, fmt.Errorf("start modules: %w", err), nil)
	}
	for _, id := range graph.Order {
		if err := startCtx.Err(); err != nil {
			return host.startFailure(startCtx, fmt.Errorf("start modules: %w", err), nil)
		}
		registration := registrations[id]
		runtime := &moduleRuntime{id: id, active: true, mutable: true}
		install := &installScope{
			host: host, manifest: registration.Definition.Manifest, runtime: runtime, active: true,
		}
		providesDatabase := contains(registration.Definition.Manifest.Provides, CapabilityDatabase)
		if !providesDatabase && !host.skipMigrations {
			if err := host.applyMigrations(startCtx, registration.Definition); err != nil {
				install.expire()
				return host.startFailure(startCtx, fmt.Errorf("apply migrations for module %s: %w", id, err), runtime)
			}
		}
		var startErr error
		if registration.Start != nil {
			startErr = invokeStart(registration.Start, startCtx, install)
		}
		install.expire()
		if startErr != nil {
			return host.startFailure(startCtx, fmt.Errorf("start module %s: %w", id, startErr), runtime)
		}
		if err := startCtx.Err(); err != nil {
			return host.startFailure(startCtx, fmt.Errorf("start module %s: %w", id, err), runtime)
		}
		if providesDatabase && !host.skipMigrations {
			if err := host.applyMigrations(startCtx, registration.Definition); err != nil {
				return host.startFailure(startCtx, fmt.Errorf("apply migrations for module %s: %w", id, err), runtime)
			}
		}
		for _, binding := range registration.Definition.Actions {
			resolver := &installResolver{
				host: host, manifest: registration.Definition.Manifest, runtime: runtime, active: true,
			}
			handler, err := invokeHandlerFactory(binding.NewHandler, startCtx, resolver)
			resolver.expire()
			if err != nil {
				return host.startFailure(startCtx, fmt.Errorf("create handler %s for module %s: %w", binding.Descriptor.ID, id, err), runtime)
			}
			if err := startCtx.Err(); err != nil {
				return host.startFailure(startCtx, fmt.Errorf("create handler %s for module %s: %w", binding.Descriptor.ID, id, err), runtime)
			}
			contract, ok := prepared[binding.Descriptor.ID]
			if !ok {
				return host.startFailure(startCtx, fmt.Errorf("bind action %s for module %s: prepared contract is missing", binding.Descriptor.ID, id), runtime)
			}
			if err := host.registry.BindPrepared(id, contract, handler); err != nil {
				return host.startFailure(startCtx, fmt.Errorf("bind action %s for module %s: %w", binding.Descriptor.ID, id, err), runtime)
			}
		}
		runtime.deactivate()
		host.mu.Lock()
		host.runtimes = append(host.runtimes, runtime)
		host.started = append(host.started, id)
		host.mu.Unlock()
	}

	host.mu.Lock()
	if host.stopRequested || startCtx.Err() != nil {
		startErr := startCtx.Err()
		if startErr == nil {
			startErr = context.Canceled
		}
		host.mu.Unlock()
		return host.startFailure(startCtx, fmt.Errorf("start modules: %w", startErr), nil)
	}
	host.state = StateRunning
	host.releaseStartupReferencesLocked()
	host.mu.Unlock()
	return nil
}

func validateRegistrations(registrations map[string]Registration, prepared map[string]action.PreparedDescriptor) (Graph, []action.CatalogEntry, error) {
	manifests := make([]Manifest, 0, len(registrations))
	catalog := make([]action.CatalogEntry, 0)
	moduleIDs := make([]string, 0, len(registrations))
	for id := range registrations {
		moduleIDs = append(moduleIDs, id)
	}
	sort.Strings(moduleIDs)
	for _, id := range moduleIDs {
		registration := registrations[id]
		manifest := registration.Definition.Manifest
		if manifest.ID != id {
			return Graph{}, nil, fmt.Errorf("module registration key %s contains definition %s", id, manifest.ID)
		}
		for _, binding := range registration.Definition.Actions {
			contract, ok := prepared[binding.Descriptor.ID]
			if !ok {
				return Graph{}, nil, fmt.Errorf("module %s action %s has no prepared contract", id, binding.Descriptor.ID)
			}
			catalog = append(catalog, action.CatalogEntry{Descriptor: contract.Descriptor(), ModuleID: id, ContractHash: contract.ContractHash()})
		}
		manifests = append(manifests, manifest)
	}
	graph, err := Verify(manifests)
	if err != nil {
		return Graph{}, nil, err
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].Descriptor.ID < catalog[j].Descriptor.ID })
	return graph, catalog, nil
}

func (host *Host) startFailure(ctx context.Context, primary error, current *moduleRuntime) error {
	host.mu.RLock()
	started := append([]*moduleRuntime(nil), host.runtimes...)
	host.mu.RUnlock()
	if current != nil {
		started = append(started, current)
	}
	cleanupErr := host.cleanup(ctx, started)
	host.completeStartFailure(cleanupErr)
	return errors.Join(primary, cleanupErr)
}

func (host *Host) completeStartFailure(cleanupErr error) {
	host.mu.Lock()
	defer host.mu.Unlock()
	if cleanupErr != nil {
		host.state = StateFailed
	} else if host.stopRequested {
		host.state = StateStopped
	} else {
		host.state = StateFailed
	}
	host.releaseStartupReferencesLocked()
	host.shutdownErr = cleanupErr
	select {
	case <-host.shutdownDone:
	default:
		close(host.shutdownDone)
	}
}

func (host *Host) finishStart() {
	host.mu.Lock()
	if host.startCancel != nil {
		host.startCancel()
		host.startCancel = nil
	}
	done := host.startDone
	host.startDone = nil
	host.mu.Unlock()
	if done != nil {
		close(done)
	}
}

// Shutdown starts the shared Host shutdown sequence and waits for it within ctx.
// The sequence revokes and drains every assembled facade before Module cleanup,
// and continues independently if this caller stops waiting.
func (host *Host) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return ErrContextRequired
	}
	if !host.available() {
		return ErrHostUnavailable
	}
	host.mu.Lock()
	switch host.state {
	case StateStopped, StateFailed:
		err := host.shutdownErr
		host.mu.Unlock()
		return err
	case StateStopping:
		done := host.shutdownDone
		host.mu.Unlock()
		return host.waitForShutdown(ctx, done)
	case StateStarting:
		host.stopRequested = true
		cancel := host.startCancel
		done := host.startDone
		host.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if done == nil {
			return nil
		}
		return host.waitForShutdown(ctx, done)
	case StateRunning:
		host.state = StateStopping
		drained := host.facades.revoke()
		revokeErr := host.registry.Revoke()
		runtimes := append([]*moduleRuntime(nil), host.runtimes...)
		done := host.shutdownDone
		host.mu.Unlock()
		go host.finishShutdown(drained, runtimes, revokeErr)
		return host.waitForShutdown(ctx, done)
	default:
		state := host.state
		host.mu.Unlock()
		return &StateError{Operation: "shutdown", State: state}
	}
}

func (host *Host) finishShutdown(drained <-chan struct{}, runtimes []*moduleRuntime, revokeErr error) {
	<-drained
	var err error
	if revokeErr != nil {
		err = fmt.Errorf("revoke Action bindings: %w", revokeErr)
	}
	err = errors.Join(err, host.cleanup(context.Background(), runtimes))
	host.mu.Lock()
	if err != nil {
		host.state = StateFailed
	} else {
		host.state = StateStopped
	}
	host.shutdownErr = err
	close(host.shutdownDone)
	host.mu.Unlock()
}

func (host *Host) waitForShutdown(ctx context.Context, done <-chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-done:
		host.mu.RLock()
		defer host.mu.RUnlock()
		return host.shutdownErr
	case <-ctx.Done():
		select {
		case <-done:
			host.mu.RLock()
			defer host.mu.RUnlock()
			return host.shutdownErr
		default:
			return ctx.Err()
		}
	}
}

func (host *Host) cleanup(parent context.Context, runtimes []*moduleRuntime) error {
	var result error
	// Revoke new executions before any service or process resource is released.
	if err := host.registry.Reset(context.WithoutCancel(parent)); err != nil {
		return errors.Join(result, fmt.Errorf("revoke and drain Action bindings: %w", err))
	}
	cleanupParent := context.WithoutCancel(parent)
	for moduleIndex := len(runtimes) - 1; moduleIndex >= 0; moduleIndex-- {
		runtime := runtimes[moduleIndex]
		services, cleanups, ok := runtime.beginCleanup()
		if !ok {
			continue
		}
		for cleanupIndex := len(cleanups) - 1; cleanupIndex >= 0; cleanupIndex-- {
			err := invokeCleanupBounded(cleanups[cleanupIndex], cleanupParent, host.shutdownPolicy.CallbackTimeout)
			if err != nil {
				result = errors.Join(result, fmt.Errorf("cleanup module %s: %w", runtime.id, err))
			}
		}
		host.mu.Lock()
		for _, name := range services {
			if record, ok := host.services[name]; ok && record.owner == runtime.id {
				delete(host.services, name)
				if name == databaseStoreKey.Name() && host.databaseOwner == runtime.id {
					host.database = nil
					host.databaseOwner = ""
				}
				if name == runtimecontrol.ServiceName && host.persistenceOwner == runtime.id {
					host.persistence = nil
					host.persistenceOwner = ""
				}
			}
		}
		host.mu.Unlock()
	}
	return result
}

func (host *Host) resolveService(key keySpec) (any, error) {
	host.mu.RLock()
	defer host.mu.RUnlock()
	if host.state != StateRunning {
		return nil, &StateError{Operation: "resolve services", State: host.state}
	}
	return host.resolveLocked(key)
}

func (host *Host) resolveLocked(key keySpec) (any, error) {
	if !key.valid() {
		return nil, fmt.Errorf("invalid module service key")
	}
	record, ok := host.services[key.identity.name]
	if !ok {
		return nil, fmt.Errorf("required service %s is not available", key.identity.name)
	}
	if record.key.identity != key.identity {
		return nil, fmt.Errorf("service key %s does not match the registered key", key.identity.name)
	}
	return record.value, nil
}

// Catalog validates and returns the static read-only Action catalog.
func (host *Host) Catalog() ([]action.CatalogEntry, error) {
	if !host.available() {
		return nil, ErrHostUnavailable
	}
	host.mu.RLock()
	registrations := make(map[string]Registration, len(host.registrations))
	for id, registration := range host.registrations {
		registrations[id] = registration
	}
	prepared := make(map[string]action.PreparedDescriptor, len(host.prepared))
	for actionID, contract := range host.prepared {
		prepared[actionID] = contract
	}
	host.mu.RUnlock()
	_, catalog, err := validateRegistrations(registrations, prepared)
	return catalog, err
}

func cloneRegistration(registration Registration) Registration {
	clone := registration
	clone.Definition.Manifest.Requires = append([]Capability(nil), registration.Definition.Manifest.Requires...)
	clone.Definition.Manifest.Provides = append([]Capability(nil), registration.Definition.Manifest.Provides...)
	clone.Definition.Actions = make([]ActionBinding, len(registration.Definition.Actions))
	for index, binding := range registration.Definition.Actions {
		clone.Definition.Actions[index] = binding
		clone.Definition.Actions[index].Descriptor = cloneDescriptor(binding.Descriptor)
	}
	clone.Definition.Migrations = append([]MigrationSource(nil), registration.Definition.Migrations...)
	return clone
}

func cloneDescriptor(descriptor action.Descriptor) action.Descriptor {
	clone := descriptor
	clone.InputSchema = append([]byte(nil), descriptor.InputSchema...)
	clone.PreviewSchema = append([]byte(nil), descriptor.PreviewSchema...)
	clone.OutputSchema = append([]byte(nil), descriptor.OutputSchema...)
	clone.Channels = append([]action.Channel(nil), descriptor.Channels...)
	clone.Errors = append([]action.ErrorSpec(nil), descriptor.Errors...)
	return clone
}

// releaseStartupReferencesLocked discards only the Host-owned callback and
// migration-source references after the one permitted startup attempt. Static
// descriptors remain available for inspection. The caller-owned Registration
// is a separate defensive copy and retains its own lifecycle.
func (host *Host) releaseStartupReferencesLocked() {
	for id, registration := range host.registrations {
		registration.Start = nil
		for index := range registration.Definition.Actions {
			registration.Definition.Actions[index].NewHandler = nil
		}
		for index := range registration.Definition.Migrations {
			registration.Definition.Migrations[index].Files = nil
		}
		host.registrations[id] = registration
	}
}

func (host *Host) available() bool {
	return host != nil && host.initialization != nil && host.initialization.owner == host
}

// Manifests returns defensive copies of registered Module manifests.
func (host *Host) Manifests() []Manifest {
	if !host.available() {
		return nil
	}
	host.mu.RLock()
	defer host.mu.RUnlock()
	manifests := make([]Manifest, 0, len(host.registrations))
	for _, registration := range host.registrations {
		manifest := registration.Definition.Manifest
		manifest.Requires = append([]Capability(nil), manifest.Requires...)
		manifest.Provides = append([]Capability(nil), manifest.Provides...)
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].ID < manifests[j].ID })
	return manifests
}

// StartedModules returns Module IDs that completed startup in dependency order.
func (host *Host) StartedModules() []string {
	if !host.available() {
		return nil
	}
	host.mu.RLock()
	defer host.mu.RUnlock()
	return append([]string(nil), host.started...)
}

type installScope struct {
	mu       sync.Mutex
	host     *Host
	manifest Manifest
	runtime  *moduleRuntime
	active   bool
}

type installResolver struct {
	mu       sync.Mutex
	host     *Host
	manifest Manifest
	runtime  *moduleRuntime
	active   bool
}

func (*installScope) moduleResolver()    {}
func (*installScope) moduleScope()       {}
func (*installResolver) moduleResolver() {}

func (scope *installScope) provideService(key keySpec, value any) error {
	if scope == nil {
		return ErrInvalidScope
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if !scope.active {
		return ErrInvalidScope
	}
	if !key.valid() || isNilValue(value) {
		return fmt.Errorf("service %s cannot be nil or invalid", keyName(key))
	}
	if canonical := canonicalServiceIdentity(key.identity.name); canonical != nil && key.identity != canonical {
		return fmt.Errorf("%w: %s requires its canonical framework key", ErrReservedServiceName, key.identity.name)
	}
	scope.runtime.mu.Lock()
	defer scope.runtime.mu.Unlock()
	if !scope.runtime.active || !scope.runtime.mutable || !contains(scope.manifest.Provides, key.identity.capability) {
		return fmt.Errorf("module %s cannot provide capability %q through service %s", scope.manifest.ID, key.identity.capability, key.identity.name)
	}
	if key.identity == databaseStoreKey.spec.identity || key.identity == actionDatabaseKey.spec.identity {
		return fmt.Errorf("database access must be installed through privileged database control")
	}
	scope.host.mu.Lock()
	defer scope.host.mu.Unlock()
	if scope.host.state != StateStarting {
		return &StateError{Operation: "provide services", State: scope.host.state}
	}
	if key.identity.name == databasecontrol.ServiceName {
		return scope.provideDatabaseServiceLocked(key, value)
	}
	if key.identity.name == runtimecontrol.ServiceName {
		return scope.provideActionPersistenceLocked(key, value)
	}
	if existing, exists := scope.host.services[key.identity.name]; exists {
		return fmt.Errorf("service %s is already provided by module %s", key.identity.name, existing.owner)
	}
	scope.host.services[key.identity.name] = serviceRecord{key: key, value: value, owner: scope.manifest.ID}
	scope.runtime.services = append(scope.runtime.services, key.identity.name)
	return nil
}

// provideDatabaseServiceLocked recognizes the private internal service type
// and atomically publishes its public Access facade alongside it. The caller
// holds the scope, runtime, and Host locks in that order.
func (scope *installScope) provideDatabaseServiceLocked(key keySpec, value any) error {
	controlType := reflect.TypeOf((*databasecontrol.Control)(nil)).Elem()
	if key.identity.capability != CapabilityDatabase || key.identity.valueType != controlType {
		return fmt.Errorf("%w: %s requires internal database control", ErrReservedServiceName, databasecontrol.ServiceName)
	}
	control, ok := value.(databasecontrol.Control)
	if !ok || isNilValue(control) || isNilValue(control.Access()) || isNilValue(control.Store()) {
		return fmt.Errorf("database control, store, and governed access are required")
	}
	if existing, exists := scope.host.services[databasecontrol.ServiceName]; exists {
		return fmt.Errorf("service %s is already provided by module %s", databasecontrol.ServiceName, existing.owner)
	}
	if existing, exists := scope.host.services[databaseStoreKey.Name()]; exists {
		return fmt.Errorf("service %s is already provided by module %s", databaseStoreKey.Name(), existing.owner)
	}
	if existing, exists := scope.host.services[actionDatabaseKey.Name()]; exists {
		return fmt.Errorf("service %s is already provided by module %s", actionDatabaseKey.Name(), existing.owner)
	}
	if scope.host.database != nil {
		return fmt.Errorf("database control is already provided by module %s", scope.host.databaseOwner)
	}
	owner := scope.manifest.ID
	scope.host.services[databasecontrol.ServiceName] = serviceRecord{key: key, value: control, owner: owner}
	scope.host.services[databaseStoreKey.Name()] = serviceRecord{key: databaseStoreKey.spec, value: control.Store(), owner: owner}
	scope.host.services[actionDatabaseKey.Name()] = serviceRecord{key: actionDatabaseKey.spec, value: control.Access(), owner: owner}
	scope.host.database = control
	scope.host.databaseOwner = owner
	scope.runtime.services = append(scope.runtime.services, databasecontrol.ServiceName, databaseStoreKey.Name(), actionDatabaseKey.Name())
	return nil
}

// provideActionPersistenceLocked recognizes the sealed private persistence
// bundle. The caller holds the scope, runtime, and Host locks in that order.
func (scope *installScope) provideActionPersistenceLocked(key keySpec, value any) error {
	persistenceType := reflect.TypeOf((*runtimecontrol.Persistence)(nil)).Elem()
	if key.identity.capability != CapabilityDatabase || key.identity.valueType != persistenceType {
		return fmt.Errorf("%w: %s requires internal Action persistence", ErrReservedServiceName, runtimecontrol.ServiceName)
	}
	persistence, ok := value.(runtimecontrol.Persistence)
	if !ok || isNilValue(persistence) || isNilValue(persistence.Plans()) ||
		isNilValue(persistence.Idempotency()) || isNilValue(persistence.Transactions()) {
		return fmt.Errorf("complete Action persistence is required")
	}
	if existing, exists := scope.host.services[runtimecontrol.ServiceName]; exists {
		return fmt.Errorf("service %s is already provided by module %s", runtimecontrol.ServiceName, existing.owner)
	}
	if scope.host.persistence != nil {
		return fmt.Errorf("Action persistence is already provided by module %s", scope.host.persistenceOwner)
	}
	owner := scope.manifest.ID
	scope.host.services[runtimecontrol.ServiceName] = serviceRecord{key: key, value: persistence, owner: owner}
	scope.host.persistence = persistence
	scope.host.persistenceOwner = owner
	scope.runtime.services = append(scope.runtime.services, runtimecontrol.ServiceName)
	return nil
}

func (scope *installScope) resolveService(key keySpec) (any, error) {
	if scope == nil {
		return nil, ErrInvalidScope
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if !scope.active {
		return nil, ErrInvalidScope
	}
	return resolveModuleService(scope.host, scope.manifest, scope.runtime, key)
}

func resolveModuleService(host *Host, manifest Manifest, runtime *moduleRuntime, key keySpec) (any, error) {
	if !key.valid() {
		return nil, fmt.Errorf("invalid module service key")
	}
	if host == nil || runtime == nil {
		return nil, ErrInvalidScope
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if !runtime.active || (!contains(manifest.Requires, key.identity.capability) && !contains(manifest.Provides, key.identity.capability)) {
		return nil, fmt.Errorf("module %s cannot access capability %q through service %s because it is not declared", manifest.ID, key.identity.capability, key.identity.name)
	}
	host.mu.RLock()
	defer host.mu.RUnlock()
	if host.state != StateStarting {
		return nil, &StateError{Operation: "resolve services", State: host.state}
	}
	return host.resolveLocked(key)
}

func (resolver *installResolver) resolveService(key keySpec) (any, error) {
	if resolver == nil {
		return nil, errors.Join(ErrInvalidResolver, ErrInvalidScope)
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if !resolver.active {
		// ErrInvalidScope remains joined for source compatibility with F0
		// callers that classified retained Resolver values as expired scopes.
		return nil, errors.Join(ErrInvalidResolver, ErrInvalidScope)
	}
	return resolveModuleService(resolver.host, resolver.manifest, resolver.runtime, key)
}

func (scope *installScope) onStop(cleanup Cleanup) error {
	if scope == nil {
		return ErrInvalidScope
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if !scope.active {
		return ErrInvalidScope
	}
	scope.runtime.mu.Lock()
	defer scope.runtime.mu.Unlock()
	if !scope.runtime.active || !scope.runtime.mutable {
		return ErrInvalidScope
	}
	scope.host.mu.RLock()
	state := scope.host.state
	scope.host.mu.RUnlock()
	if state != StateStarting {
		return &StateError{Operation: "register cleanup", State: state}
	}
	scope.runtime.cleanups = append(scope.runtime.cleanups, cleanup)
	return nil
}

func (scope *installScope) expire() {
	if scope == nil {
		return
	}
	scope.mu.Lock()
	scope.active = false
	scope.mu.Unlock()
}

func (resolver *installResolver) expire() {
	if resolver == nil {
		return
	}
	resolver.mu.Lock()
	resolver.active = false
	resolver.mu.Unlock()
}

func (runtime *moduleRuntime) deactivate() {
	runtime.mu.Lock()
	runtime.active = false
	runtime.mutable = false
	runtime.mu.Unlock()
}

func (runtime *moduleRuntime) beginCleanup() ([]string, []Cleanup, bool) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.cleaned {
		return nil, nil, false
	}
	runtime.active = false
	runtime.mutable = false
	runtime.cleaned = true
	return append([]string(nil), runtime.services...), append([]Cleanup(nil), runtime.cleanups...), true
}

func keyName(key keySpec) string {
	if key.identity == nil {
		return "<invalid>"
	}
	return key.identity.name
}

func contains(values []Capability, target Capability) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func invokeStart(callback StartFunc, ctx context.Context, scope Scope) (err error) {
	returned := false
	defer recoverCallback("start", &returned, &err)
	err = callback(ctx, scope)
	returned = true
	return wrapDependencyError("start callback", err)
}

func invokeHandlerFactory(callback HandlerFactory, ctx context.Context, resolver Resolver) (handler action.Handler, err error) {
	returned := false
	defer recoverCallback("handler factory", &returned, &err)
	handler, err = callback(ctx, resolver)
	returned = true
	if err != nil {
		return nil, wrapDependencyError("handler factory callback", err)
	}
	return handler, nil
}

func invokeCleanup(callback Cleanup, ctx context.Context) (err error) {
	returned := false
	defer recoverCallback("cleanup", &returned, &err)
	err = callback(ctx)
	returned = true
	return wrapDependencyError("cleanup callback", err)
}

func wrapDependencyError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &dependencyError{operation: operation, cause: err}
}

func invokeCleanupBounded(callback Cleanup, parent context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- invokeCleanup(callback, ctx)
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		select {
		case err := <-result:
			return err
		default:
			return ctx.Err()
		}
	}
}

func recoverCallback(callback string, returned *bool, err *error) {
	if returned == nil || !*returned {
		_ = recover()
		*err = &CallbackPanicError{Callback: callback}
	}
}
