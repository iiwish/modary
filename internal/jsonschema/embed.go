package jsonschema

import (
	"fmt"
	"sort"
	"strconv"
)

// Rebase embeds the prepared root at pointer. Local references to structural
// schema locations are rebased directly. Targets that originated inside
// literal or unknown-keyword data are copied into one collision-free unknown
// keyword on the embedded root, leaving the original literal byte semantics
// untouched.
func (graph *SchemaGraph) Rebase(pointer, annotationBase string) (any, error) {
	if graph == nil || graph.root == nil || !graph.metaschemaValid {
		return nil, fmt.Errorf("JSON Schema graph is not initialized")
	}
	baseTokens, err := parseLocalPointer(pointer)
	if err != nil {
		return nil, fmt.Errorf("invalid embedding pointer %q: %w", pointer, err)
	}
	if annotationBase == "" {
		return nil, fmt.Errorf("embedding annotation name is empty")
	}
	root, err := cloneJSON(graph.root)
	if err != nil {
		return nil, fmt.Errorf("clone embedded JSON Schema: %w", err)
	}
	rootObject, objectRoot := root.(map[string]any)
	if !objectRoot {
		return root, nil
	}
	delete(rootObject, "$schema")

	owners, hiddenRoots := graph.hiddenOwners()
	annotationName := collisionFreeName(rootObject, annotationBase)
	entryNames := make(map[NodeID]string, len(hiddenRoots))
	for index, rootID := range hiddenRoots {
		entryNames[rootID] = "n" + strconv.Itoa(index)
	}

	targetTokens := func(target NodeID) ([]string, error) {
		node := graph.nodes[target]
		if node.primary {
			return appendToken(baseTokens, node.tokens...), nil
		}
		owner, exists := owners[target]
		if !exists {
			return nil, fmt.Errorf("hidden schema target %s has no embedding owner", node.pointer)
		}
		relative, ok := trimTokenPrefix(node.tokens, graph.nodes[owner].tokens)
		if !ok {
			return nil, fmt.Errorf("hidden schema target %s is outside owner %s", node.pointer, graph.nodes[owner].pointer)
		}
		result := appendToken(baseTokens, annotationName, entryNames[owner])
		return appendToken(result, relative...), nil
	}

	for id := range graph.nodes {
		node := graph.nodes[id]
		if !node.primary || node.ref == invalidNodeID {
			continue
		}
		value, err := valueAtTokens(root, node.tokens)
		if err != nil {
			return nil, err
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("prepared schema node %s is no longer an object", node.pointer)
		}
		tokens, err := targetTokens(node.ref)
		if err != nil {
			return nil, err
		}
		object["$ref"] = fragmentReference(tokens)
	}

	if len(hiddenRoots) == 0 {
		return root, nil
	}
	entries := make(map[string]any, len(hiddenRoots))
	for _, owner := range hiddenRoots {
		ownerNode := graph.nodes[owner]
		cloned, err := cloneJSON(ownerNode.value)
		if err != nil {
			return nil, fmt.Errorf("clone hidden schema target %s: %w", ownerNode.pointer, err)
		}
		for id := range graph.nodes {
			nodeID := NodeID(id)
			if owners[nodeID] != owner || graph.nodes[id].ref == invalidNodeID {
				continue
			}
			relative, ok := trimTokenPrefix(graph.nodes[id].tokens, ownerNode.tokens)
			if !ok {
				return nil, fmt.Errorf("hidden schema node %s is outside owner %s", graph.nodes[id].pointer, ownerNode.pointer)
			}
			value, err := valueAtTokens(cloned, relative)
			if err != nil {
				return nil, err
			}
			object, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("hidden schema node %s is no longer an object", graph.nodes[id].pointer)
			}
			tokens, err := targetTokens(graph.nodes[id].ref)
			if err != nil {
				return nil, err
			}
			object["$ref"] = fragmentReference(tokens)
		}
		entries[entryNames[owner]] = cloned
	}
	rootObject[annotationName] = entries
	return root, nil
}

// hiddenOwners groups lexically overlapping hidden targets under their
// shallowest target. The copied source subtrees are therefore disjoint and add
// at most one extra copy of any source JSON value.
func (graph *SchemaGraph) hiddenOwners() (map[NodeID]NodeID, []NodeID) {
	hidden := make([]NodeID, 0)
	for id := range graph.nodes {
		if !graph.nodes[id].primary {
			hidden = append(hidden, NodeID(id))
		}
	}
	sort.Slice(hidden, func(left, right int) bool {
		leftTokens := graph.nodes[hidden[left]].tokens
		rightTokens := graph.nodes[hidden[right]].tokens
		if len(leftTokens) != len(rightTokens) {
			return len(leftTokens) < len(rightTokens)
		}
		return graph.nodes[hidden[left]].pointer < graph.nodes[hidden[right]].pointer
	})
	owners := make(map[NodeID]NodeID, len(hidden))
	roots := make([]NodeID, 0)
	for _, id := range hidden {
		owner := invalidNodeID
		for _, candidate := range roots {
			if hasTokenPrefix(graph.nodes[id].tokens, graph.nodes[candidate].tokens) {
				owner = candidate
				break
			}
		}
		if owner == invalidNodeID {
			owner = id
			roots = append(roots, id)
		}
		owners[id] = owner
	}
	sort.Slice(roots, func(left, right int) bool {
		return graph.nodes[roots[left]].pointer < graph.nodes[roots[right]].pointer
	})
	return owners, roots
}

func collisionFreeName(object map[string]any, base string) string {
	if _, exists := object[base]; !exists {
		return base
	}
	for suffix := 1; ; suffix++ {
		candidate := base + "-" + strconv.Itoa(suffix)
		if _, exists := object[candidate]; !exists {
			return candidate
		}
	}
}

func valueAtTokens(root any, tokens []string) (any, error) {
	current := root
	for _, token := range tokens {
		switch typed := current.(type) {
		case map[string]any:
			nested, exists := typed[token]
			if !exists {
				return nil, fmt.Errorf("prepared JSON pointer token %q disappeared", token)
			}
			current = nested
		case []any:
			index, err := parseArrayPointerIndex(token, len(typed))
			if err != nil {
				return nil, err
			}
			current = typed[index]
		default:
			return nil, fmt.Errorf("prepared JSON pointer traverses non-container at %q", token)
		}
	}
	return current, nil
}

func hasTokenPrefix(tokens, prefix []string) bool {
	_, ok := trimTokenPrefix(tokens, prefix)
	return ok
}

func trimTokenPrefix(tokens, prefix []string) ([]string, bool) {
	if len(prefix) > len(tokens) {
		return nil, false
	}
	for index := range prefix {
		if tokens[index] != prefix[index] {
			return nil, false
		}
	}
	return append([]string(nil), tokens[len(prefix):]...), true
}
