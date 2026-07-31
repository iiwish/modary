package jsonschema

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Schema documents retain the public Action JSON envelope. These additional
// limits bound executable schema structure, not source JSON validity.
const (
	MaxSchemaNodes              = 2_048
	MaxSchemaCollectionEntries  = 512
	MaxSchemaEnumValues         = 256
	MaxSchemaLiteralBytes       = 16 << 10
	MaxSchemaPatternBytes       = 4 << 10
	MaxSameInstanceSchemaVisits = 1_024

	MaxSchemaNumericCompileWorkUnits = 64 << 20
	MaxEvaluationWorkUnits           = 64 << 20
	MaxGeneratedDiagnostics          = 4_096

	// Metaschema admission shares one budget across the root and every hidden
	// reference root. It includes the full numeric compile allowance plus a
	// separate bounded amount for Draft 7 structural validation.
	maxMetaschemaValidationWorkUnits = 128 << 20

	// These are clone/cycle guards, not public source limits. The value-node
	// guard also covers a protocol-only copy of hidden local-reference targets.
	maxFrozenValueNodes     = 262_144
	maxFrozenContainerDepth = 1_024
	maxSchemaNestingDepth   = 512
)

const canonicalDraft7URI = "http://json-schema.org/draft-07/schema#"

type valueIdentity struct {
	kind reflect.Kind
	ptr  uintptr
}

// CompileLimits bounds executable schema structure. It does not change the
// per-validation evaluation budget.
type CompileLimits struct {
	MaxSchemaNodes              int
	MaxSchemaCollectionEntries  int
	MaxSchemaEnumValues         int
	MaxSchemaLiteralBytes       int
	MaxSchemaPatternBytes       int
	MaxSameInstanceSchemaVisits int
	MaxNumericCompileWorkUnits  int
}

// DefaultCompileLimits returns the Action schema compilation profile.
func DefaultCompileLimits() CompileLimits {
	return CompileLimits{
		MaxSchemaNodes:              MaxSchemaNodes,
		MaxSchemaCollectionEntries:  MaxSchemaCollectionEntries,
		MaxSchemaEnumValues:         MaxSchemaEnumValues,
		MaxSchemaLiteralBytes:       MaxSchemaLiteralBytes,
		MaxSchemaPatternBytes:       MaxSchemaPatternBytes,
		MaxSameInstanceSchemaVisits: MaxSameInstanceSchemaVisits,
		MaxNumericCompileWorkUnits:  MaxSchemaNumericCompileWorkUnits,
	}
}

func (limits CompileLimits) validate() error {
	if limits.MaxSchemaNodes <= 0 ||
		limits.MaxSchemaCollectionEntries <= 0 ||
		limits.MaxSchemaEnumValues <= 0 ||
		limits.MaxSchemaLiteralBytes <= 0 ||
		limits.MaxSchemaPatternBytes <= 0 ||
		limits.MaxSameInstanceSchemaVisits <= 0 ||
		limits.MaxNumericCompileWorkUnits <= 0 {
		return fmt.Errorf("JSON Schema compile limits must be positive")
	}
	return nil
}

// Freeze returns an immutable, profiled clone of document.
func Freeze(document any) (any, error) {
	return FreezeWithLimits(document, DefaultCompileLimits())
}

// FreezeWithLimits returns an immutable clone after building the complete
// executable schema graph with the supplied limits.
func FreezeWithLimits(document any, limits CompileLimits) (any, error) {
	graph, err := PrepareWithLimits(document, limits)
	if err != nil {
		return nil, err
	}
	return graph.RootClone(), nil
}

