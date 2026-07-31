package actionruntime

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/iiwish/modary/action"
)

type registration struct {
	Descriptor   action.Descriptor
	Handler      action.Handler
	ModuleID     string
	ContractHash string
	prepared     action.PreparedDescriptor
}

func (registered registration) validateInput(value []byte) error {
	return registered.prepared.ValidateInput(value)
}

func (registered registration) validatePreview(value []byte) error {
	return registered.prepared.ValidatePreview(value)
}

func (registered registration) validateOutput(value []byte) error {
	return registered.prepared.ValidateOutput(value)
}

// Registry owns mutable Handler bindings exclusively inside framework
// assembly. Callers outside this internal package cannot construct or mutate it.
type Registry struct {
	mu          sync.RWMutex
	actions     map[string]registration
	leases      map[uint64]context.CancelFunc
	nextLeaseID uint64
	revoked     bool
	drained     chan struct{}
	drainClosed bool
}

// NewRegistry constructs an empty internal Action registry.
func NewRegistry() *Registry {
	return &Registry{
		actions: make(map[string]registration),
		leases:  make(map[uint64]context.CancelFunc),
		drained: make(chan struct{}),
	}
}

// Register validates and binds one Action descriptor and Handler.
func (registry *Registry) Register(moduleID string, descriptor action.Descriptor, handler action.Handler) error {
	prepared, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		return err
	}
	return registry.BindPrepared(moduleID, prepared, handler)
}

// BindPrepared attaches runtime behavior to an already compiled contract.
func (registry *Registry) BindPrepared(moduleID string, prepared action.PreparedDescriptor, handler action.Handler) error {
	if registry == nil {
		return fmt.Errorf("action registry is required")
	}
	descriptor := prepared.Descriptor()
	if !prepared.Valid() {
		return fmt.Errorf("prepared Action descriptor is invalid")
	}
	if isNilValue(handler) {
		return fmt.Errorf("action %s has no handler", descriptor.ID)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.revoked {
		return fmt.Errorf("action registry is revoked")
	}
	if existing, ok := registry.actions[descriptor.ID]; ok {
		return fmt.Errorf("action %s is already registered by module %s", descriptor.ID, existing.ModuleID)
	}
	registry.actions[descriptor.ID] = registration{
		Descriptor: descriptor, Handler: handler, ModuleID: moduleID,
		ContractHash: prepared.ContractHash(), prepared: prepared,
	}
	return nil
}

// Revoke atomically rejects new execution, clears bindings, and cancels leases.
func (registry *Registry) Revoke() error {
	if registry == nil {
		return fmt.Errorf("action registry is required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if !registry.revoked {
		registry.revoked = true
		clear(registry.actions)
		for _, cancel := range registry.leases {
			cancel()
		}
		registry.closeDrainedLocked()
	}
	return nil
}

// Reset revokes execution and waits for every active lease to drain.
func (registry *Registry) Reset(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("reset context is required")
	}
	if err := registry.Revoke(); err != nil {
		return err
	}
	registry.mu.Lock()
	drained := registry.drained
	registry.mu.Unlock()
	select {
	case <-drained:
		return nil
	default:
	}
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		select {
		case <-drained:
			return nil
		default:
			return fmt.Errorf("drain active Action executions: %w", ctx.Err())
		}
	}
}

func (registry *Registry) resolve(id string) (registration, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	registered, ok := registry.actions[id]
	return registered, ok
}

func (registry *Registry) acquire(parent context.Context, id string) (registration, context.Context, func(), bool, bool) {
	executionCtx, cancel := context.WithCancel(parent)
	registry.mu.Lock()
	if registry.revoked {
		registry.mu.Unlock()
		cancel()
		return registration{}, nil, nil, false, true
	}
	registered, ok := registry.actions[id]
	leaseID := registry.registerLeaseLocked(cancel)
	registry.mu.Unlock()
	return registered, executionCtx, registry.releaseLease(leaseID, cancel), ok, false
}

func (registry *Registry) acquireLease(parent context.Context) (context.Context, func(), bool) {
	executionCtx, cancel := context.WithCancel(parent)
	registry.mu.Lock()
	if registry.revoked {
		registry.mu.Unlock()
		cancel()
		return nil, nil, true
	}
	leaseID := registry.registerLeaseLocked(cancel)
	registry.mu.Unlock()
	return executionCtx, registry.releaseLease(leaseID, cancel), false
}

func (registry *Registry) registerLeaseLocked(cancel context.CancelFunc) uint64 {
	for {
		registry.nextLeaseID++
		if _, exists := registry.leases[registry.nextLeaseID]; !exists {
			registry.leases[registry.nextLeaseID] = cancel
			return registry.nextLeaseID
		}
	}
}

func (registry *Registry) releaseLease(leaseID uint64, cancel context.CancelFunc) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			registry.mu.Lock()
			if _, active := registry.leases[leaseID]; active {
				delete(registry.leases, leaseID)
			}
			registry.closeDrainedLocked()
			registry.mu.Unlock()
		})
	}
}

func (registry *Registry) closeDrainedLocked() {
	if registry.revoked && len(registry.leases) == 0 && !registry.drainClosed {
		registry.drainClosed = true
		close(registry.drained)
	}
}

func (registry *Registry) available() bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return !registry.revoked
}

// Resolve returns one defensive read-only catalog entry without its Handler.
func (registry *Registry) Resolve(id string) (action.CatalogEntry, bool) {
	registered, ok := registry.resolve(id)
	if !ok {
		return action.CatalogEntry{}, false
	}
	return action.CatalogEntry{
		Descriptor: registered.prepared.Descriptor(), ModuleID: registered.ModuleID,
		ContractHash: registered.ContractHash,
	}, true
}

// List returns deterministic defensive read-only catalog entries.
func (registry *Registry) List() []action.CatalogEntry {
	registry.mu.RLock()
	entries := make([]action.CatalogEntry, 0, len(registry.actions))
	for _, registered := range registry.actions {
		entries = append(entries, action.CatalogEntry{
			Descriptor: registered.prepared.Descriptor(), ModuleID: registered.ModuleID,
			ContractHash: registered.ContractHash,
		})
	}
	registry.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Descriptor.ID < entries[j].Descriptor.ID })
	return entries
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
