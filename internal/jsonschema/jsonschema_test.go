package jsonschema

import "testing"

func TestCompiledSchemaValidatesWithoutExposingDependencyObjects(t *testing.T) {
	compiled, err := Compile(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
		"required":             []any{"value"},
		"additionalProperties": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := compiled.ValidateFlag(map[string]any{"value": "ok"}); err != nil || !valid {
		t.Fatalf("valid value = %v, %v", valid, err)
	}
	if valid, err := compiled.ValidateFlag(map[string]any{}); err != nil || valid {
		t.Fatalf("invalid value = %v, %v", valid, err)
	}
	var unavailable *Compiled
	if _, err := unavailable.ValidateFlag(nil); err == nil {
		t.Fatal("nil compiled schema validated a value")
	}
}

func TestCompileWithLimitsChangesOnlyStaticAdmissionProfile(t *testing.T) {
	document := map[string]any{"allOf": []any{map[string]any{}, map[string]any{}}}
	limits := DefaultCompileLimits()
	limits.MaxSchemaNodes = 2
	if _, err := CompileWithLimits(document, limits); err == nil {
		t.Fatal("schema above explicit node profile compiled")
	}
	limits.MaxSchemaNodes = 3
	compiled, err := CompileWithLimits(document, limits)
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := compiled.ValidateFlag(map[string]any{"value": true}); err != nil || !valid {
		t.Fatalf("ValidateFlag() = %v, %v", valid, err)
	}
}
