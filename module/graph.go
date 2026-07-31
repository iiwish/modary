package module

import (
	"fmt"
	"sort"
	"strings"
)

// Graph is the deterministic result of resolving Module capability dependencies.
type Graph struct {
	Modules  []string              `json:"modules"`
	Edges    []GraphEdge           `json:"edges"`
	Order    []string              `json:"order"`
	Provides map[Capability]string `json:"provides"`
}

// GraphEdge connects a requiring Module to one capability provider.
type GraphEdge struct {
	From       string     `json:"from"`
	To         string     `json:"to"`
	Capability Capability `json:"capability"`
}

// Verify validates manifests and returns their acyclic capability graph.
func Verify(manifests []Manifest) (Graph, error) {
	byID := make(map[string]Manifest, len(manifests))
	providers := make(map[Capability]string)
	for _, manifest := range manifests {
		if err := ValidateManifest(manifest); err != nil {
			return Graph{}, err
		}
		if previous, exists := byID[manifest.ID]; exists {
			return Graph{}, fmt.Errorf("duplicate module id %q (%s and %s)", manifest.ID, previous.Version, manifest.Version)
		}
		byID[manifest.ID] = manifest
		for _, capability := range manifest.Provides {
			if owner, exists := providers[capability]; exists {
				return Graph{}, fmt.Errorf("capability %q has multiple providers: %s and %s", capability, owner, manifest.ID)
			}
			providers[capability] = manifest.ID
		}
	}

	adjacency := make(map[string][]GraphEdge, len(manifests))
	edges := make([]GraphEdge, 0)
	for _, manifest := range manifests {
		for _, requirement := range manifest.Requires {
			provider, ok := providers[requirement]
			if !ok {
				return Graph{}, fmt.Errorf("module %s requires missing capability %q", manifest.ID, requirement)
			}
			edge := GraphEdge{From: manifest.ID, To: provider, Capability: requirement}
			adjacency[manifest.ID] = append(adjacency[manifest.ID], edge)
			edges = append(edges, edge)
		}
	}

	order := make([]string, 0, len(manifests))
	state := make(map[string]uint8, len(manifests))
	stack := make([]string, 0, len(manifests))
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			start := 0
			for i, item := range stack {
				if item == id {
					start = i
					break
				}
			}
			cycle := append(append([]string{}, stack[start:]...), id)
			return fmt.Errorf("module dependency cycle: %s", strings.Join(cycle, " -> "))
		case 2:
			return nil
		}
		state[id] = 1
		stack = append(stack, id)
		deps := append([]GraphEdge(nil), adjacency[id]...)
		sort.Slice(deps, func(i, j int) bool {
			if deps[i].To == deps[j].To {
				return deps[i].Capability < deps[j].Capability
			}
			return deps[i].To < deps[j].To
		})
		for _, edge := range deps {
			if err := visit(edge.To); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
		order = append(order, id)
		return nil
	}

	moduleIDs := make([]string, 0, len(byID))
	for id := range byID {
		moduleIDs = append(moduleIDs, id)
	}
	sort.Strings(moduleIDs)
	for _, id := range moduleIDs {
		if err := visit(id); err != nil {
			return Graph{}, err
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			if edges[i].To == edges[j].To {
				return edges[i].Capability < edges[j].Capability
			}
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})
	return Graph{Modules: moduleIDs, Edges: edges, Order: order, Provides: providers}, nil
}
