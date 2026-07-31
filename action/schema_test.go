package action

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	frameworkschema "github.com/iiwish/modary/internal/jsonschema"
)

func TestSchemaBuildersProduceDeterministicStrictObjects(t *testing.T) {
	schema := Object(map[string]Field{
		"count": OptionalField(Integer(Minimum(0), Description("Number of rows"))),
		"name":  RequiredField(String(MinLength(1), Description("Stable name"))),
	}).JSON()
	if err := ValidateSchema(schema); err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSON(schema, json.RawMessage(`{"name":"example","count":2}`)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSON(schema, json.RawMessage(`{"name":"example","extra":true}`)); ErrorCode(err) != CodeValidationFailed {
		t.Fatalf("additional property error = %v", err)
	}
	if strings.Index(string(schema), `"count"`) > strings.Index(string(schema), `"name"`) {
		t.Fatalf("schema properties are not deterministic: %s", schema)
	}
}

func TestPublishedSchemaLimitsMatchCompilerProfile(t *testing.T) {
	published := []int{
		MaxSchemaNodes,
		MaxSchemaCollectionEntries,
		MaxSchemaEnumValues,
		MaxSchemaLiteralBytes,
		MaxSchemaPatternBytes,
		MaxSchemaSameInstanceVisits,
		MaxSchemaNumericCompileWorkUnits,
		MaxSchemaEvaluationWorkUnits,
		MaxSchemaMismatchEvents,
		MaxSchemaEvaluationFrames,
	}
	implemented := []int{
		frameworkschema.MaxSchemaNodes,
		frameworkschema.MaxSchemaCollectionEntries,
		frameworkschema.MaxSchemaEnumValues,
		frameworkschema.MaxSchemaLiteralBytes,
		frameworkschema.MaxSchemaPatternBytes,
		frameworkschema.MaxSameInstanceSchemaVisits,
		frameworkschema.MaxSchemaNumericCompileWorkUnits,
		frameworkschema.MaxEvaluationWorkUnits,
		frameworkschema.MaxGeneratedDiagnostics,
		int(frameworkschema.MaxEvaluationFrames),
	}
	for index := range published {
		if published[index] != implemented[index] {
			t.Fatalf("published schema limit %d = %d, implementation = %d", index, published[index], implemented[index])
		}
	}
}

func TestSchemaBuilderSealsOptionAndNestedState(t *testing.T) {
	var captured map[string]any
	option := SchemaOption{name: "test metadata", allowed: schemaKindAny, apply: func(node map[string]any) {
		captured = node
		node["metadata"] = map[string]any{"stable": true}
	}}
	child := String(option)
	parent := Object(map[string]Field{"value": RequiredField(child)})
	captured["type"] = "integer"
	captured["metadata"].(map[string]any)["stable"] = false
	child.node.(map[string]any)["type"] = "boolean"

	encoded := string(parent.JSON())
	if !strings.Contains(encoded, `"type":"string"`) || !strings.Contains(encoded, `"stable":true`) {
		t.Fatalf("sealed Schema changed through aliased builder state: %s", encoded)
	}
}

func TestCompileValidatorRejectsRemoteSchemaReferencesWithoutIO(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		response.Header().Set("Content-Type", "application/schema+json")
		_, _ = response.Write([]byte(`{"type":"string"}`))
	}))
	defer server.Close()

	tests := []struct {
		name   string
		schema string
		match  string
	}{
		{name: "http ref", schema: fmt.Sprintf(`{"$ref":%q}`, server.URL+"/remote.json"), match: "non-local $ref"},
		{name: "relative ref", schema: `{"$ref":"schemas/remote.json"}`, match: "non-local $ref"},
		{name: "named fragment ref", schema: `{"$ref":"#anchor"}`, match: "non-local $ref"},
		{name: "dependency schema http ref", schema: fmt.Sprintf(`{"type":"object","dependencies":{"trigger":{"$ref":%q}}}`, server.URL+"/dependency.json"), match: "non-local $ref"},
		{name: "referenced annotation http ref", schema: fmt.Sprintf(`{"default":{"$ref":%q},"$ref":"#/default"}`, server.URL+"/hidden.json"), match: "non-local $ref"},
		{name: "remote dollar id", schema: fmt.Sprintf(`{"$id":%q,"type":"string"}`, server.URL+"/root.json"), match: "non-local base"},
		{name: "remote legacy id", schema: fmt.Sprintf(`{"id":%q,"type":"string"}`, server.URL+"/root.json"), match: "non-local base"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CompileValidator(json.RawMessage(test.schema)); err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("CompileValidator() error = %v, want %q", err, test.match)
			}
		})
	}
	if hits.Load() != 0 {
		t.Fatalf("schema compilation performed %d HTTP requests", hits.Load())
	}
}

