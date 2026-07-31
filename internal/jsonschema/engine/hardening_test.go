package gojsonschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLocalSchemaReferencesRemainOfflineAndUnmodified(t *testing.T) {
	hidden := map[string]any{
		"$id":  "https://example.invalid/hidden",
		"$ref": "file:///must-not-be-read",
	}
	document := map[string]any{
		"$id": "https://example.invalid/root",
		"definitions": map[string]any{
			"value": map[string]any{"type": "string"},
		},
		"default": hidden,
		"$ref":    "#/definitions/value",
	}
	schema, err := NewLocalSchema(document, Draft7)
	if err != nil {
		t.Fatal(err)
	}
	if got := hidden["$ref"]; got != "file:///must-not-be-read" {
		t.Fatalf("hidden annotation reference was rewritten to %#v", got)
	}
	if got := document["$ref"]; got != "#/definitions/value" {
		t.Fatalf("root local reference was rewritten to %#v", got)
	}
	if got := len(schema.pool.schemaPoolDocuments); got != 1 {
		t.Fatalf("schema pool contains %d documents, want root only", got)
	}
	if _, exists := schema.pool.schemaPoolDocuments[""]; !exists {
		t.Fatal("schema pool does not contain the root document at the empty key")
	}
	if schema.rootSchema.refSchema == nil || !schema.rootSchema.refSchema.types.Contains(TYPE_STRING) {
		t.Fatalf("local pointer compiled target = %#v", schema.rootSchema.refSchema)
	}
	if valid, err := schema.ValidateFlag("ok", generousTestBudget()); err != nil || !valid {
		t.Fatalf("local pointer valid value = %v, %v", valid, err)
	}
	if valid, err := schema.ValidateFlag(json.Number("1"), generousTestBudget()); err != nil || valid {
		t.Fatalf("local pointer invalid value = %v, %v", valid, err)
	}
}

func TestLocalSchemaReferenceSupportsEscapedJSONPointerTokens(t *testing.T) {
	schema, err := NewLocalSchema(map[string]any{
		"definitions": map[string]any{
			"row/count": map[string]any{"type": "integer"},
		},
		"$ref": "#/definitions/row~1count",
	}, Draft7)
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := schema.ValidateFlag(json.Number("1.0"), generousTestBudget()); err != nil || !valid {
		t.Fatalf("escaped local pointer valid value = %v, %v", valid, err)
	}
}

func TestLocalSchemaReferenceFollowsHiddenMultiHopChain(t *testing.T) {
	schema, err := NewLocalSchema(map[string]any{
		"$ref": "#/const",
		"const": map[string]any{
			"$ref": "#/hidden",
		},
		"hidden": map[string]any{"type": "string"},
	}, Draft7)
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := schema.ValidateFlag("ok", generousTestBudget()); err != nil || !valid {
		t.Fatalf("multi-hop local pointer valid value = %v, %v", valid, err)
	}
	if valid, err := schema.ValidateFlag(json.Number("1"), generousTestBudget()); err != nil || valid {
		t.Fatalf("multi-hop local pointer invalid value = %v, %v", valid, err)
	}
}

func TestExternalSchemaReferencesAreRejectedWithoutIO(t *testing.T) {
	var httpCalls atomic.Int64
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		httpCalls.Add(1)
		return nil, fmt.Errorf("unexpected HTTP request")
	})
	originalFS := osFS
	var fileCalls atomic.Int64
	osFS = osFileSystem(func(string) (*os.File, error) {
		fileCalls.Add(1)
		return nil, fmt.Errorf("unexpected file read")
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
		osFS = originalFS
	})

	for _, reference := range []string{
		"https://example.invalid/schema",
		"file:///must-not-be-read",
		"relative/schema.json",
		"#named-fragment",
		"?",
		"//example.invalid/schema",
		"urn:example:schema",
	} {
		t.Run(reference, func(t *testing.T) {
			_, err := NewLocalSchema(map[string]any{"$ref": reference}, Draft7)
			if err == nil || !strings.Contains(err.Error(), "is not a local JSON Pointer") {
				t.Fatalf("NewLocalSchema(%q) error = %v", reference, err)
			}
		})
	}
	for _, reference := range []string{
		"https://example.invalid/root",
		"file:///must-not-be-read",
	} {
		t.Run("root "+reference, func(t *testing.T) {
			_, err := NewSchema(NewReferenceLoader(reference))
			if err == nil || !strings.Contains(err.Error(), "is not a local JSON Pointer") {
				t.Fatalf("NewSchema(%q) error = %v", reference, err)
			}
		})
	}
	if got := httpCalls.Load(); got != 0 {
		t.Fatalf("schema compilation made %d HTTP requests", got)
	}
	if got := fileCalls.Load(); got != 0 {
		t.Fatalf("schema compilation made %d file reads", got)
	}
}

func TestRecursiveLocalReferenceIsBoundedByActiveFrames(t *testing.T) {
	schema, err := NewLocalSchema(map[string]any{
		"definitions": map[string]any{
			"node": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"next": map[string]any{"$ref": "#/definitions/node"},
				},
			},
		},
		"$ref": "#/definitions/node",
	}, Draft7)
	if err != nil {
		t.Fatal(err)
	}
	value := interface{}(map[string]any{})
	for depth := 0; depth < 32; depth++ {
		value = map[string]any{"next": value}
	}
	valid, err := schema.ValidateFlag(value, Budget{
		MaxWorkUnits:   1 << 20,
		MaxDiagnostics: 100,
		MaxFrames:      8,
	})
	if valid || !errors.Is(err, ErrEvaluationLimit) {
		t.Fatalf("recursive local reference = %v, %v", valid, err)
	}
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Resource != "active frames" {
		t.Fatalf("recursive frame limit = %#v, %v", limit, err)
	}
}

