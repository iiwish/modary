package projecttool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/module"
)

// ModuleInfo is the deterministic callback-free projection of one Module.
type ModuleInfo struct {
	ID         string              `json:"id"`
	Version    string              `json:"version"`
	Type       module.ModuleType   `json:"type"`
	Requires   []module.Capability `json:"requires,omitempty"`
	Provides   []module.Capability `json:"provides,omitempty"`
	Migrations []string            `json:"migrations,omitempty"`
}

// Snapshot is a defensive, deterministic projection of the pure consumer
// Definition. It contains no lifecycle callback, handler, migration FS, or
// service resolver.
type Snapshot struct {
	Application appkit.Metadata       `json:"application"`
	Modules     []ModuleInfo          `json:"modules"`
	Graph       module.Graph          `json:"graph"`
	Actions     []action.CatalogEntry `json:"actions"`
}

// Inspect validates static Module registrations, migration declarations,
// capabilities, dependency graph, Action ownership, and Action contracts. It
// never starts Modules or invokes a consumer callback.
func Inspect(definition appkit.Definition) (Snapshot, error) {
	return InspectContext(context.Background(), definition)
}

// InspectContext is the cancelable form of Inspect. Cancellation is observed
// between each registration and contract canonicalization boundary.
func InspectContext(ctx context.Context, definition appkit.Definition) (Snapshot, error) {
	if ctx == nil {
		return Snapshot{}, ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := appkit.ValidateMetadata(definition.Metadata); err != nil {
		return Snapshot{}, err
	}
	if len(definition.Modules) == 0 {
		return Snapshot{}, fmt.Errorf("application Definition must contain at least one Module")
	}
	if err := preScanDuplicateIdentities(ctx, definition.Modules); err != nil {
		return Snapshot{}, err
	}
	registrations, err := canonicalRegistrationsContext(ctx, definition.Modules)
	if err != nil {
		return Snapshot{}, err
	}
	host, err := module.NewHostWithOptions(module.HostOptions{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("create pure Module inspector: %w", err)
	}
	for _, registration := range registrations {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		if err := host.Register(registration); err != nil {
			return Snapshot{}, fmt.Errorf("inspect Module registrations: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	catalog, err := host.Catalog()
	if err != nil {
		return Snapshot{}, fmt.Errorf("inspect Module graph and Action catalog: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	manifests := make([]module.Manifest, 0, len(registrations))
	modules := make([]ModuleInfo, 0, len(registrations))
	for _, registration := range registrations {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		manifest := registration.Definition.Manifest
		manifests = append(manifests, manifest)
		requires := append([]module.Capability(nil), manifest.Requires...)
		provides := append([]module.Capability(nil), manifest.Provides...)
		slices.Sort(requires)
		slices.Sort(provides)
		migrations := make([]string, 0, len(registration.Definition.Migrations))
		for _, migration := range registration.Definition.Migrations {
			if err := ctx.Err(); err != nil {
				return Snapshot{}, err
			}
			migrations = append(migrations, migration.Driver)
		}
		sort.Strings(migrations)
		modules = append(modules, ModuleInfo{
			ID: manifest.ID, Version: manifest.Version, Type: manifest.Type,
			Requires: requires, Provides: provides, Migrations: migrations,
		})
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	graph, err := module.Verify(manifests)
	if err != nil {
		return Snapshot{}, fmt.Errorf("inspect Module graph: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].ID < modules[j].ID })
	for index := range catalog {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		catalog[index].Descriptor.InputSchema, err = canonicalJSON(catalog[index].Descriptor.InputSchema)
		if err != nil {
			return Snapshot{}, fmt.Errorf("canonicalize Action %s input schema: %w", catalog[index].Descriptor.ID, err)
		}
		if len(catalog[index].Descriptor.PreviewSchema) != 0 {
			catalog[index].Descriptor.PreviewSchema, err = canonicalJSON(catalog[index].Descriptor.PreviewSchema)
			if err != nil {
				return Snapshot{}, fmt.Errorf("canonicalize Action %s preview schema: %w", catalog[index].Descriptor.ID, err)
			}
		}
		catalog[index].Descriptor.OutputSchema, err = canonicalJSON(catalog[index].Descriptor.OutputSchema)
		if err != nil {
			return Snapshot{}, fmt.Errorf("canonicalize Action %s output schema: %w", catalog[index].Descriptor.ID, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Application: definition.Metadata,
		Modules:     modules,
		Graph:       cloneGraph(graph),
		Actions:     cloneCatalog(catalog),
	}, nil
}

func canonicalRegistrations(source []module.Registration) []module.Registration {
	registrations, _ := canonicalRegistrationsContext(context.Background(), source)
	return registrations
}

func canonicalRegistrationsContext(ctx context.Context, source []module.Registration) ([]module.Registration, error) {
	registrations := make([]module.Registration, len(source))
	for index, registration := range source {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		clone := registration
		clone.Definition.Manifest.Requires = append([]module.Capability(nil), registration.Definition.Manifest.Requires...)
		clone.Definition.Manifest.Provides = append([]module.Capability(nil), registration.Definition.Manifest.Provides...)
		slices.Sort(clone.Definition.Manifest.Requires)
		slices.Sort(clone.Definition.Manifest.Provides)
		clone.Definition.Actions = make([]module.ActionBinding, len(registration.Definition.Actions))
		for actionIndex, binding := range registration.Definition.Actions {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			clone.Definition.Actions[actionIndex] = binding
			clone.Definition.Actions[actionIndex].Descriptor.InputSchema = append(json.RawMessage(nil), binding.Descriptor.InputSchema...)
			clone.Definition.Actions[actionIndex].Descriptor.PreviewSchema = append(json.RawMessage(nil), binding.Descriptor.PreviewSchema...)
			clone.Definition.Actions[actionIndex].Descriptor.OutputSchema = append(json.RawMessage(nil), binding.Descriptor.OutputSchema...)
			clone.Definition.Actions[actionIndex].Descriptor.Channels = append([]action.Channel(nil), binding.Descriptor.Channels...)
			sort.Slice(clone.Definition.Actions[actionIndex].Descriptor.Channels, func(i, j int) bool {
				return clone.Definition.Actions[actionIndex].Descriptor.Channels[i] <
					clone.Definition.Actions[actionIndex].Descriptor.Channels[j]
			})
			clone.Definition.Actions[actionIndex].Descriptor.Errors = append([]action.ErrorSpec(nil), binding.Descriptor.Errors...)
			sort.Slice(clone.Definition.Actions[actionIndex].Descriptor.Errors, func(i, j int) bool {
				return clone.Definition.Actions[actionIndex].Descriptor.Errors[i].Code < clone.Definition.Actions[actionIndex].Descriptor.Errors[j].Code
			})
		}
		sort.SliceStable(clone.Definition.Actions, func(i, j int) bool {
			first := clone.Definition.Actions[i].Descriptor
			second := clone.Definition.Actions[j].Descriptor
			if first.ID == second.ID {
				return first.Version < second.Version
			}
			return first.ID < second.ID
		})
		clone.Definition.Migrations = append([]module.MigrationSource(nil), registration.Definition.Migrations...)
		sort.SliceStable(clone.Definition.Migrations, func(i, j int) bool {
			return clone.Definition.Migrations[i].Driver < clone.Definition.Migrations[j].Driver
		})
		registrations[index] = clone
	}
	sort.SliceStable(registrations, func(i, j int) bool {
		first := registrations[i].Definition.Manifest
		second := registrations[j].Definition.Manifest
		if first.ID != second.ID {
			return first.ID < second.ID
		}
		if first.Version != second.Version {
			return first.Version < second.Version
		}
		return first.Type < second.Type
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return registrations, nil
}

type compositionIdentity struct {
	owner string
	value string
}

func preScanDuplicateIdentities(ctx context.Context, registrations []module.Registration) error {
	moduleCounts := make(map[string]int, len(registrations))
	actionCounts := make(map[compositionIdentity]int)
	migrationCounts := make(map[compositionIdentity]int)
	for _, registration := range registrations {
		if err := ctx.Err(); err != nil {
			return err
		}
		owner := registration.Definition.Manifest.ID
		moduleCounts[owner]++
		for _, binding := range registration.Definition.Actions {
			if err := ctx.Err(); err != nil {
				return err
			}
			actionCounts[compositionIdentity{owner: owner, value: binding.Descriptor.ID}]++
		}
		for _, migration := range registration.Definition.Migrations {
			if err := ctx.Err(); err != nil {
				return err
			}
			migrationCounts[compositionIdentity{owner: owner, value: migration.Driver}]++
		}
	}
	if duplicate, ok := firstDuplicateString(moduleCounts); ok {
		return fmt.Errorf("duplicate Module identity %q", duplicate)
	}
	if duplicate, ok := firstDuplicateCompositionIdentity(actionCounts); ok {
		return fmt.Errorf("duplicate Action identity %q owned by Module %q", duplicate.value, duplicate.owner)
	}
	if duplicate, ok := firstDuplicateCompositionIdentity(migrationCounts); ok {
		return fmt.Errorf("duplicate migration identity %q owned by Module %q", duplicate.value, duplicate.owner)
	}
	return ctx.Err()
}

func firstDuplicateString(counts map[string]int) (string, bool) {
	values := make([]string, 0)
	for value, count := range counts {
		if count > 1 {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	if len(values) == 0 {
		return "", false
	}
	return values[0], true
}

func firstDuplicateCompositionIdentity(counts map[compositionIdentity]int) (compositionIdentity, bool) {
	values := make([]compositionIdentity, 0)
	for value, count := range counts {
		if count > 1 {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].owner != values[j].owner {
			return values[i].owner < values[j].owner
		}
		return values[i].value < values[j].value
	})
	if len(values) == 0 {
		return compositionIdentity{}, false
	}
	return values[0], true
}

func canonicalJSON(data json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func cloneGraph(graph module.Graph) module.Graph {
	clone := graph
	clone.Modules = append([]string(nil), graph.Modules...)
	clone.Edges = append([]module.GraphEdge(nil), graph.Edges...)
	clone.Order = append([]string(nil), graph.Order...)
	clone.Provides = make(map[module.Capability]string, len(graph.Provides))
	for capability, owner := range graph.Provides {
		clone.Provides[capability] = owner
	}
	return clone
}

func cloneCatalog(catalog []action.CatalogEntry) []action.CatalogEntry {
	clone := make([]action.CatalogEntry, len(catalog))
	for index, entry := range catalog {
		clone[index] = entry
		clone[index].Descriptor.InputSchema = append(json.RawMessage(nil), entry.Descriptor.InputSchema...)
		clone[index].Descriptor.PreviewSchema = append(json.RawMessage(nil), entry.Descriptor.PreviewSchema...)
		clone[index].Descriptor.OutputSchema = append(json.RawMessage(nil), entry.Descriptor.OutputSchema...)
		clone[index].Descriptor.Channels = append([]action.Channel(nil), entry.Descriptor.Channels...)
		clone[index].Descriptor.Errors = append([]action.ErrorSpec(nil), entry.Descriptor.Errors...)
	}
	return clone
}

// Verify validates the manifest/Definition identity and renders every expected
// artifact in memory, but performs no write.
func (project *Project) Verify(definition appkit.Definition) (Snapshot, error) {
	return project.VerifyContext(context.Background(), definition)
}

// VerifyContext is the cancelable form of Verify.
func (project *Project) VerifyContext(ctx context.Context, definition appkit.Definition) (Snapshot, error) {
	if ctx == nil {
		return Snapshot{}, ErrContextRequired
	}
	root, err := project.openVerifiedRoot(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer root.Close()
	snapshot, _, err := project.prepare(ctx, root, definition)
	if err != nil {
		return Snapshot{}, err
	}
	if err := project.verifyRootPathBinding(root); err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, err
}
