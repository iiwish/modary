package jsonschema

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	engine "github.com/iiwish/modary/internal/jsonschema/engine"
)

// NodeID identifies one source location in a SchemaGraph. A location is
// admitted once, even when multiple local references reach it.
type NodeID uint32

const invalidNodeID = NodeID(math.MaxUint32)

type schemaNode struct {
	id       NodeID
	pointer  string
	tokens   []string
	value    any
	depth    int
	grammar  []NodeID
	ref      NodeID
	primary  bool
	same     []NodeID
	branches []NodeID
}

// SchemaGraph is an immutable Draft 7 schema document plus the unique closure
// of grammar children and local JSON Pointer reference targets.
type SchemaGraph struct {
	root            any
	limits          CompileLimits
	nodes           []schemaNode
	byPointer       map[string]NodeID
	grammarParent   map[NodeID]NodeID
	numericWork     uint64
	metaRoots       []any
	metaschemaValid bool
}

// Prepare clones and profiles a Draft 7 document without compiling it.
func Prepare(document any) (*SchemaGraph, error) {
	return PrepareWithLimits(document, DefaultCompileLimits())
}

// PrepareWithLimits builds one unique executable graph and applies all static
// framework policy before the engine sees the document.
func PrepareWithLimits(document any, limits CompileLimits) (*SchemaGraph, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	frozen, err := cloneJSON(document)
	if err != nil {
		return nil, fmt.Errorf("JSON Schema complexity: %w", err)
	}
	if !isSchemaValue(frozen) {
		return nil, fmt.Errorf("JSON Schema root must be an object or boolean")
	}

	graph := &SchemaGraph{
		root:          frozen,
		limits:        limits,
		byPointer:     make(map[string]NodeID),
		grammarParent: make(map[NodeID]NodeID),
	}
	if _, err := graph.addNode(frozen, nil, 1); err != nil {
		return nil, err
	}
	for index := 0; index < len(graph.nodes); index++ {
		if err := graph.inspectNode(NodeID(index)); err != nil {
			return nil, err
		}
	}
	graph.markPrimaryNodes()
	if graph.numericWork > uint64(limits.MaxNumericCompileWorkUnits) {
		return nil, schemaLimitError("numeric compile work", limits.MaxNumericCompileWorkUnits)
	}
	if err := graph.validateMetaschema(); err != nil {
		return nil, err
	}
	if err := graph.checkSameInstanceExpansion(); err != nil {
		return nil, err
	}
	return graph, nil
}

// RootClone returns a deep clone so the graph's source remains immutable.
func (graph *SchemaGraph) RootClone() any {
	if graph == nil {
		return nil
	}
	cloned, err := cloneJSON(graph.root)
	if err != nil {
		panic(fmt.Sprintf("clone prepared JSON Schema: %v", err))
	}
	return cloned
}

func (graph *SchemaGraph) addNode(value any, tokens []string, depth int) (NodeID, error) {
	if !isSchemaValue(value) {
		return invalidNodeID, fmt.Errorf("JSON Schema at %s must be an object or boolean", displayPointer(tokens))
	}
	if depth > maxSchemaNestingDepth {
		return invalidNodeID, schemaLimitError("schema nesting depth", maxSchemaNestingDepth)
	}
	pointer := pointerKey(tokens)
	if existing, ok := graph.byPointer[pointer]; ok {
		return existing, nil
	}
	if len(graph.nodes) >= graph.limits.MaxSchemaNodes {
		return invalidNodeID, schemaLimitError("schema nodes", graph.limits.MaxSchemaNodes)
	}
	id := NodeID(len(graph.nodes))
	copiedTokens := append([]string(nil), tokens...)
	graph.nodes = append(graph.nodes, schemaNode{
		id: id, pointer: pointer, tokens: copiedTokens, value: value, depth: depth, ref: invalidNodeID,
	})
	graph.byPointer[pointer] = id
	return id, nil
}

func (graph *SchemaGraph) addGrammarChild(parent NodeID, value any, tokens []string) (NodeID, error) {
	child, err := graph.addNode(value, tokens, graph.nodes[parent].depth+1)
	if err != nil {
		return invalidNodeID, err
	}
	graph.nodes[parent].grammar = appendUniqueNode(graph.nodes[parent].grammar, child)
	if _, exists := graph.grammarParent[child]; !exists {
		graph.grammarParent[child] = parent
	}
	return child, nil
}

