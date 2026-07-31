package gojsonschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestValidateFlagEnforcesActualEvaluationResources(t *testing.T) {
	t.Run("work", func(t *testing.T) {
		schema := mustCompileSchema(t, map[string]any{
			"type":    "string",
			"pattern": strings.Repeat("a?", 128),
		})
		valid, err := schema.ValidateFlag(strings.Repeat("a", 128), Budget{
			MaxWorkUnits:   1_000,
			MaxDiagnostics: 100,
		})
		if valid || !errors.Is(err, ErrEvaluationLimit) {
			t.Fatalf("ValidateFlag() = %v, %v", valid, err)
		}
	})

	t.Run("diagnostics", func(t *testing.T) {
		schema := mustCompileSchema(t, map[string]any{"items": false})
		valid, err := schema.ValidateFlag([]any{true, true, true}, Budget{
			MaxWorkUnits:   1_000,
			MaxDiagnostics: 2,
		})
		if valid || !errors.Is(err, ErrEvaluationLimit) {
			t.Fatalf("ValidateFlag() = %v, %v", valid, err)
		}
	})

	t.Run("number", func(t *testing.T) {
		schema := mustCompileSchema(t, map[string]any{"type": "number"})
		valid, err := schema.ValidateFlag(json.Number(strings.Repeat("9", 64)), Budget{
			MaxWorkUnits:   1_000,
			MaxDiagnostics: 100,
		})
		if valid || !errors.Is(err, ErrEvaluationLimit) {
			t.Fatalf("ValidateFlag() = %v, %v", valid, err)
		}
	})
}

func TestValidateFlagEnforcesActiveFrameBoundary(t *testing.T) {
	const frames = 8
	document := interface{}(true)
	for index := 1; index < frames; index++ {
		document = map[string]any{"allOf": []any{document}}
	}
	schema := mustCompileSchema(t, document)
	exact := Budget{MaxWorkUnits: 1_000, MaxDiagnostics: 100, MaxFrames: frames}
	if valid, err := schema.ValidateFlag(nil, exact); err != nil || !valid {
		t.Fatalf("exact frame boundary = %v, %v", valid, err)
	}

	above := exact
	above.MaxFrames--
	valid, err := schema.ValidateFlag(nil, above)
	if valid || !errors.Is(err, ErrEvaluationLimit) {
		t.Fatalf("one above frame boundary = %v, %v", valid, err)
	}
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Resource != "active frames" || limit.Limit != frames-1 {
		t.Fatalf("frame limit error = %#v, %v", limit, err)
	}
}

func TestEvaluationFrameDefaultIsStable(t *testing.T) {
	budget, err := newEvaluationBudget(Budget{MaxWorkUnits: 1, MaxDiagnostics: 1})
	if err != nil {
		t.Fatal(err)
	}
	if budget.limits.MaxFrames != DefaultMaxEvaluationFrames {
		t.Fatalf("default max frames = %d, want %d", budget.limits.MaxFrames, DefaultMaxEvaluationFrames)
	}
}

func TestValidateManyFlagSharesOneBudget(t *testing.T) {
	schema := mustCompileSchema(t, true)
	roots := []interface{}{true, false}
	exact := Budget{MaxWorkUnits: 2, MaxDiagnostics: 10, MaxFrames: 1}
	if valid, err := schema.ValidateManyFlag(roots, exact); err != nil || !valid {
		t.Fatalf("exact shared budget = %v, %v", valid, err)
	}
	above := exact
	above.MaxWorkUnits--
	if valid, err := schema.ValidateManyFlag(roots, above); valid || !errors.Is(err, ErrEvaluationLimit) {
		t.Fatalf("one above shared budget = %v, %v", valid, err)
	}
}

func TestObjectKeyLookupsChargeBytesAndUsePropertyIndex(t *testing.T) {
	const keyBytes = 512
	schemaKey := strings.Repeat("x", keyBytes-1) + "a"
	inputKey := strings.Repeat("x", keyBytes-1) + "b"
	schema := mustCompileSchema(t, map[string]any{
		"properties": map[string]any{schemaKey: true},
	})
	if len(schema.rootSchema.properties) != 1 {
		t.Fatalf("compiled property index has %d entries", len(schema.rootSchema.properties))
	}
	value := map[string]any{inputKey: true}
	exactWork := uint64(2*keyBytes + 4)
	exact := Budget{MaxWorkUnits: exactWork, MaxDiagnostics: 10, MaxFrames: 10}
	if valid, err := schema.ValidateFlag(value, exact); err != nil || !valid {
		t.Fatalf("exact key-byte boundary = %v, %v", valid, err)
	}
	above := exact
	above.MaxWorkUnits--
	if valid, err := schema.ValidateFlag(value, above); valid || !errors.Is(err, ErrEvaluationLimit) {
		t.Fatalf("one above key-byte boundary = %v, %v", valid, err)
	}
}