func cloneJSON(value any) (any, error) {
	active := make(map[valueIdentity]struct{})
	nodes := 0
	var clone func(any, int) (any, error)
	clone = func(current any, containerDepth int) (any, error) {
		nodes++
		if nodes > maxFrozenValueNodes {
			return nil, fmt.Errorf("document exceeds value node limit (%d)", maxFrozenValueNodes)
		}
		switch typed := current.(type) {
		case nil, bool, string:
			return typed, nil
		case json.Number:
			if !validJSONNumberToken(string(typed)) {
				return nil, fmt.Errorf("document contains invalid JSON number %q", typed)
			}
			return typed, nil
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) {
				return nil, fmt.Errorf("document contains non-JSON number %v", typed)
			}
			return json.Number(strconv.FormatFloat(typed, 'g', -1, 64)), nil
		case float32:
			value := float64(typed)
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("document contains non-JSON number %v", typed)
			}
			return json.Number(strconv.FormatFloat(value, 'g', -1, 32)), nil
		case int:
			return json.Number(strconv.FormatInt(int64(typed), 10)), nil
		case int8:
			return json.Number(strconv.FormatInt(int64(typed), 10)), nil
		case int16:
			return json.Number(strconv.FormatInt(int64(typed), 10)), nil
		case int32:
			return json.Number(strconv.FormatInt(int64(typed), 10)), nil
		case int64:
			return json.Number(strconv.FormatInt(typed, 10)), nil
		case uint:
			return json.Number(strconv.FormatUint(uint64(typed), 10)), nil
		case uint8:
			return json.Number(strconv.FormatUint(uint64(typed), 10)), nil
		case uint16:
			return json.Number(strconv.FormatUint(uint64(typed), 10)), nil
		case uint32:
			return json.Number(strconv.FormatUint(uint64(typed), 10)), nil
		case uint64:
			return json.Number(strconv.FormatUint(typed, 10)), nil
		case map[string]any:
			nextDepth := containerDepth + 1
			if nextDepth > maxFrozenContainerDepth {
				return nil, fmt.Errorf("document exceeds internal container depth limit (%d)", maxFrozenContainerDepth)
			}
			identity := identityOf(typed)
			if _, cyclic := active[identity]; cyclic {
				return nil, fmt.Errorf("document contains a cyclic object")
			}
			active[identity] = struct{}{}
			defer delete(active, identity)
			result := make(map[string]any, len(typed))
			for _, key := range sortedKeys(typed) {
				child, err := clone(typed[key], nextDepth)
				if err != nil {
					return nil, err
				}
				result[key] = child
			}
			return result, nil
		case []any:
			nextDepth := containerDepth + 1
			if nextDepth > maxFrozenContainerDepth {
				return nil, fmt.Errorf("document exceeds internal container depth limit (%d)", maxFrozenContainerDepth)
			}
			identity := identityOf(typed)
			if _, cyclic := active[identity]; cyclic {
				return nil, fmt.Errorf("document contains a cyclic array")
			}
			active[identity] = struct{}{}
			defer delete(active, identity)
			result := make([]any, len(typed))
			for index, nested := range typed {
				child, err := clone(nested, nextDepth)
				if err != nil {
					return nil, err
				}
				result[index] = child
			}
			return result, nil
		case []string:
			result := make([]any, len(typed))
			for index, nested := range typed {
				result[index] = nested
			}
			return clone(result, containerDepth)
		default:
			return nil, fmt.Errorf("document contains non-JSON value of type %T", current)
		}
	}
	return clone(value, 0)
}

func identityOf(value any) valueIdentity {
	reflected := reflect.ValueOf(value)
	return valueIdentity{kind: reflected.Kind(), ptr: reflected.Pointer()}
}

func schemaLimitError(name string, limit int) error {
	return fmt.Errorf("JSON Schema exceeds %s limit (%d)", name, limit)
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isSchemaValue(value any) bool {
	switch value.(type) {
	case bool, map[string]any:
		return true
	default:
		return false
	}
}

func encodedJSONSize(value any, limit int) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		return limit + 1
	}
	return len(encoded)
}

func numberToken(value any) (string, bool) {
	switch typed := value.(type) {
	case json.Number:
		return string(typed), true
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32), true
	case int:
		return strconv.FormatInt(int64(typed), 10), true
	case int8:
		return strconv.FormatInt(int64(typed), 10), true
	case int16:
		return strconv.FormatInt(int64(typed), 10), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case uint:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint8:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint16:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	default:
		return "", false
	}
}

func validJSONNumberToken(token string) bool {
	if token == "" {
		return false
	}
	index := 0
	if token[index] == '-' {
		index++
		if index == len(token) {
			return false
		}
	}
	if token[index] == '0' {
		index++
	} else {
		if token[index] < '1' || token[index] > '9' {
			return false
		}
		for index < len(token) && token[index] >= '0' && token[index] <= '9' {
			index++
		}
	}
	if index < len(token) && token[index] == '.' {
		index++
		start := index
		for index < len(token) && token[index] >= '0' && token[index] <= '9' {
			index++
		}
		if index == start {
			return false
		}
	}
	if index < len(token) && (token[index] == 'e' || token[index] == 'E') {
		index++
		if index < len(token) && (token[index] == '+' || token[index] == '-') {
			index++
		}
		start := index
		for index < len(token) && token[index] >= '0' && token[index] <= '9' {
			index++
		}
		if index == start {
			return false
		}
	}
	return index == len(token)
}

func numericCompileWork(token string) uint64 {
	tokenBytes := uint64(len(token))
	work := saturatingAdd(1, saturatingMultiply(tokenBytes, tokenBytes))
	if index := strings.IndexAny(token, "eE"); index >= 0 {
		exponent := token[index+1:]
		if strings.HasPrefix(exponent, "-") || strings.HasPrefix(exponent, "+") {
			exponent = exponent[1:]
		}
		magnitude, err := strconv.ParseUint(exponent, 10, 64)
		if err != nil {
			return ^uint64(0)
		}
		work = saturatingAdd(work, saturatingMultiply(magnitude, magnitude))
	}
	return work
}

func saturatingAdd(left, right uint64) uint64 {
	if ^uint64(0)-left < right {
		return ^uint64(0)
	}
	return left + right
}

func saturatingMultiply(left, right uint64) uint64 {
	if left == 0 || right == 0 {
		return 0
	}
	if left > ^uint64(0)/right {
		return ^uint64(0)
	}
	return left * right
}
