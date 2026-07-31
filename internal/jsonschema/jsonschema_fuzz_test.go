package jsonschema

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func FuzzCompileAndValidateFlagFailsClosed(f *testing.F) {
	for _, seed := range []string{
		`true`,
		`false`,
		`{}`,
		`{"type":"integer","minimum":0}`,
		`{"definitions":{"node":{"type":"object","properties":{"next":{"$ref":"#/definitions/node"}}}},"$ref":"#/definitions/node"}`,
		`{"const":9007199254740993}`,
	} {
		f.Add([]byte(seed), []byte(`{"next":null}`))
	}

	f.Fuzz(func(t *testing.T, schemaData, valueData []byte) {
		if len(schemaData) > 64<<10 || len(valueData) > 64<<10 {
			t.Skip()
		}
		schemaDocument, ok := decodeFuzzJSON(schemaData)
		if !ok {
			return
		}
		compiled, err := Compile(schemaDocument)
		if err != nil {
			return
		}
		value, ok := decodeFuzzJSON(valueData)
		if !ok {
			return
		}
		_, _ = compiled.ValidateFlag(value)
	})
}

func FuzzPrepareRebaseSemanticEquivalence(f *testing.F) {
	for _, seed := range []string{
		`true`,
		`false`,
		`{"type":"string"}`,
		`{"definitions":{"positive":{"type":"integer","minimum":0}},"$ref":"#/definitions/positive"}`,
		`{"$ref":"#/const","const":{"$ref":"#/hidden"},"hidden":{"type":"string"}}`,
	} {
		f.Add([]byte(seed), []byte(`"value"`))
		f.Add([]byte(seed), []byte(`1`))
	}
	f.Fuzz(func(t *testing.T, schemaData, valueData []byte) {
		if len(schemaData) > 64<<10 || len(valueData) > 64<<10 {
			t.Skip()
		}
		document, ok := decodeFuzzJSON(schemaData)
		if !ok {
			return
		}
		graph, err := Prepare(document)
		if err != nil {
			return
		}
		actionCompiled, err := graph.Compile()
		if err != nil {
			return
		}
		embedded, err := graph.Rebase("#/properties/input", "x-modary-mcp-ref-targets")
		if err != nil {
			t.Fatalf("Rebase() after Prepare() = %v", err)
		}
		limits := DefaultCompileLimits()
		limits.MaxSchemaNodes += 8
		wrapperCompiled, err := CompileWithLimits(map[string]any{
			"type":       "object",
			"properties": map[string]any{"input": embedded},
			"required":   []any{"input"},
		}, limits)
		if err != nil {
			// Wrapper-owned numeric and static allowance is exercised in MCP;
			// arbitrary fuzz schemas may already sit exactly at a public limit.
			return
		}
		value, ok := decodeFuzzJSON(valueData)
		if !ok {
			return
		}
		actionValid, actionErr := actionCompiled.ValidateFlag(value)
		wrapperValid, wrapperErr := wrapperCompiled.ValidateFlag(map[string]any{"input": value})
		if (actionErr == nil) != (wrapperErr == nil) || actionValid != wrapperValid {
			t.Fatalf("Action = %v, %v; rebased = %v, %v", actionValid, actionErr, wrapperValid, wrapperErr)
		}
	})
}

func decodeFuzzJSON(data []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, false
	}
	return value, true
}
