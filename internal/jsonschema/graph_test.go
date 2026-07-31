package jsonschema

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestPrepareAppliesDraft7MetaschemaToEveryExecutableRoot(t *testing.T) {
	tests := []struct {
		name     string
		document any
	}{
		{name: "empty enum", document: map[string]any{"enum": []any{}}},
		{name: "duplicate exact enum numbers", document: map[string]any{"enum": []any{json.Number("1"), json.Number("1.0")}}},
		{name: "empty type", document: map[string]any{"type": []any{}}},
		{name: "empty allOf", document: map[string]any{"allOf": []any{}}},
		{name: "empty tuple", document: map[string]any{"items": []any{}}},
		{name: "invalid annotation", document: map[string]any{"description": true}},
		{
			name: "hidden invalid target",
			document: map[string]any{
				"default": map[string]any{"enum": []any{}},
				"$ref":    "#/default",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Prepare(test.document); err == nil {
				t.Fatal("invalid Draft 7 schema prepared")
			}
		})
	}

	for _, document := range []any{
		true,
		map[string]any{},
		map[string]any{"required": []any{}},
		map[string]any{"maxItems": json.Number("1.0")},
	} {
		if _, err := Prepare(document); err != nil {
			t.Fatalf("valid Draft 7 schema rejected: %v", err)
		}
	}
}

func TestPrepareRejectsIdentifiersOnlyAtActualSchemaNodes(t *testing.T) {
	literal := map[string]any{
		"default": map[string]any{"$id": "https://must-not-be-used.invalid/schema"},
		"type":    "string",
	}
	if _, err := Prepare(literal); err != nil {
		t.Fatalf("identifier inside unreferenced literal data rejected: %v", err)
	}
	literal["$ref"] = "#/default"
	if _, err := Prepare(literal); err == nil || !strings.Contains(err.Error(), "prohibited $id") {
		t.Fatalf("identifier at referenced schema node error = %v", err)
	}
}

func TestPrepareCountsReferencedBooleanTargetsOnce(t *testing.T) {
	document := map[string]any{
		"default": false,
		"allOf": []any{
			map[string]any{"$ref": "#/default"},
			map[string]any{"$ref": "#/default"},
		},
	}
	limits := DefaultCompileLimits()
	limits.MaxSchemaNodes = 4
	if _, err := PrepareWithLimits(document, limits); err != nil {
		t.Fatalf("unique root, branches, and boolean target rejected: %v", err)
	}
	limits.MaxSchemaNodes = 3
	if _, err := PrepareWithLimits(document, limits); err == nil || !strings.Contains(err.Error(), "schema nodes") {
		t.Fatalf("referenced boolean target node limit error = %v", err)
	}
}

func TestPrepareAggregatesNumericCompileWorkAtExactBoundary(t *testing.T) {
	exact := map[string]any{
		"allOf": []any{
			map[string]any{"maximum": json.Number("1e8191")},
			map[string]any{"maximum": json.Number(strings.Repeat("9", 127))},
			map[string]any{"maximum": json.Number(strings.Repeat("9", 14))},
			map[string]any{"maximum": json.Number("9999")},
			map[string]any{"maxItems": json.Number("0")},
		},
	}
	if _, err := Prepare(exact); err != nil {
		t.Fatalf("exact 64 MiB numeric work rejected: %v", err)
	}
	exact["minLength"] = json.Number("0")
	if _, err := Prepare(exact); err == nil || !strings.Contains(err.Error(), "numeric compile work") {
		t.Fatalf("numeric work above 64 MiB error = %v", err)
	}
}

func TestPrepareNormalizesProgrammaticNumbersAndRejectsNonJSONFloats(t *testing.T) {
	graph, err := Prepare(map[string]any{"minimum": 1, "maximum": float32(2.5)})
	if err != nil {
		t.Fatal(err)
	}
	root := graph.RootClone().(map[string]any)
	if _, ok := root["minimum"].(json.Number); !ok {
		t.Fatalf("minimum type = %T, want json.Number", root["minimum"])
	}
	if _, ok := root["maximum"].(json.Number); !ok {
		t.Fatalf("maximum type = %T, want json.Number", root["maximum"])
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := Prepare(map[string]any{"const": value}); err == nil {
			t.Fatalf("non-JSON float %v prepared", value)
		}
	}
	for _, value := range []json.Number{"", "01", "NaN", "Inf", "+Inf", "-Inf", "not-a-number"} {
		if _, err := Prepare(map[string]any{"const": value}); err == nil {
			t.Fatalf("invalid programmatic json.Number %q prepared", value)
		}
	}
}