func TestDraft7MetaSchemaIsOfflineAndFlagOnly(t *testing.T) {
	metaSchema, err := NewDraft7MetaSchema()
	if err != nil {
		t.Fatal(err)
	}
	valid, err := metaSchema.ValidateManyFlag([]interface{}{
		true,
		map[string]any{"type": "string"},
		map[string]any{"allOf": []any{map[string]any{"type": "number"}}},
	}, generousTestBudget())
	if err != nil || !valid {
		t.Fatalf("valid Draft 7 schemas = %v, %v", valid, err)
	}
	valid, err = metaSchema.ValidateManyFlag([]interface{}{
		map[string]any{"allOf": []any{}},
	}, generousTestBudget())
	if err != nil || valid {
		t.Fatalf("invalid Draft 7 schema = %v, %v", valid, err)
	}
}

func TestDraft7MetaSchemaCacheCompilesOnceUnderConcurrency(t *testing.T) {
	var cache draft7MetaSchemaCache
	const workers = 64
	start := make(chan struct{})
	schemas := make(chan *Schema, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			schema, err := cache.load()
			if err != nil {
				errs <- err
				return
			}
			schemas <- schema
		}()
	}
	close(start)
	wait.Wait()
	close(schemas)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	var first *Schema
	for schema := range schemas {
		if schema == nil {
			t.Fatal("cached Draft 7 metaschema is nil")
		}
		if first == nil {
			first = schema
			continue
		}
		if schema != first {
			t.Fatal("concurrent cache calls returned different compiled schemas")
		}
	}
}

func TestMustBeIntegerAcceptsExactDraft7IntegerFormsAndSaturates(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	tests := []struct {
		token string
		want  int
		ok    bool
	}{
		{token: "1.0", want: 1, ok: true},
		{token: "1e1", want: 10, ok: true},
		{token: "10e-1", want: 1, ok: true},
		{token: "9223372036854775808000000", want: maxInt, ok: true},
		{token: "-9223372036854775809000000", want: minInt, ok: true},
		{token: "1e-1", ok: false},
	}
	for _, test := range tests {
		t.Run(test.token, func(t *testing.T) {
			got := mustBeInteger(json.Number(test.token))
			if !test.ok {
				if got != nil {
					t.Fatalf("mustBeInteger(%s) = %d, want nil", test.token, *got)
				}
				return
			}
			if got == nil || *got != test.want {
				t.Fatalf("mustBeInteger(%s) = %v, want %d", test.token, got, test.want)
			}
		})
	}
}

func TestDraft7IntegerKeywordsAcceptExactFormsAndLargeBounds(t *testing.T) {
	schema := mustCompileSchema(t, map[string]any{
		"minLength":     json.Number("1.0"),
		"maxLength":     json.Number("1e1"),
		"minItems":      json.Number("0e1000"),
		"maxItems":      json.Number("9223372036854775808000000"),
		"minProperties": json.Number("0.0"),
		"maxProperties": json.Number("1e20"),
	})
	if valid, err := schema.ValidateFlag("a", generousTestBudget()); err != nil || !valid {
		t.Fatalf("exact integer keyword valid value = %v, %v", valid, err)
	}
	if valid, err := schema.ValidateFlag("", generousTestBudget()); err != nil || valid {
		t.Fatalf("exact integer keyword invalid value = %v, %v", valid, err)
	}
	if got := *schema.rootSchema.maxItems; got != int(^uint(0)>>1) {
		t.Fatalf("large maxItems saturated to %d", got)
	}
}

func TestDraft7FormatIgnoresNonStringNumbers(t *testing.T) {
	schema := mustCompileSchema(t, map[string]any{"format": "email"})
	for _, value := range []json.Number{"12", "13.7"} {
		if valid, err := schema.ValidateFlag(value, generousTestBudget()); err != nil || !valid {
			t.Fatalf("format applied to JSON number %s = %v, %v", value, valid, err)
		}
	}
}

func TestContradictoryBoundsRemainValidSchemas(t *testing.T) {
	tests := map[string]struct {
		document   map[string]any
		applicable interface{}
	}{
		"string": {
			document:   map[string]any{"minLength": json.Number("2"), "maxLength": json.Number("1")},
			applicable: "a",
		},
		"object": {
			document: map[string]any{
				"minProperties": json.Number("2"),
				"maxProperties": json.Number("1"),
			},
			applicable: map[string]any{"a": true},
		},
		"array": {
			document: map[string]any{
				"minItems": json.Number("2"),
				"maxItems": json.Number("1"),
			},
			applicable: []any{true},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			schema := mustCompileSchema(t, test.document)
			if valid, err := schema.ValidateFlag(true, generousTestBudget()); err != nil || !valid {
				t.Fatalf("non-applicable instance = %v, %v", valid, err)
			}
			if valid, err := schema.ValidateFlag(test.applicable, generousTestBudget()); err != nil || valid {
				t.Fatalf("applicable instance = %v, %v", valid, err)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