func (graph *SchemaGraph) inspectNode(id NodeID) error {
	node := &graph.nodes[id]
	object, ok := node.value.(map[string]any)
	if !ok {
		return nil
	}
	path := displayPointer(node.tokens)

	for _, keyword := range []string{"id", "$id"} {
		if _, exists := object[keyword]; exists {
			return fmt.Errorf("JSON Schema %s contains prohibited %s; schema identifiers and non-local bases are unsupported", path, keyword)
		}
	}
	if raw, exists := object["$schema"]; exists {
		declaration, ok := raw.(string)
		if !ok || !isDraft7URI(declaration) {
			return fmt.Errorf("JSON Schema %s declares unsupported $schema %q; only JSON Schema Draft 7 is supported", path, raw)
		}
		object["$schema"] = canonicalDraft7URI
	}
	if raw, exists := object["$ref"]; exists {
		reference, ok := raw.(string)
		if !ok {
			return fmt.Errorf("JSON Schema %s $ref must be a string", path)
		}
		target, tokens, err := resolveLocalReference(graph.root, reference)
		if err != nil {
			if strings.Contains(err.Error(), "non-local") {
				return fmt.Errorf("JSON Schema %s contains non-local $ref %q", path, reference)
			}
			return fmt.Errorf("JSON Schema %s local reference %q: %w", path, reference, err)
		}
		targetID, err := graph.addNode(target, tokens, node.depth+1)
		if err != nil {
			return err
		}
		node = &graph.nodes[id]
		node.ref = targetID
	}

	if err := graph.inspectCollectionsAndLiterals(object); err != nil {
		return fmt.Errorf("JSON Schema %s: %w", path, err)
	}
	if err := graph.collectGrammarChildren(id, object); err != nil {
		return fmt.Errorf("JSON Schema %s: %w", path, err)
	}
	return nil
}

