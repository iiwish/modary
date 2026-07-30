package module

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type Installer func(context.Context, *Host) error

type Registration struct {
	Manifest Manifest
	Install  Installer
}

type Host struct {
	mu            sync.RWMutex
	registrations map[string]Registration
	services      map[string]any
	started       []string
}

func NewHost() *Host {
	return &Host{
		registrations: make(map[string]Registration),
		services:      make(map[string]any),
	}
}

func (h *Host) Register(registrations ...Registration) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, registration := range registrations {
		if registration.Manifest.ID == "" {
			return fmt.Errorf("registered module has no id")
		}
		if _, exists := h.registrations[registration.Manifest.ID]; exists {
			return fmt.Errorf("module %s is already registered", registration.Manifest.ID)
		}
		h.registrations[registration.Manifest.ID] = registration
	}
	return nil
}

func (h *Host) Start(ctx context.Context) error {
	h.mu.RLock()
	manifests := make([]Manifest, 0, len(h.registrations))
	for _, registration := range h.registrations {
		manifests = append(manifests, registration.Manifest)
	}
	h.mu.RUnlock()
	graph, err := Verify(manifests)
	if err != nil {
		return err
	}
	for _, id := range graph.Order {
		h.mu.RLock()
		registration := h.registrations[id]
		h.mu.RUnlock()
		if registration.Install != nil {
			if err := registration.Install(ctx, h); err != nil {
				return fmt.Errorf("install module %s: %w", id, err)
			}
		}
		h.mu.Lock()
		h.started = append(h.started, id)
		h.mu.Unlock()
	}
	return nil
}

func (h *Host) Provide(name string, value any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.services[name]; exists {
		return fmt.Errorf("service %s is already provided", name)
	}
	h.services[name] = value
	return nil
}

func (h *Host) Replace(name string, value any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.services[name] = value
}

func (h *Host) Service(name string) (any, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	value, ok := h.services[name]
	return value, ok
}

func ServiceAs[T any](host *Host, name string) (T, error) {
	var zero T
	value, ok := host.Service(name)
	if !ok {
		return zero, fmt.Errorf("required service %s is not available", name)
	}
	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("service %s has type %T", name, value)
	}
	return typed, nil
}

func (h *Host) HasModule(id string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.registrations[id]
	return ok
}

func (h *Host) Manifests() []Manifest {
	h.mu.RLock()
	defer h.mu.RUnlock()
	manifests := make([]Manifest, 0, len(h.registrations))
	for _, registration := range h.registrations {
		manifests = append(manifests, registration.Manifest)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].ID < manifests[j].ID })
	return manifests
}

func (h *Host) StartedModules() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]string(nil), h.started...)
}