func TestCompileValidatorCannotBeHijackedByIdentifiersInLiteralData(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		response.Header().Set("Content-Type", "application/schema+json")
		_, _ = response.Write([]byte(`{"type":"integer"}`))
	}))
	defer server.Close()

	schema := mustMarshalSchema(t, map[string]any{
		"definitions": map[string]any{
			"safe": map[string]any{"type": "string"},
		},
		"default": map[string]any{
			"$id":  "#/definitions/safe",
			"$ref": server.URL + "/must-not-load.json",
		},
		"$ref": "#/definitions/safe",
	})
	validator, err := CompileValidator(schema)
	if err != nil {
		t.Fatalf("compile schema with inert identifier literal: %v", err)
	}
	if err := validator.Validate(json.RawMessage(`"safe"`)); err != nil {
		t.Fatalf("safe local target rejected: %v", err)
	}
	if err := validator.Validate(json.RawMessage(`1`)); ErrorCode(err) != CodeValidationFailed {
		t.Fatalf("literal identifier hijacked local target: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("schema compilation performed %d HTTP requests", hits.Load())
	}

	active := mustMarshalSchema(t, map[string]any{
		"default": map[string]any{
			"$id":  "#/definitions/safe",
			"$ref": server.URL + "/must-not-load.json",
		},
		"$ref": "#/default",
	})
	if _, err := CompileValidator(active); err == nil || !strings.Contains(err.Error(), "prohibited $id") {
		t.Fatalf("active identifier error = %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("active identifier path performed %d HTTP requests", hits.Load())
	}
}

func TestCompileValidatorRejectsFileSchemaReferenceBeforeReadingSentinel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "must-not-be-read.json")
	if err := os.WriteFile(path, []byte(`this sentinel is deliberately not JSON`), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	ref := (&url.URL{Scheme: "file", Path: path}).String()
	_, err := CompileValidator(json.RawMessage(fmt.Sprintf(`{"type":"object","dependencies":{"trigger":{"$ref":%q}}}`, ref)))
	if err == nil || !strings.Contains(err.Error(), "non-local $ref") || strings.Contains(err.Error(), "sentinel") {
		t.Fatalf("CompileValidator(file ref) error = %v", err)
	}
}

func TestCompileValidatorRejectsRemoteRefsInEveryDraft7SubschemaPosition(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	defer server.Close()
	ref := fmt.Sprintf(`{"$ref":%q}`, server.URL+"/remote.json")
	tests := []struct {
		name   string
		schema string
	}{
		{name: "additionalItems", schema: fmt.Sprintf(`{"additionalItems":%s}`, ref)},
		{name: "additionalProperties", schema: fmt.Sprintf(`{"additionalProperties":%s}`, ref)},
		{name: "contains", schema: fmt.Sprintf(`{"contains":%s}`, ref)},
		{name: "else", schema: fmt.Sprintf(`{"else":%s}`, ref)},
		{name: "if", schema: fmt.Sprintf(`{"if":%s}`, ref)},
		{name: "not", schema: fmt.Sprintf(`{"not":%s}`, ref)},
		{name: "propertyNames", schema: fmt.Sprintf(`{"propertyNames":%s}`, ref)},
		{name: "then", schema: fmt.Sprintf(`{"then":%s}`, ref)},
		{name: "allOf", schema: fmt.Sprintf(`{"allOf":[%s]}`, ref)},
		{name: "anyOf", schema: fmt.Sprintf(`{"anyOf":[%s]}`, ref)},
		{name: "oneOf", schema: fmt.Sprintf(`{"oneOf":[%s]}`, ref)},
		{name: "items schema", schema: fmt.Sprintf(`{"items":%s}`, ref)},
		{name: "items tuple", schema: fmt.Sprintf(`{"items":[%s]}`, ref)},
		{name: "definitions", schema: fmt.Sprintf(`{"definitions":{"nested":%s}}`, ref)},
		{name: "properties", schema: fmt.Sprintf(`{"properties":{"nested":%s}}`, ref)},
		{name: "patternProperties", schema: fmt.Sprintf(`{"patternProperties":{".*":%s}}`, ref)},
		{name: "schema dependency", schema: fmt.Sprintf(`{"dependencies":{"trigger":%s}}`, ref)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CompileValidator(json.RawMessage(test.schema)); err == nil || !strings.Contains(err.Error(), "non-local $ref") {
				t.Fatalf("CompileValidator() error = %v", err)
			}
		})
	}
	if hits.Load() != 0 {
		t.Fatalf("subschema compilation performed %d HTTP requests", hits.Load())
	}
}

func TestCompileValidatorAllowsSameDocumentSchemaReferences(t *testing.T) {
	schema := json.RawMessage(`{
		"$schema":"http://json-schema.org/draft-07/schema#",
		"definitions":{"positive":{"type":"integer","minimum":0}},
		"$ref":"#/definitions/positive"
	}`)
	validator, err := CompileValidator(schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(json.RawMessage(`7`)); err != nil {
		t.Fatalf("local reference rejected valid input: %v", err)
	}
	if err := validator.Validate(json.RawMessage(`-1`)); ErrorCode(err) != CodeValidationFailed {
		t.Fatalf("local reference invalid input error = %v", err)
	}

}

func TestCompileValidatorProfilesReferencedSchemaOutsideOrdinaryPositions(t *testing.T) {
	properties := mappableBooleanSchemas(MaxSchemaCollectionEntries + 1)
	schema := mustMarshalSchema(t, map[string]any{
		"default": map[string]any{"properties": properties},
		"$ref":    "#/default",
	})
	if _, err := CompileValidator(schema); err == nil || !strings.Contains(err.Error(), "properties entries") {
		t.Fatalf("referenced hidden schema profile error = %v", err)
	}

	validator, err := CompileValidator(json.RawMessage(`{
		"default":{"type":"integer","minimum":0},
		"$ref":"#/default"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(json.RawMessage(`1`)); err != nil {
		t.Fatalf("referenced hidden schema rejected valid value: %v", err)
	}
	if err := validator.Validate(json.RawMessage(`-1`)); ErrorCode(err) != CodeValidationFailed {
		t.Fatalf("referenced hidden schema invalid value error = %v", err)
	}
}

func TestCompileValidatorRejectsZeroProgressLocalReferenceCycles(t *testing.T) {
	tests := []struct {
		name   string
		schema string
	}{
		{name: "direct root", schema: `{"$ref":"#","type":"integer"}`},
		{name: "allOf root", schema: `{"allOf":[{"$ref":"#"}]}`},
		{name: "conditional root", schema: `{"if":{"$ref":"#"},"then":true}`},
		{name: "schema dependency", schema: `{"type":"object","dependencies":{"trigger":{"$ref":"#"}}}`},
		{name: "referenced annotation", schema: `{"default":{"$ref":"#"},"$ref":"#/default"}`},
		{name: "indirect definitions", schema: `{
			"definitions":{
				"first":{"anyOf":[{"$ref":"#/definitions/second"}]},
				"second":{"not":{"$ref":"#/definitions/first"}}
			},
			"$ref":"#/definitions/first"
		}`},
		{name: "escaped pointer definitions", schema: `{
			"definitions":{
				"first/part":{"$ref":"#/definitions/second~0part"},
				"second~part":{"$ref":"#/definitions/first~1part"}
			},
			"type":"string"
		}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CompileValidator(json.RawMessage(test.schema)); err == nil || !strings.Contains(err.Error(), "zero-progress local $ref cycle") {
				t.Fatalf("CompileValidator() error = %v", err)
			}
		})
	}
}