func (graph *SchemaGraph) collectGrammarChildren(id NodeID, object map[string]any) error {
	base := graph.nodes[id].tokens
	singles := []string{
		"additionalItems", "additionalProperties", "contains", "else",
		"if", "not", "propertyNames", "then",
	}
	for _, keyword := range singles {
		value, exists := object[keyword]
		if !exists {
			continue
		}
		if !isSchemaValue(value) {
			return fmt.Errorf("%s must be an object or boolean", keyword)
		}
		child, err := graph.addGrammarChild(id, value, appendToken(base, keyword))
		if err != nil {
			return err
		}
		switch keyword {
		case "not", "if":
			graph.nodes[id].same = appendUniqueNode(graph.nodes[id].same, child)
		case "then", "else":
			graph.nodes[id].branches = appendUniqueNode(graph.nodes[id].branches, child)
		}
	}

	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		value, exists := object[keyword]
		if !exists {
			continue
		}
		children, ok := value.([]any)
		if !ok || len(children) == 0 {
			return fmt.Errorf("%s must be a non-empty array of schemas", keyword)
		}
		if err := graph.requireCollectionLimit(keyword, len(children)); err != nil {
			return err
		}
		for index, childValue := range children {
			if !isSchemaValue(childValue) {
				return fmt.Errorf("%s/%d must be an object or boolean", keyword, index)
			}
			child, err := graph.addGrammarChild(id, childValue, appendToken(base, keyword, strconv.Itoa(index)))
			if err != nil {
				return err
			}
			graph.nodes[id].same = appendUniqueNode(graph.nodes[id].same, child)
		}
	}

	if value, exists := object["items"]; exists {
		switch typed := value.(type) {
		case []any:
			if len(typed) == 0 {
				return fmt.Errorf("items tuple must be non-empty")
			}
			if err := graph.requireCollectionLimit("tuple", len(typed)); err != nil {
				return err
			}
			for index, childValue := range typed {
				if !isSchemaValue(childValue) {
					return fmt.Errorf("items/%d must be an object or boolean", index)
				}
				if _, err := graph.addGrammarChild(id, childValue, appendToken(base, "items", strconv.Itoa(index))); err != nil {
					return err
				}
			}
		default:
			if !isSchemaValue(typed) {
				return fmt.Errorf("items must be a schema or non-empty array of schemas")
			}
			if _, err := graph.addGrammarChild(id, typed, appendToken(base, "items")); err != nil {
				return err
			}
		}
	}

	for _, keyword := range []string{"definitions", "properties", "patternProperties"} {
		value, exists := object[keyword]
		if !exists {
			continue
		}
		children, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object of schemas", keyword)
		}
		if err := graph.requireCollectionLimit(keyword, len(children)); err != nil {
			return err
		}
		for _, name := range sortedKeys(children) {
			childValue := children[name]
			if !isSchemaValue(childValue) {
				return fmt.Errorf("%s/%s must be an object or boolean", keyword, name)
			}
			if _, err := graph.addGrammarChild(id, childValue, appendToken(base, keyword, name)); err != nil {
				return err
			}
		}
	}

	if value, exists := object["dependencies"]; exists {
		dependencies, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("dependencies must be an object")
		}
		if err := graph.requireCollectionLimit("dependencies", len(dependencies)); err != nil {
			return err
		}
		for _, name := range sortedKeys(dependencies) {
			dependency := dependencies[name]
			if isSchemaValue(dependency) {
				child, err := graph.addGrammarChild(id, dependency, appendToken(base, "dependencies", name))
				if err != nil {
					return err
				}
				graph.nodes[id].same = appendUniqueNode(graph.nodes[id].same, child)
				continue
			}
			names, ok := dependency.([]any)
			if !ok {
				return fmt.Errorf("dependencies/%s must be a schema or string array", name)
			}
			if err := graph.requireCollectionLimit("dependency names", len(names)); err != nil {
				return err
			}
			if err := requireUniqueStrings(names, "dependency names"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (graph *SchemaGraph) inspectCollectionsAndLiterals(object map[string]any) error {
	if value, exists := object["enum"]; exists {
		values, ok := value.([]any)
		if !ok || len(values) == 0 {
			return fmt.Errorf("enum must be a non-empty array")
		}
		if len(values) > graph.limits.MaxSchemaEnumValues {
			return schemaLimitError("enum values", graph.limits.MaxSchemaEnumValues)
		}
		if encodedJSONSize(values, graph.limits.MaxSchemaLiteralBytes) > graph.limits.MaxSchemaLiteralBytes {
			return schemaLimitError("enum literal bytes", graph.limits.MaxSchemaLiteralBytes)
		}
		graph.addLiteralNumericWork(values)
	}
	if value, exists := object["const"]; exists {
		if encodedJSONSize(value, graph.limits.MaxSchemaLiteralBytes) > graph.limits.MaxSchemaLiteralBytes {
			return schemaLimitError("const literal bytes", graph.limits.MaxSchemaLiteralBytes)
		}
		graph.addLiteralNumericWork(value)
	}

	for _, keyword := range []string{"exclusiveMaximum", "exclusiveMinimum", "maximum", "minimum", "multipleOf"} {
		value, exists := object[keyword]
		if !exists {
			continue
		}
		token, ok := numberToken(value)
		if !ok {
			return fmt.Errorf("%s must be a number", keyword)
		}
		if keyword == "multipleOf" && !positiveNumber(token) {
			return fmt.Errorf("multipleOf must be greater than zero")
		}
		graph.numericWork = saturatingAdd(graph.numericWork, numericCompileWork(token))
	}
	for _, keyword := range []string{"maxItems", "maxLength", "maxProperties", "minItems", "minLength", "minProperties"} {
		if value, exists := object[keyword]; exists {
			token, numeric := numberToken(value)
			if !numeric || !nonNegativeInteger(value) {
				return fmt.Errorf("%s must be a non-negative integer", keyword)
			}
			graph.numericWork = saturatingAdd(graph.numericWork, numericCompileWork(token))
		}
	}
	if value, exists := object["uniqueItems"]; exists {
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("uniqueItems must be a boolean")
		}
	}
	if value, exists := object["readOnly"]; exists {
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("readOnly must be a boolean")
		}
	}
	for _, keyword := range []string{"$comment", "contentEncoding", "contentMediaType", "description", "format", "title"} {
		if value, exists := object[keyword]; exists {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s must be a string", keyword)
			}
		}
	}
	if value, exists := object["examples"]; exists {
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("examples must be an array")
		}
	}
	if value, exists := object["required"]; exists {
		names, ok := value.([]any)
		if !ok {
			return fmt.Errorf("required must be an array of unique strings")
		}
		if err := graph.requireCollectionLimit("required", len(names)); err != nil {
			return err
		}
		if err := requireUniqueStrings(names, "required"); err != nil {
			return err
		}
	}
	if value, exists := object["type"]; exists {
		if err := validateTypeKeyword(value, graph.limits.MaxSchemaCollectionEntries); err != nil {
			return err
		}
	}
	if value, exists := object["pattern"]; exists {
		pattern, ok := value.(string)
		if !ok {
			return fmt.Errorf("pattern must be a string")
		}
		if err := validatePattern(pattern, graph.limits.MaxSchemaPatternBytes); err != nil {
			return err
		}
	}
	if patterns, ok := object["patternProperties"].(map[string]any); ok {
		for _, pattern := range sortedKeys(patterns) {
			if err := validatePattern(pattern, graph.limits.MaxSchemaPatternBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

func (graph *SchemaGraph) addLiteralNumericWork(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for _, name := range sortedKeys(typed) {
			graph.addLiteralNumericWork(typed[name])
		}
	case []any:
		for _, nested := range typed {
			graph.addLiteralNumericWork(nested)
		}
	default:
		if token, ok := numberToken(value); ok {
			graph.numericWork = saturatingAdd(graph.numericWork, numericCompileWork(token))
		}
	}
}

func (graph *SchemaGraph) requireCollectionLimit(name string, length int) error {
	if length > graph.limits.MaxSchemaCollectionEntries {
		return schemaLimitError(name+" entries", graph.limits.MaxSchemaCollectionEntries)
	}
	return nil
}

func (graph *SchemaGraph) markPrimaryNodes() {
	if len(graph.nodes) == 0 {
		return
	}
	stack := []NodeID{0}
	for len(stack) > 0 {
		last := len(stack) - 1
		id := stack[last]
		stack = stack[:last]
		if graph.nodes[id].primary {
			continue
		}
		graph.nodes[id].primary = true
		stack = append(stack, graph.nodes[id].grammar...)
	}
	graph.metaRoots = []any{graph.root}
	for id := range graph.nodes {
		nodeID := NodeID(id)
		if graph.nodes[id].primary {
			continue
		}
		parent, hasParent := graph.grammarParent[nodeID]
		if hasParent && !graph.nodes[parent].primary {
			continue
		}
		graph.metaRoots = append(graph.metaRoots, graph.nodes[id].value)
	}
}

func (graph *SchemaGraph) validateMetaschema() error {
	meta, err := engine.NewDraft7MetaSchema()
	if err != nil {
		return fmt.Errorf("compile pinned Draft 7 metaschema: %w", err)
	}
	valid, err := meta.ValidateManyFlag(graph.metaRoots, engine.Budget{
		MaxWorkUnits:   maxMetaschemaValidationWorkUnits,
		MaxDiagnostics: MaxGeneratedDiagnostics,
		MaxFrames:      MaxEvaluationFrames,
	})
	if err != nil {
		return fmt.Errorf("validate Draft 7 metaschema: %w", err)
	}
	if !valid {
		return fmt.Errorf("JSON Schema does not satisfy the Draft 7 metaschema")
	}
	graph.metaschemaValid = true
	return nil
}

func (graph *SchemaGraph) checkSameInstanceExpansion() error {
	state := make([]uint8, len(graph.nodes))
	memo := make([]int, len(graph.nodes))
	var cost func(NodeID) (int, error)
	cost = func(id NodeID) (int, error) {
		if state[id] == 1 {
			return 0, fmt.Errorf("JSON Schema contains a zero-progress local $ref cycle")
		}
		if state[id] == 2 {
			return memo[id], nil
		}
		state[id] = 1
		node := graph.nodes[id]
		total := 1
		if node.ref != invalidNodeID {
			nested, err := cost(node.ref)
			if err != nil {
				return 0, err
			}
			total = boundedAdd(total, nested, graph.limits.MaxSameInstanceSchemaVisits)
		} else {
			for _, child := range node.same {
				nested, err := cost(child)
				if err != nil {
					return 0, err
				}
				total = boundedAdd(total, nested, graph.limits.MaxSameInstanceSchemaVisits)
			}
			// then/else are mutually exclusive and ignored unless if exists.
			if object, ok := node.value.(map[string]any); ok {
				if _, conditional := object["if"]; conditional {
					maxBranch := 0
					for _, child := range node.branches {
						nested, err := cost(child)
						if err != nil {
							return 0, err
						}
						if nested > maxBranch {
							maxBranch = nested
						}
					}
					total = boundedAdd(total, maxBranch, graph.limits.MaxSameInstanceSchemaVisits)
				}
			}
		}
		if total > graph.limits.MaxSameInstanceSchemaVisits {
			return 0, schemaLimitError("same-instance schema visits", graph.limits.MaxSameInstanceSchemaVisits)
		}
		state[id] = 2
		memo[id] = total
		return total, nil
	}
	for id := range graph.nodes {
		if _, err := cost(NodeID(id)); err != nil {
			return err
		}
	}
	return nil
}

func resolveLocalReference(document any, reference string) (any, []string, error) {
	tokens, err := parseLocalPointer(reference)
	if err != nil {
		return nil, nil, err
	}
	current := document
	for _, token := range tokens {
		switch typed := current.(type) {
		case map[string]any:
			value, ok := typed[token]
			if !ok {
				return nil, nil, fmt.Errorf("pointer token %q does not exist", token)
			}
			current = value
		case []any:
			position, err := parseArrayPointerIndex(token, len(typed))
			if err != nil {
				return nil, nil, err
			}
			current = typed[position]
		default:
			return nil, nil, fmt.Errorf("pointer traverses non-container at token %q", token)
		}
	}
	if !isSchemaValue(current) {
		return nil, nil, fmt.Errorf("target does not resolve to a schema")
	}
	return current, tokens, nil
}

func parseLocalPointer(reference string) ([]string, error) {
	if reference == "" || reference[0] != '#' {
		return nil, fmt.Errorf("non-local reference")
	}
	parsed, err := url.Parse(reference)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.Path != "" || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.User != nil || parsed.Opaque != "" {
		return nil, fmt.Errorf("non-local reference")
	}
	fragment := parsed.Fragment
	if fragment == "" {
		return nil, nil
	}
	if !strings.HasPrefix(fragment, "/") {
		return nil, fmt.Errorf("non-local reference: named fragments are unsupported")
	}
	rawTokens := strings.Split(strings.TrimPrefix(fragment, "/"), "/")
	tokens := make([]string, len(rawTokens))
	for index, rawToken := range rawTokens {
		token, err := unescapePointerToken(rawToken)
		if err != nil {
			return nil, err
		}
		tokens[index] = token
	}
	return tokens, nil
}

func unescapePointerToken(token string) (string, error) {
	var result strings.Builder
	result.Grow(len(token))
	for index := 0; index < len(token); index++ {
		if token[index] != '~' {
			result.WriteByte(token[index])
			continue
		}
		if index+1 >= len(token) {
			return "", fmt.Errorf("invalid JSON Pointer escape")
		}
		index++
		switch token[index] {
		case '0':
			result.WriteByte('~')
		case '1':
			result.WriteByte('/')
		default:
			return "", fmt.Errorf("invalid JSON Pointer escape")
		}
	}
	return result.String(), nil
}

func parseArrayPointerIndex(token string, length int) (int, error) {
	if token == "" || token == "-" || len(token) > 1 && token[0] == '0' {
		return 0, fmt.Errorf("invalid array index %q", token)
	}
	index, err := strconv.Atoi(token)
	if err != nil || index < 0 || index >= length {
		return 0, fmt.Errorf("array index %q is out of range", token)
	}
	return index, nil
}

func pointerKey(tokens []string) string {
	if len(tokens) == 0 {
		return "#"
	}
	var result strings.Builder
	result.WriteByte('#')
	for _, token := range tokens {
		result.WriteByte('/')
		result.WriteString(escapePointerToken(token))
	}
	return result.String()
}

func fragmentReference(tokens []string) string {
	if len(tokens) == 0 {
		return "#"
	}
	pointer := strings.TrimPrefix(pointerKey(tokens), "#")
	return (&url.URL{Fragment: pointer}).String()
}

func displayPointer(tokens []string) string {
	return pointerKey(tokens)
}

func escapePointerToken(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

func appendToken(base []string, tokens ...string) []string {
	result := make([]string, 0, len(base)+len(tokens))
	result = append(result, base...)
	result = append(result, tokens...)
	return result
}

func appendUniqueNode(nodes []NodeID, candidate NodeID) []NodeID {
	for _, node := range nodes {
		if node == candidate {
			return nodes
		}
	}
	return append(nodes, candidate)
}

func isDraft7URI(value string) bool {
	normalized := strings.TrimSuffix(value, "#")
	return normalized == "http://json-schema.org/draft-07/schema" ||
		normalized == "https://json-schema.org/draft-07/schema"
}

func validatePattern(pattern string, limit int) error {
	if len(pattern) > limit {
		return schemaLimitError("pattern bytes", limit)
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return fmt.Errorf("pattern is not a valid regular expression: %w", err)
	}
	return nil
}

func requireUniqueStrings(values []any, keyword string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must contain only strings", keyword)
		}
		if _, duplicate := seen[text]; duplicate {
			return fmt.Errorf("%s must contain unique strings", keyword)
		}
		seen[text] = struct{}{}
	}
	return nil
}

func validateTypeKeyword(value any, collectionLimit int) error {
	valid := func(name string) bool {
		switch name {
		case "array", "boolean", "integer", "null", "number", "object", "string":
			return true
		default:
			return false
		}
	}
	if name, ok := value.(string); ok {
		if !valid(name) {
			return fmt.Errorf("type contains unsupported JSON type %q", name)
		}
		return nil
	}
	names, ok := value.([]any)
	if !ok || len(names) == 0 {
		return fmt.Errorf("type must be a JSON type name or non-empty array of unique type names")
	}
	if len(names) > collectionLimit {
		return schemaLimitError("type entries", collectionLimit)
	}
	if err := requireUniqueStrings(names, "type"); err != nil {
		return err
	}
	for _, value := range names {
		if !valid(value.(string)) {
			return fmt.Errorf("type contains unsupported JSON type %q", value)
		}
	}
	return nil
}

func nonNegativeInteger(value any) bool {
	token, ok := numberToken(value)
	if !ok || token == "" {
		return false
	}
	negative := strings.HasPrefix(token, "-")
	if negative {
		token = token[1:]
	}
	exponent := int64(0)
	if index := strings.IndexAny(token, "eE"); index >= 0 {
		rawExponent := token[index+1:]
		token = token[:index]
		parsed, err := strconv.ParseInt(rawExponent, 10, 64)
		if err != nil {
			zero := onlyZeroDigits(token)
			if negative && !zero {
				return false
			}
			return strings.HasPrefix(rawExponent, "+") || rawExponent[0] != '-'
		}
		exponent = parsed
	}
	fractionDigits := 0
	if dot := strings.IndexByte(token, '.'); dot >= 0 {
		fractionDigits = len(token) - dot - 1
		token = token[:dot] + token[dot+1:]
	}
	zero := onlyZeroDigits(token)
	if negative && !zero {
		return false
	}
	if zero || exponent >= int64(fractionDigits) {
		return true
	}
	unshifted := int64(fractionDigits) - exponent
	if unshifted > int64(len(token)) {
		return false
	}
	return onlyZeroDigits(token[len(token)-int(unshifted):])
}

func positiveNumber(token string) bool {
	if token == "" || strings.HasPrefix(token, "-") {
		return false
	}
	// JSON number tokens cannot spell NaN or infinity. Zero has only zero
	// digits in its significand, regardless of decimal point or exponent.
	significand := token
	if index := strings.IndexAny(significand, "eE"); index >= 0 {
		significand = significand[:index]
	}
	for _, character := range significand {
		if character >= '1' && character <= '9' {
			return true
		}
	}
	return false
}

func onlyZeroDigits(token string) bool {
	if token == "" {
		return false
	}
	for _, character := range token {
		switch character {
		case '0', '.':
		default:
			return false
		}
	}
	return true
}

func boundedAdd(left, right, limit int) int {
	if left > limit || right > limit-left {
		return limit + 1
	}
	return left + right
}