func TestObjectPropertyIndexAvoidsCartesianKeyComparison(t *testing.T) {
	const (
		entries  = 128
		keyBytes = 2_048
	)
	prefix := strings.Repeat("k", keyBytes-4)
	properties := make(map[string]any, entries)
	value := make(map[string]any, entries)
	for index := 0; index < entries; index++ {
		properties[prefix+fmt.Sprintf("s%03d", index)] = true
		value[prefix+fmt.Sprintf("i%03d", index)] = true
	}
	schema := mustCompileSchema(t, map[string]any{"properties": properties})
	if len(schema.rootSchema.properties) != entries {
		t.Fatalf("compiled property index has %d entries, want %d", len(schema.rootSchema.properties), entries)
	}

	// One root visit, one scalar operation plus one key lookup per input
	// property, and one indexed lookup per declared property.
	exactWork := uint64(1 + entries*(keyBytes+2) + entries*(keyBytes+1))
	cartesianComparisonBytes := uint64(entries * entries * keyBytes)
	if exactWork >= cartesianComparisonBytes/32 {
		t.Fatalf("linear budget %d does not distinguish cartesian work %d", exactWork, cartesianComparisonBytes)
	}
	exact := Budget{MaxWorkUnits: exactWork, MaxDiagnostics: 10, MaxFrames: 1}
	if valid, err := schema.ValidateFlag(value, exact); err != nil || !valid {
		t.Fatalf("linear key budget = %v, %v", valid, err)
	}
	oneBelow := exact
	oneBelow.MaxWorkUnits--
	if valid, err := schema.ValidateFlag(value, oneBelow); valid || !errors.Is(err, ErrEvaluationLimit) {
		t.Fatalf("one below linear key budget = %v, %v", valid, err)
	}
}

func TestRequiredAndDependenciesChargeKeyBytes(t *testing.T) {
	longKey := strings.Repeat("k", 512)
	tests := map[string]struct {
		schema map[string]any
		value  map[string]any
	}{
		"required": {
			schema: map[string]any{"required": []any{longKey}},
			value:  map[string]any{},
		},
		"dependency trigger": {
			schema: map[string]any{"dependencies": map[string]any{longKey: []any{"short"}}},
			value:  map[string]any{},
		},
		"dependency target": {
			schema: map[string]any{"dependencies": map[string]any{"trigger": []any{longKey}}},
			value:  map[string]any{"trigger": true},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			schema := mustCompileSchema(t, test.schema)
			valid, err := schema.ValidateFlag(test.value, Budget{
				MaxWorkUnits:   256,
				MaxDiagnostics: 10,
				MaxFrames:      10,
			})
			if valid || !errors.Is(err, ErrEvaluationLimit) {
				t.Fatalf("long key validation = %v, %v", valid, err)
			}
		})
	}
}

func TestNumericSchemaOperandsConsumeWork(t *testing.T) {
	operand := json.Number(strings.Repeat("9", 256))
	tests := map[string]struct {
		document map[string]any
		work     func(*subSchema) uint64
		valid    bool
	}{
		"maximum": {
			document: map[string]any{"maximum": operand},
			work:     func(schema *subSchema) uint64 { return schema.maximumWork },
			valid:    true,
		},
		"multipleOf": {
			document: map[string]any{"multipleOf": operand},
			work:     func(schema *subSchema) uint64 { return schema.multipleOfWork },
			valid:    false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			schema := mustCompileSchema(t, test.document)
			operandWork := test.work(schema.rootSchema)
			if operandWork <= 1 {
				t.Fatalf("numeric operand work = %d", operandWork)
			}
			exactWork := saturatingAdd(3, operandWork)
			exact := Budget{MaxWorkUnits: exactWork, MaxDiagnostics: 10, MaxFrames: 10}
			valid, err := schema.ValidateFlag(json.Number("1"), exact)
			if err != nil || valid != test.valid {
				t.Fatalf("exact numeric operand boundary = %v, %v; want %v", valid, err, test.valid)
			}
			above := exact
			above.MaxWorkUnits--
			if valid, err := schema.ValidateFlag(json.Number("1"), above); valid || !errors.Is(err, ErrEvaluationLimit) {
				t.Fatalf("one above numeric operand boundary = %v, %v", valid, err)
			}
		})
	}
}

func TestValidateFlagKeepsExactNumericEquality(t *testing.T) {
	schema := mustCompileSchema(t, map[string]any{"const": json.Number("9007199254740993")})
	for _, test := range []struct {
		value json.Number
		valid bool
	}{
		{value: json.Number("9007199254740993"), valid: true},
		{value: json.Number("9007199254740993.0"), valid: true},
		{value: json.Number("9007199254740992"), valid: false},
	} {
		valid, err := schema.ValidateFlag(test.value, generousTestBudget())
		if err != nil || valid != test.valid {
			t.Fatalf("ValidateFlag(%s) = %v, %v; want %v", test.value, valid, err, test.valid)
		}
	}
}

func TestValidateFlagAllowsLegalWideArray(t *testing.T) {
	schema := mustCompileSchema(t, map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "boolean"},
	})
	value := make([]any, 32_000)
	for index := range value {
		value[index] = index%2 == 0
	}
	valid, err := schema.ValidateFlag(value, generousTestBudget())
	if err != nil || !valid {
		t.Fatalf("ValidateFlag() = %v, %v", valid, err)
	}
}

func TestValidateFlagIsConcurrencySafe(t *testing.T) {
	schema := mustCompileSchema(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "integer", "minimum": json.Number("0")},
		},
		"required": []any{"value"},
	})
	const workers = 64
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			valid, err := schema.ValidateFlag(
				map[string]any{"value": json.Number("1")},
				generousTestBudget(),
			)
			if err != nil {
				errs <- err
			} else if !valid {
				errs <- errors.New("valid value was rejected")
			}
		}(worker)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestValidateFlagContainsUnexpectedEnginePanic(t *testing.T) {
	valid, err := (&Schema{}).ValidateFlag(nil, generousTestBudget())
	if valid || err == nil || err.Error() != "JSON Schema validator panicked" {
		t.Fatalf("ValidateFlag() = %v, %v", valid, err)
	}
}

func mustCompileSchema(t *testing.T, document any) *Schema {
	t.Helper()
	schema, err := NewSchema(NewRawLoader(document))
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func generousTestBudget() Budget {
	return Budget{MaxWorkUnits: 64 << 20, MaxDiagnostics: 4_096}
}