func TestCompileValidatorAllowsRecursiveReferencesAfterInstanceProgress(t *testing.T) {
	schema := json.RawMessage(`{
		"$schema":"http://json-schema.org/draft-07/schema#",
		"definitions":{"node":{
			"type":"object",
			"properties":{
				"value":{"type":"integer"},
				"next":{"oneOf":[{"type":"null"},{"$ref":"#/definitions/node"}]}
			},
			"required":["value"],
			"additionalProperties":false
		}},
		"$ref":"#/definitions/node"
	}`)
	validator, err := CompileValidator(schema)
	if err != nil {
		t.Fatalf("recursive tree schema was rejected: %v", err)
	}
	if err := validator.Validate(json.RawMessage(`{"value":1,"next":{"value":2,"next":null}}`)); err != nil {
		t.Fatalf("recursive tree rejected valid input: %v", err)
	}
	if err := validator.Validate(json.RawMessage(`{"value":1,"next":{"value":"wrong"}}`)); ErrorCode(err) != CodeValidationFailed {
		t.Fatalf("recursive tree invalid input error = %v", err)
	}
}

func TestValidatorPreservesLargeJSONIntegers(t *testing.T) {
	t.Run("maximum", func(t *testing.T) {
		validator, err := CompileValidator(json.RawMessage(`{"type":"integer","maximum":9007199254740992}`))
		if err != nil {
			t.Fatal(err)
		}
		if err := validator.Validate(json.RawMessage(`9007199254740992`)); err != nil {
			t.Fatalf("maximum boundary rejected: %v", err)
		}
		if err := validator.Validate(json.RawMessage(`9007199254740993`)); ErrorCode(err) != CodeValidationFailed {
			t.Fatalf("integer above exact maximum error = %v", err)
		}
	})

	t.Run("const", func(t *testing.T) {
		validator, err := CompileValidator(json.RawMessage(`{"const":9007199254740993}`))
		if err != nil {
			t.Fatal(err)
		}
		if err := validator.Validate(json.RawMessage(`9007199254740993`)); err != nil {
			t.Fatalf("exact large const rejected: %v", err)
		}
		if err := validator.Validate(json.RawMessage(`9007199254740992`)); ErrorCode(err) != CodeValidationFailed {
			t.Fatalf("neighboring large integer matched const: %v", err)
		}
	})

	t.Run("nested const and enum", func(t *testing.T) {
		validator, err := CompileValidator(json.RawMessage(`{
			"type":"object",
			"properties":{
				"constant":{"const":9007199254740993},
				"choice":{"enum":[9007199254740993,9007199254740995,"other"]},
				"items":{"type":"array","items":{"enum":[9007199254740993]}}
			},
			"required":["constant","choice","items"]
		}`))
		if err != nil {
			t.Fatal(err)
		}
		valid := json.RawMessage(`{"constant":9007199254740993,"choice":9007199254740995,"items":[9007199254740993]}`)
		if err := validator.Validate(valid); err != nil {
			t.Fatalf("exact nested numeric constraints rejected: %v", err)
		}
		for _, input := range []json.RawMessage{
			json.RawMessage(`{"constant":9007199254740992,"choice":9007199254740995,"items":[9007199254740993]}`),
			json.RawMessage(`{"constant":9007199254740993,"choice":9007199254740994,"items":[9007199254740993]}`),
			json.RawMessage(`{"constant":9007199254740993,"choice":9007199254740995,"items":[9007199254740992]}`),
		} {
			if err := validator.Validate(input); ErrorCode(err) != CodeValidationFailed {
				t.Fatalf("neighboring large integer matched nested constraint: input=%s error=%v", input, err)
			}
		}
		if err := validator.Validate(json.RawMessage(`{"constant":9007199254740993,"choice":"other","items":[9007199254740993]}`)); err != nil {
			t.Fatalf("mixed non-numeric enum value rejected: %v", err)
		}
	})

	t.Run("local reference enum", func(t *testing.T) {
		validator, err := CompileValidator(json.RawMessage(`{
			"definitions":{"large":{"enum":[9007199254740993]}},
			"$ref":"#/definitions/large"
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if err := validator.Validate(json.RawMessage(`9007199254740993`)); err != nil {
			t.Fatalf("exact referenced numeric enum rejected: %v", err)
		}
		if err := validator.Validate(json.RawMessage(`9007199254740992`)); ErrorCode(err) != CodeValidationFailed {
			t.Fatalf("neighboring referenced numeric enum matched: %v", err)
		}
	})

	t.Run("compound const and enum values", func(t *testing.T) {
		validator, err := CompileValidator(json.RawMessage(`{
			"anyOf":[
				{"const":{"kind":"const","nested":{"values":[9007199254740993,true]}}},
				{"enum":[{"kind":"enum","values":[9007199254740995,"stable"]}]}
			]
		}`))
		if err != nil {
			t.Fatal(err)
		}
		for _, input := range []json.RawMessage{
			json.RawMessage(`{"kind":"const","nested":{"values":[9007199254740993,true]}}`),
			json.RawMessage(`{"kind":"enum","values":[9007199254740995,"stable"]}`),
		} {
			if err := validator.Validate(input); err != nil {
				t.Fatalf("exact compound value %s rejected: %v", input, err)
			}
		}
		for _, input := range []json.RawMessage{
			json.RawMessage(`{"kind":"const","nested":{"values":[9007199254740992,true]}}`),
			json.RawMessage(`{"kind":"enum","values":[9007199254740994,"stable"]}`),
			json.RawMessage(`{"kind":"enum","values":[9007199254740995,"stable"],"extra":true}`),
		} {
			if err := validator.Validate(input); ErrorCode(err) != CodeValidationFailed {
				t.Fatalf("neighboring compound value matched: input=%s error=%v", input, err)
			}
		}
	})

	t.Run("maximum number token", func(t *testing.T) {
		number := strings.Repeat("9", MaxJSONNumberBytes)
		validator, err := CompileValidator(json.RawMessage(`{"const":` + number + `}`))
		if err != nil {
			t.Fatal(err)
		}
		if err := validator.Validate(json.RawMessage(number)); err != nil {
			t.Fatalf("4 KiB numeric const rejected: %v", err)
		}
		neighbor := number[:len(number)-1] + "8"
		if err := validator.Validate(json.RawMessage(neighbor)); ErrorCode(err) != CodeValidationFailed {
			t.Fatalf("neighboring 4 KiB numeric const matched: %v", err)
		}
	})
}

func TestValidatorRequiresExactlyOneJSONValue(t *testing.T) {
	validator, err := CompileValidator(Object(map[string]Field{"name": RequiredField(String())}).JSON())
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []json.RawMessage{
		json.RawMessage(`{"name":"first"} {"name":"second"}`),
		json.RawMessage(`{"name":"first"} trailing`),
		json.RawMessage(``),
	} {
		if err := validator.Validate(input); ErrorCode(err) != CodeValidationFailed {
			t.Fatalf("Validate(%q) error = %v", input, err)
		}
	}
	if err := validator.Validate(json.RawMessage("{\"name\":\"only\"}\n\t")); err != nil {
		t.Fatalf("single JSON value with trailing whitespace: %v", err)
	}
}

func TestParseSchemaRejectsNonDraft7Declaration(t *testing.T) {
	for _, declaration := range []string{
		"http://json-schema.org/draft-04/schema#",
		"https://json-schema.org/draft/2020-12/schema",
	} {
		t.Run(declaration, func(t *testing.T) {
			_, err := ParseSchema(json.RawMessage(fmt.Sprintf(`{"$schema":%q,"type":"string"}`, declaration)))
			if err == nil || !strings.Contains(err.Error(), "only JSON Schema Draft 7 is supported") {
				t.Fatalf("ParseSchema() error = %v", err)
			}
		})
	}
	schema, err := ParseSchema(json.RawMessage(`{"$schema":"https://json-schema.org/draft-07/schema#","type":"string"}`))
	if err != nil {
		t.Fatalf("Draft 7 declaration rejected: %v", err)
	}
	if !strings.Contains(string(schema.JSON()), draft7URI) {
		t.Fatalf("canonical Draft 7 declaration missing: %s", schema.JSON())
	}
}

func TestParseSchemaSupportsEveryDraft7RootForm(t *testing.T) {
	tests := []struct {
		name      string
		schema    json.RawMessage
		input     json.RawMessage
		wantValid bool
	}{
		{name: "true", schema: json.RawMessage(`true`), input: json.RawMessage(`{"anything":1}`), wantValid: true},
		{name: "false", schema: json.RawMessage(`false`), input: json.RawMessage(`null`), wantValid: false},
		{name: "empty object", schema: json.RawMessage(`{}`), input: json.RawMessage(`[1,2,3]`), wantValid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := ParseSchema(test.schema)
			if err != nil {
				t.Fatal(err)
			}
			validator, err := CompileValidator(schema.JSON())
			if err != nil {
				t.Fatal(err)
			}
			err = validator.Validate(test.input)
			if test.wantValid && err != nil {
				t.Fatalf("valid input rejected: %v", err)
			}
			if !test.wantValid && ErrorCode(err) != CodeValidationFailed {
				t.Fatalf("invalid input error = %v", err)
			}
		})
	}

	booleanSchema, err := ParseSchema(json.RawMessage(`true`))
	if err != nil {
		t.Fatal(err)
	}
	assertSchemaPanic(t, "cannot apply options to a boolean Schema", func() {
		booleanSchema.With(Description("not applicable"))
	})
}

func TestCompileValidatorEnforcesSchemaExecutionProfileBoundaries(t *testing.T) {
	t.Run("schema nodes", func(t *testing.T) {
		exact := schemaNodeBoundary(MaxSchemaNodes)
		if _, err := CompileValidator(mustMarshalSchema(t, exact)); err != nil {
			t.Fatalf("exact schema node limit: %v", err)
		}
		above := schemaNodeBoundary(MaxSchemaNodes + 1)
		if _, err := CompileValidator(mustMarshalSchema(t, above)); err == nil || !strings.Contains(err.Error(), "schema nodes") {
			t.Fatalf("above schema node limit: %v", err)
		}
	})

	t.Run("collection entries", func(t *testing.T) {
		schema := func(entries int) map[string]any {
			properties := make(map[string]any, entries)
			for index := 0; index < entries; index++ {
				properties[fmt.Sprintf("p%03d", index)] = true
			}
			return map[string]any{"properties": properties}
		}
		if _, err := CompileValidator(mustMarshalSchema(t, schema(MaxSchemaCollectionEntries))); err != nil {
			t.Fatalf("exact collection limit: %v", err)
		}
		if _, err := CompileValidator(mustMarshalSchema(t, schema(MaxSchemaCollectionEntries+1))); err == nil || !strings.Contains(err.Error(), "properties entries") {
			t.Fatalf("above collection limit: %v", err)
		}
	})

	t.Run("enum values", func(t *testing.T) {
		schema := func(values int) map[string]any {
			enum := make([]any, values)
			for index := range enum {
				enum[index] = fmt.Sprintf("v%03d", index)
			}
			return map[string]any{"enum": enum}
		}
		if _, err := CompileValidator(mustMarshalSchema(t, schema(MaxSchemaEnumValues))); err != nil {
			t.Fatalf("exact enum limit: %v", err)
		}
		if _, err := CompileValidator(mustMarshalSchema(t, schema(MaxSchemaEnumValues+1))); err == nil || !strings.Contains(err.Error(), "enum values") {
			t.Fatalf("above enum limit: %v", err)
		}
	})

	t.Run("literal bytes", func(t *testing.T) {
		exact := map[string]any{"const": strings.Repeat("x", MaxSchemaLiteralBytes-2)}
		if _, err := CompileValidator(mustMarshalSchema(t, exact)); err != nil {
			t.Fatalf("exact literal limit: %v", err)
		}
		above := map[string]any{"const": strings.Repeat("x", MaxSchemaLiteralBytes-1)}
		if _, err := CompileValidator(mustMarshalSchema(t, above)); err == nil || !strings.Contains(err.Error(), "const literal bytes") {
			t.Fatalf("above literal limit: %v", err)
		}
	})

	t.Run("pattern bytes", func(t *testing.T) {
		exact := map[string]any{"pattern": strings.Repeat("a", MaxSchemaPatternBytes)}
		if _, err := CompileValidator(mustMarshalSchema(t, exact)); err != nil {
			t.Fatalf("exact pattern limit: %v", err)
		}
		above := map[string]any{"pattern": strings.Repeat("a", MaxSchemaPatternBytes+1)}
		if _, err := CompileValidator(mustMarshalSchema(t, above)); err == nil || !strings.Contains(err.Error(), "pattern bytes") {
			t.Fatalf("above pattern limit: %v", err)
		}
	})

	t.Run("same instance visits", func(t *testing.T) {
		schema := func(nested int) map[string]any {
			branches := make([]any, MaxSchemaCollectionEntries)
			for index := range branches {
				branches[index] = map[string]any{}
			}
			children := make([]any, nested)
			for index := range children {
				children[index] = map[string]any{}
			}
			branches[0] = map[string]any{"allOf": children}
			return map[string]any{"allOf": branches}
		}
		exactNested := MaxSchemaSameInstanceVisits - 1 - MaxSchemaCollectionEntries
		if _, err := CompileValidator(mustMarshalSchema(t, schema(exactNested))); err != nil {
			t.Fatalf("exact same-instance limit: %v", err)
		}
		if _, err := CompileValidator(mustMarshalSchema(t, schema(exactNested+1))); err == nil || !strings.Contains(err.Error(), "same-instance schema visits") {
			t.Fatalf("above same-instance limit: %v", err)
		}
	})
}

func TestCompileValidatorRejectsNumericExponentBombBeforeCompilation(t *testing.T) {
	exponent := strings.Repeat("9", MaxJSONNumberBytes-2)
	schema := json.RawMessage(`{"maximum":1e` + exponent + `}`)
	if _, err := CompileValidator(schema); err == nil || !strings.Contains(err.Error(), "numeric compile work") {
		t.Fatalf("CompileValidator(exponent bomb) error = %v", err)
	}

	ordinary := json.RawMessage(`{"maximum":` + strings.Repeat("9", MaxJSONNumberBytes) + `}`)
	if _, err := CompileValidator(ordinary); err != nil {
		t.Fatalf("ordinary 4 KiB number rejected: %v", err)
	}
}

func TestCompileValidatorUsesEngineNativeExactEnumWithoutExpansion(t *testing.T) {
	items := make([]any, MaxSchemaCollectionEntries)
	for index := range items {
		items[index] = []any{1, 2, 3}
	}
	schema := mustMarshalSchema(t, map[string]any{"enum": []any{items}})
	validator, err := CompileValidator(schema)
	if err != nil {
		t.Fatalf("compile native exact enum: %v", err)
	}
	value, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(value); err != nil {
		t.Fatalf("validate native exact enum: %v", err)
	}
}

func TestExactNumericRewritePreservesSourceDepthBoundary(t *testing.T) {
	arrays := MaxJSONNestingDepth - 1
	exactValue := strings.Repeat("[", arrays) + "9007199254740993" + strings.Repeat("]", arrays)
	validator, err := CompileValidator(json.RawMessage(`{"const":` + exactValue + `}`))
	if err != nil {
		t.Fatalf("compile exact-depth numeric const: %v", err)
	}
	if err := validator.Validate(json.RawMessage(exactValue)); err != nil {
		t.Fatalf("validate exact-depth numeric const: %v", err)
	}
}

func TestValidatorEnforcesEvaluationWorkAndMismatchBoundaries(t *testing.T) {
	patternValidator, err := CompileValidator(mustMarshalSchema(t, map[string]any{
		"type":    "string",
		"pattern": strings.Repeat("a", MaxSchemaPatternBytes),
	}))
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(strings.Repeat("a", 20_000))
	if err != nil {
		t.Fatal(err)
	}
	if err := patternValidator.Validate(input); ErrorCode(err) != CodeLimitExceeded {
		t.Fatalf("regex evaluation error = %v", err)
	}

	mismatchValidator, err := CompileValidator(json.RawMessage(`{"type":"array","items":false}`))
	if err != nil {
		t.Fatal(err)
	}
	mismatches := func(count int) json.RawMessage {
		return json.RawMessage("[" + strings.TrimSuffix(strings.Repeat("true,", count), ",") + "]")
	}
	if err := mismatchValidator.Validate(mismatches(MaxSchemaMismatchEvents)); ErrorCode(err) != CodeValidationFailed {
		t.Fatalf("exact mismatch event limit error = %v", err)
	}
	if err := mismatchValidator.Validate(mismatches(MaxSchemaMismatchEvents + 1)); ErrorCode(err) != CodeLimitExceeded {
		t.Fatalf("above mismatch event limit error = %v", err)
	}
}

func TestValidatorIsConcurrencySafe(t *testing.T) {
	validator, err := CompileValidator(json.RawMessage(`{"type":"integer","minimum":0}`))
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(valid bool) {
			defer wait.Done()
			err := validator.Validate(json.RawMessage(`1`))
			if !valid {
				err = validator.Validate(json.RawMessage(`-1`))
				if ErrorCode(err) == CodeValidationFailed {
					err = nil
				}
			}
			if err != nil {
				errs <- err
			}
		}(worker%2 == 0)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func schemaNodeBoundary(nodes int) map[string]any {
	root := map[string]any{"properties": map[string]any{}}
	properties := root["properties"].(map[string]any)
	remaining := nodes - 1
	for index := 0; remaining > 0; index++ {
		child := map[string]any{"properties": map[string]any{}}
		properties[fmt.Sprintf("p%03d", index)] = child
		remaining--
		nested := child["properties"].(map[string]any)
		for childIndex := 0; childIndex < 3 && remaining > 0; childIndex++ {
			nested[fmt.Sprintf("n%d", childIndex)] = map[string]any{}
			remaining--
		}
	}
	return root
}

func mustMarshalSchema(t *testing.T, schema any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func mappableBooleanSchemas(entries int) map[string]any {
	properties := make(map[string]any, entries)
	for index := 0; index < entries; index++ {
		properties[fmt.Sprintf("p%03d", index)] = true
	}
	return properties
}

func TestSchemaOptionsRejectWrongTargetAndZeroValue(t *testing.T) {
	tests := []struct {
		name  string
		build func()
		match string
	}{
		{name: "string minimum", build: func() { String(Minimum(1)) }, match: "Minimum cannot be applied to string Schema"},
		{name: "integer min length", build: func() { Integer(MinLength(1)) }, match: "MinLength cannot be applied to integer Schema"},
		{name: "array format", build: func() { Array(String(), Format("date-time")) }, match: "Format cannot be applied to array Schema"},
		{name: "zero option", build: func() { String(SchemaOption{}) }, match: "SchemaOption 0 is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertSchemaPanic(t, test.match, test.build)
		})
	}
}

func TestSchemaBuildersRejectZeroNestedSchemas(t *testing.T) {
	tests := []struct {
		name  string
		build func()
		match string
	}{
		{name: "object", build: func() { Object(map[string]Field{"missing": {Schema: Schema{}}}) }, match: "Object field \"missing\" received a zero Schema"},
		{name: "array", build: func() { Array(Schema{}) }, match: "Array items received a zero Schema"},
		{name: "one of member", build: func() { OneOf(String(), Schema{}) }, match: "OneOf schema 1 received a zero Schema"},
		{name: "empty one of", build: func() { OneOf() }, match: "OneOf requires at least one Schema"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertSchemaPanic(t, test.match, test.build)
		})
	}
}

func assertSchemaPanic(t *testing.T, match string, build func()) {
	t.Helper()
	defer func() {
		value := recover()
		if value == nil || !strings.Contains(fmt.Sprint(value), match) {
			t.Fatalf("panic = %v, want containing %q", value, match)
		}
	}()
	build()
}

func typeScriptDescriptor(id string, input, output json.RawMessage) Descriptor {
	return Descriptor{
		ID:           id,
		Version:      "1.0.0",
		Title:        "Generated contract",
		InputSchema:  input,
		OutputSchema: output,
		Permission:   id,
		Preview:      PreviewNone,
		AuditLevel:   AuditMetadata,
		Channels:     []Channel{ChannelHTTP},
	}
}

func TestGenerateTypeScriptCatalogUsesActionSchemas(t *testing.T) {
	descriptor := Descriptor{
		ID:            "example.create",
		Version:       "0.1.0",
		Title:         "Create example",
		InputSchema:   Object(map[string]Field{"name": RequiredField(String())}).JSON(),
		PreviewSchema: Object(map[string]Field{"matched": RequiredField(Integer())}).JSON(),
		OutputSchema:  Object(map[string]Field{"id": RequiredField(String())}).JSON(),
		Permission:    "example.create",
		Preview:       PreviewRequired,
		AuditLevel:    AuditMetadata,
		Channels:      []Channel{ChannelHTTP},
		Errors: []ErrorSpec{
			{Code: "EXAMPLE.NAME_TAKEN", Kind: ErrorKindConflict},
			{Code: "EXAMPLE.NOT_READY", Kind: ErrorKindPrecondition},
		},
	}
	generated, err := GenerateTypeScriptCatalog([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	for _, expected := range []string{
		`"example.create": {`,
		`input: { name: string }`,
		`preview: { matched: number }`,
		`output: { id: string }`,
		`"EXAMPLE.NAME_TAKEN": "conflict"`,
		`"EXAMPLE.NOT_READY": "precondition"`,
		`export interface FrameworkActionErrors`,
		`"INTERNAL_ERROR": "internal"`,
		`export type PreviewActionID`,
		`export type ActionOutput`,
		`export type FrameworkActionErrorCode`,
		`export type DeclaredActionErrorCode`,
		`export type ActionErrorCode`,
		`export type ActionErrorKind`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated catalog missing %q:\n%s", expected, text)
		}
	}
}

func TestGenerateTypeScriptCatalogIncludesCompleteFrameworkErrorContract(t *testing.T) {
	generated, err := GenerateTypeScriptCatalog([]Descriptor{
		typeScriptDescriptor("example.read", Object(nil).JSON(), Object(nil).JSON()),
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	want := []string{
		`"ACTION_NOT_FOUND": "not_found";`,
		`"AUTHZ_DENIED": "denied";`,
		`"IDEMPOTENCY_CONFLICT": "conflict";`,
		`"IDEMPOTENCY_IN_PROGRESS": "conflict";`,
		`"IDEMPOTENCY_REQUIRED": "precondition_required";`,
		`"INTERNAL_ERROR": "internal";`,
		`"LIMIT_EXCEEDED": "limit";`,
		`"PLAN_NOT_FOUND": "not_found";`,
		`"PLAN_REQUIRED": "precondition_required";`,
		`"PLAN_STALE": "conflict";`,
		`"PRECONDITION_FAILED": "precondition";`,
		`"UNAVAILABLE": "unavailable";`,
		`"VALIDATION_FAILED": "validation";`,
	}
	previous := -1
	for _, entry := range want {
		index := strings.Index(text, entry)
		if index < 0 || index <= previous || strings.Count(text, entry) != 1 {
			t.Fatalf("framework error mapping is incomplete or non-canonical at %q:\n%s", entry, text)
		}
		previous = index
	}
	for _, declaration := range []string{
		`export type FrameworkActionErrorCode = Extract<keyof FrameworkActionErrors, string>;`,
		`export type DeclaredActionErrorCode<ID extends ActionID> = ActionContracts[ID] extends { errors: infer Errors } ? Extract<keyof Errors, string> : never;`,
		`export type ActionErrorCode<ID extends ActionID> = FrameworkActionErrorCode | DeclaredActionErrorCode<ID>;`,
		`Code extends FrameworkActionErrorCode ? FrameworkActionErrors[Code]`,
	} {
		if !strings.Contains(text, declaration) {
			t.Fatalf("generated catalog missing %q:\n%s", declaration, text)
		}
	}
}

func TestGenerateTypeScriptCatalogCanonicalizesAndValidatesErrorContracts(t *testing.T) {
	descriptor := typeScriptDescriptor("inventory.reserve", Object(nil).JSON(), Object(nil).JSON())
	descriptor.Channels = []Channel{ChannelMCP, ChannelHTTP}
	descriptor.Errors = []ErrorSpec{
		{Code: "INVENTORY.OUT_OF_STOCK", Kind: ErrorKindConflict},
		{Code: "INVENTORY.NOT_READY", Kind: ErrorKindPrecondition},
	}
	generated, err := GenerateTypeScriptCatalog([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	first := strings.Index(text, `"INVENTORY.NOT_READY"`)
	second := strings.Index(text, `"INVENTORY.OUT_OF_STOCK"`)
	if first < 0 || second < 0 || first >= second {
		t.Fatalf("generated errors are not canonical:\n%s", text)
	}
	if descriptor.Errors[0].Code != "INVENTORY.OUT_OF_STOCK" {
		t.Fatal("GenerateTypeScriptCatalog mutated the caller's error slice")
	}
	if descriptor.Channels[0] != "mcp" {
		t.Fatal("GenerateTypeScriptCatalog mutated the caller's channel slice")
	}

	descriptor.Errors = []ErrorSpec{{Code: "not-qualified", Kind: ErrorKindConflict}}
	if _, err := GenerateTypeScriptCatalog([]Descriptor{descriptor}); err == nil || !strings.Contains(err.Error(), "error contract") {
		t.Fatalf("invalid generated error contract error = %v", err)
	}
}

func TestGenerateTypeScriptCatalogPreparesEveryDescriptorAndRejectsDuplicateIDs(t *testing.T) {
	first := typeScriptDescriptor("catalog.read", Object(nil).JSON(), Object(nil).JSON())
	duplicate := first
	if _, err := GenerateTypeScriptCatalog([]Descriptor{first, duplicate}); err == nil || !strings.Contains(err.Error(), `id "catalog.read" is declared more than once`) {
		t.Fatalf("duplicate Action id error = %v", err)
	}

	invalidDuplicate := duplicate
	invalidDuplicate.InputSchema = json.RawMessage(`{"type":"not-a-json-schema-type"}`)
	if _, err := GenerateTypeScriptCatalog([]Descriptor{first, invalidDuplicate}); err == nil || !strings.Contains(err.Error(), "prepare Action descriptor at index 1") {
		t.Fatalf("invalid duplicate descriptor error = %v", err)
	}
}

func TestGenerateTypeScriptCatalogDistinguishesClosedAndOpenEmptyObjects(t *testing.T) {
	descriptor := typeScriptDescriptor("object.inspect", Object(nil).JSON(), AnyObject().JSON())
	generated, err := GenerateTypeScriptCatalog([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	if !strings.Contains(text, `input: Record<string, never>`) {
		t.Fatalf("closed empty object type is not closed:\n%s", text)
	}
	if !strings.Contains(text, `output: Record<string, unknown>`) {
		t.Fatalf("open empty object type is not open:\n%s", text)
	}
}

func TestGenerateTypeScriptCatalogConservativelyMapsDynamicObjectProperties(t *testing.T) {
	descriptor := typeScriptDescriptor(
		"object.dynamic",
		json.RawMessage(`{
			"type":"object",
			"properties":{"fixed":{"type":"string"}},
			"required":["fixed"]
		}`),
		json.RawMessage(`{
			"type":"object",
			"additionalProperties":{"type":"integer"}
		}`),
	)
	descriptor.Preview = PreviewOptional
	descriptor.PreviewSchema = json.RawMessage(`{
		"type":"object",
		"properties":{"fixed":{"type":"boolean"}},
		"patternProperties":{"^x-":{"type":"number"}},
		"additionalProperties":false
	}`)

	generated, err := GenerateTypeScriptCatalog([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	for _, expected := range []string{
		`input: { fixed: string; [key: string]: unknown }`,
		`preview: { fixed?: boolean; [key: string]: unknown }`,
		`output: Record<string, number>`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated dynamic object contract missing %q:\n%s", expected, text)
		}
	}
}

func TestGenerateTypeScriptCatalogDoesNotApplyAdditionalSchemaToNamedProperties(t *testing.T) {
	descriptor := typeScriptDescriptor(
		"object.mixed",
		json.RawMessage(`{
			"type":"object",
			"properties":{"name":{"type":"string"}},
			"required":["name"],
			"additionalProperties":{"type":"integer"}
		}`),
		Object(nil).JSON(),
	)
	generated, err := GenerateTypeScriptCatalog([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	if expected := `input: { name: string; [key: string]: unknown }`; !strings.Contains(string(generated), expected) {
		t.Fatalf("generated mixed object contract missing %q:\n%s", expected, generated)
	}
}

func TestGenerateTypeScriptCatalogPreservesSafeNumbersAndRejectsUnsafeLiterals(t *testing.T) {
	descriptor := typeScriptDescriptor(
		"number.inspect",
		json.RawMessage(`{"type":"integer","enum":[9007199254740991,1.20e1]}`),
		json.RawMessage(`{"const":{"value":42}}`),
	)
	descriptor.Preview = PreviewRequired
	descriptor.PreviewSchema = json.RawMessage(`{"const":0.5}`)
	generated, err := GenerateTypeScriptCatalog([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"9007199254740991 | 12", `output: {"value":42}`, "preview: 0.5"} {
		if !strings.Contains(string(generated), expected) {
			t.Fatalf("generated catalog missing %q:\n%s", expected, generated)
		}
	}

	descriptor.InputSchema = json.RawMessage(`{"type":"integer","const":9007199254740993}`)
	if _, err := GenerateTypeScriptCatalog([]Descriptor{descriptor}); err == nil || !strings.Contains(err.Error(), "safe integer range") {
		t.Fatalf("unsafe numeric const error = %v", err)
	}
	descriptor.InputSchema = json.RawMessage(`{"const":{"nested":9007199254740993}}`)
	if _, err := GenerateTypeScriptCatalog([]Descriptor{descriptor}); err == nil || !strings.Contains(err.Error(), "safe integer range") {
		t.Fatalf("nested unsafe numeric const error = %v", err)
	}
	for _, schema := range []json.RawMessage{
		json.RawMessage(`{"const":0.10000000000000001}`),
		json.RawMessage(`{"const":9007199254740990.5}`),
		json.RawMessage(`{"enum":[{"nested":0.10000000000000001}]}`),
	} {
		descriptor.InputSchema = schema
		if _, err := GenerateTypeScriptCatalog([]Descriptor{descriptor}); err == nil || !strings.Contains(err.Error(), "represented exactly") {
			t.Fatalf("inexact JavaScript numeric literal %s error = %v", schema, err)
		}
	}

	firstSchema := json.RawMessage(`{"const":{"z":9007199254740993,"a":0.10000000000000001}}`)
	secondSchema := json.RawMessage(`{"const":{"a":0.10000000000000001,"z":9007199254740993}}`)
	var firstError string
	for iteration := 0; iteration < 100; iteration++ {
		descriptor.InputSchema = firstSchema
		if iteration%2 != 0 {
			descriptor.InputSchema = secondSchema
		}
		_, err := GenerateTypeScriptCatalog([]Descriptor{descriptor})
		if err == nil || !strings.Contains(err.Error(), "$/a") {
			t.Fatalf("deterministic nested numeric error = %v", err)
		}
		if iteration == 0 {
			firstError = err.Error()
		} else if err.Error() != firstError {
			t.Fatalf("nested numeric error changed:\nfirst: %s\nnext:  %s", firstError, err)
		}
	}

	descriptor.InputSchema = json.RawMessage(`{"type":"integer","const":1,"const":2}`)
	if _, err := GenerateTypeScriptCatalog([]Descriptor{descriptor}); err == nil || !strings.Contains(err.Error(), "duplicate JSON object property") {
		t.Fatalf("duplicate schema property error = %v", err)
	}
}

func TestGenerateTypeScriptCatalogUsesECMAScriptCompatibleStringEscapes(t *testing.T) {
	descriptor := typeScriptDescriptor(
		"string.inspect",
		json.RawMessage(`{"const":"\u0007"}`),
		json.RawMessage(`{"type":"object","properties":{"\u0007":{"const":"\u0007"}},"additionalProperties":false}`),
	)
	generated, err := GenerateTypeScriptCatalog([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	if strings.Contains(text, `\a`) {
		t.Fatalf("generated catalog contains Go-only escape:\n%s", text)
	}
	if strings.Count(text, `"\u0007"`) != 3 {
		t.Fatalf("generated catalog does not preserve JSON string escapes:\n%s", text)
	}
}