func TestPrepareRequiresEveryCompileLimitToBePositive(t *testing.T) {
	mutations := []func(*CompileLimits){
		func(limits *CompileLimits) { limits.MaxSchemaNodes = 0 },
		func(limits *CompileLimits) { limits.MaxSchemaCollectionEntries = -1 },
		func(limits *CompileLimits) { limits.MaxSchemaEnumValues = 0 },
		func(limits *CompileLimits) { limits.MaxSchemaLiteralBytes = -1 },
		func(limits *CompileLimits) { limits.MaxSchemaPatternBytes = 0 },
		func(limits *CompileLimits) { limits.MaxSameInstanceSchemaVisits = -1 },
		func(limits *CompileLimits) { limits.MaxNumericCompileWorkUnits = -1 },
	}
	for index, mutate := range mutations {
		limits := DefaultCompileLimits()
		mutate(&limits)
		if _, err := PrepareWithLimits(true, limits); err == nil {
			t.Fatalf("invalid compile limits case %d prepared", index)
		}
	}
}

func TestPrepareAcceptsOnlyRootJSONPointerReferences(t *testing.T) {
	for _, reference := range []string{"", "?", "relative.json", "#anchor"} {
		if _, err := Prepare(map[string]any{"$ref": reference}); err == nil || !strings.Contains(err.Error(), "non-local $ref") {
			t.Fatalf("$ref %q error = %v", reference, err)
		}
	}
	if _, err := Prepare(map[string]any{
		"$ref": "#%2Fspace%20key",
		"space key": map[string]any{
			"type": "string",
		},
	}); err != nil {
		t.Fatalf("percent-encoded root pointer rejected: %v", err)
	}
}

func TestRebasePreservesDualLiteralAndReferenceTargetSemantics(t *testing.T) {
	simpleGraph, err := Prepare(map[string]any{
		"$ref":   "#/const",
		"const":  map[string]any{"$ref": "#/hidden"},
		"hidden": map[string]any{"type": "string"},
	})
	if err != nil {
		t.Fatal(err)
	}
	simpleCompiled, err := simpleGraph.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := simpleCompiled.ValidateFlag(json.Number("1")); err != nil || valid {
		t.Fatalf("simple hidden multi-hop invalid input = %v, %v", valid, err)
	}

	targetName := "percent%field space/\u96ea~"
	targetReference := fragmentReference([]string{targetName})
	document := map[string]any{
		"$ref": "#/const",
		"const": map[string]any{
			"$ref": targetReference,
		},
		targetName:        map[string]any{"type": "string"},
		mcpTestAnnotation: "literal collision",
	}
	graph, err := Prepare(document)
	if err != nil {
		t.Fatal(err)
	}
	sourceCompiled, err := graph.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := sourceCompiled.ValidateFlag(json.Number("1")); err != nil || valid {
		t.Fatalf("source invalid input = %v, %v", valid, err)
	}
	embedded, err := graph.Rebase("#/properties/input", mcpTestAnnotation)
	if err != nil {
		t.Fatal(err)
	}
	embeddedObject := embedded.(map[string]any)
	literalTarget := embeddedObject["const"].(map[string]any)
	if literalTarget["$ref"] != targetReference {
		t.Fatalf("source literal changed: %#v", literalTarget)
	}
	if embeddedObject[mcpTestAnnotation] != "literal collision" {
		t.Fatal("framework annotation overwrote source data")
	}
	if _, ok := embeddedObject[mcpTestAnnotation+"-1"].(map[string]any); !ok {
		t.Fatalf("collision-free hidden target annotation missing: %#v", embeddedObject)
	}
	wrapper := map[string]any{
		"type":       "object",
		"properties": map[string]any{"input": embedded},
		"required":   []any{"input"},
	}
	compiled, err := CompileWithLimits(wrapper, func() CompileLimits {
		limits := DefaultCompileLimits()
		limits.MaxSchemaNodes += 8
		return limits
	}())
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := compiled.ValidateFlag(map[string]any{"input": "ok"}); err != nil || !valid {
		t.Fatalf("rebased valid input = %v, %v", valid, err)
	}
	if valid, err := compiled.ValidateFlag(map[string]any{"input": json.Number("1")}); err != nil || valid {
		t.Fatalf("rebased invalid input = %v, %v; embedded=%#v", valid, err, embedded)
	}
}

const mcpTestAnnotation = "x-modary-mcp-ref-targets"
