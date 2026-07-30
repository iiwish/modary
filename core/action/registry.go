package action

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

type Registered struct {
	Descriptor Descriptor
	Handler    Handler
	ModuleID   string
}

type Registry struct {
	mu      sync.RWMutex
	actions map[string]Registered
}

func NewRegistry() *Registry {
	return &Registry{actions: make(map[string]Registered)}
}

func (r *Registry) Register(moduleID string, descriptor Descriptor, handler Handler) error {
	if descriptor.ID == "" {
		return fmt.Errorf("action id is required")
	}
	if descriptor.Permission == "" && descriptor.ID != "system.health" {
		return fmt.Errorf("action %s must declare a permission", descriptor.ID)
	}
	if descriptor.Preview != PreviewNone && descriptor.Preview != PreviewOptional && descriptor.Preview != PreviewRequired {
		return fmt.Errorf("action %s has invalid preview policy %q", descriptor.ID, descriptor.Preview)
	}
	if descriptor.AuditLevel != AuditMetadata && descriptor.AuditLevel != AuditDetailed {
		return fmt.Errorf("action %s must declare a valid audit level", descriptor.ID)
	}
	if len(descriptor.InputSchema) == 0 || !json.Valid(descriptor.InputSchema) {
		return fmt.Errorf("action %s must declare a valid input schema", descriptor.ID)
	}
	if len(descriptor.OutputSchema) == 0 || !json.Valid(descriptor.OutputSchema) {
		return fmt.Errorf("action %s must declare a valid output schema", descriptor.ID)
	}
	if len(descriptor.Channels) == 0 {
		return fmt.Errorf("action %s must declare at least one channel", descriptor.ID)
	}
	if handler == nil {
		return fmt.Errorf("action %s has no handler", descriptor.ID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.actions[descriptor.ID]; ok {
		return fmt.Errorf("action %s is already registered by module %s", descriptor.ID, existing.ModuleID)
	}
	r.actions[descriptor.ID] = Registered{Descriptor: descriptor, Handler: handler, ModuleID: moduleID}
	return nil
}

func (r *Registry) Resolve(id string) (Registered, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	registered, ok := r.actions[id]
	return registered, ok
}

func (r *Registry) List() []Registered {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Registered, 0, len(r.actions))
	for _, item := range r.actions {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Descriptor.ID < items[j].Descriptor.ID })
	return items
}
