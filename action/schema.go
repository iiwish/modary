package action

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	frameworkschema "github.com/iiwish/modary/internal/jsonschema"
	"github.com/iiwish/modary/internal/jsonvalue"
)

const draft7URI = "http://json-schema.org/draft-07/schema#"

// JSON Schema execution limits apply independently to each compiled Action
// schema and each validation call.
const (
	MaxSchemaNodes                   = 2_048
	MaxSchemaCollectionEntries       = 512
	MaxSchemaEnumValues              = 256
	MaxSchemaLiteralBytes            = 16 << 10
	MaxSchemaPatternBytes            = 4 << 10
	MaxSchemaSameInstanceVisits      = 1_024
	MaxSchemaNumericCompileWorkUnits = 64 << 20
	MaxSchemaEvaluationWorkUnits     = 64 << 20
	MaxSchemaMismatchEvents          = 4_096
	MaxSchemaEvaluationFrames        = 4_096
)

var errJSONSchemaMismatch = errors.New("JSON Schema validation failed")

// Schema is an immutable JSON Schema node assembled through typed builders.
// JSON adds the Draft 7 declaration when the node becomes a public root schema.
type Schema struct {
	node any
}

// Validator is an immutable, concurrency-safe compiled JSON Schema.
type Validator struct {
	compiled *frameworkschema.Compiled
}

// Field associates a property Schema with its required-presence flag for Object.
type Field struct {
	Schema   Schema
	Required bool
}

type schemaKind uint16

const (
	schemaKindObject schemaKind = 1 << iota
	schemaKindString
	schemaKindInteger
	schemaKindNumber
	schemaKindBoolean
	schemaKindArray
	schemaKindComposite
	schemaKindUnknown

	schemaKindAny     = schemaKindObject | schemaKindString | schemaKindInteger | schemaKindNumber | schemaKindBoolean | schemaKindArray | schemaKindComposite | schemaKindUnknown
	schemaKindNumeric = schemaKindInteger | schemaKindNumber
)

// SchemaOption is a sealed mutation applied while a Schema is being built.
// Callers use the typed option constructors below; the internal map never
// escapes a builder.
type SchemaOption struct {
	name    string
	allowed schemaKind
	apply   func(map[string]any)
}

// RequiredField returns an Object field that must be present.
func RequiredField(schema Schema) Field {
	requireSchema(schema, "RequiredField")
	return Field{Schema: schema, Required: true}
}

// OptionalField returns an Object field that may be omitted.
func OptionalField(schema Schema) Field {
	requireSchema(schema, "OptionalField")
	return Field{Schema: schema}
}

// Object builds a closed object Schema from named fields. Properties not listed
// in fields are rejected.
func Object(fields map[string]Field, options ...SchemaOption) Schema {
	properties := make(map[string]any, len(fields))
	required := make([]string, 0, len(fields))
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field := fields[name]
		requireSchema(field.Schema, fmt.Sprintf("Object field %q", name))
		properties[name] = cloneSchemaValue(field.Schema.node)
		if field.Required {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	node := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
	applySchemaOptions(node, schemaKindObject, options)
	return sealSchema(node)
}

// AnyObject builds an object Schema that accepts arbitrary properties.
func AnyObject(options ...SchemaOption) Schema {
	node := map[string]any{"type": "object", "additionalProperties": true}
	applySchemaOptions(node, schemaKindObject, options)
	return sealSchema(node)
}

// String builds a string Schema with the supplied compatible options.
func String(options ...SchemaOption) Schema {
	node := map[string]any{"type": "string"}
	applySchemaOptions(node, schemaKindString, options)
	return sealSchema(node)
}

// StringEnum builds a string Schema restricted to the supplied values.
func StringEnum(values ...string) Schema {
	items := append([]string(nil), values...)
	sort.Strings(items)
	return sealSchema(map[string]any{"type": "string", "enum": items})
}

// ConstString builds a string Schema restricted to value.
func ConstString(value string, options ...SchemaOption) Schema {
	node := map[string]any{"type": "string", "const": value}
	applySchemaOptions(node, schemaKindString, options)
	return sealSchema(node)
}

// Integer builds an integer Schema with the supplied compatible options.
func Integer(options ...SchemaOption) Schema {
	node := map[string]any{"type": "integer"}
	applySchemaOptions(node, schemaKindInteger, options)
	return sealSchema(node)
}

// Number builds a numeric Schema with the supplied compatible options.
func Number(options ...SchemaOption) Schema {
	node := map[string]any{"type": "number"}
	applySchemaOptions(node, schemaKindNumber, options)
	return sealSchema(node)
}

// Boolean builds a boolean Schema with the supplied compatible options.
func Boolean(options ...SchemaOption) Schema {
	node := map[string]any{"type": "boolean"}
	applySchemaOptions(node, schemaKindBoolean, options)
	return sealSchema(node)
}

// Array builds an array Schema whose elements must satisfy items.
func Array(items Schema, options ...SchemaOption) Schema {
	requireSchema(items, "Array items")
	node := map[string]any{"type": "array", "items": cloneSchemaValue(items.node)}
	applySchemaOptions(node, schemaKindArray, options)
	return sealSchema(node)
}

// OneOf builds a Schema that requires exactly one supplied Schema to match.
func OneOf(schemas ...Schema) Schema {
	if len(schemas) == 0 {
		panic("action: OneOf requires at least one Schema")
	}
	items := make([]any, 0, len(schemas))
	for index, schema := range schemas {
		requireSchema(schema, fmt.Sprintf("OneOf schema %d", index))
		items = append(items, cloneSchemaValue(schema.node))
	}
	return sealSchema(map[string]any{"oneOf": items})
}

// Description adds human-readable description metadata to a Schema.
func Description(value string) SchemaOption {
	return newSchemaOption("Description", schemaKindAny, func(node map[string]any) { node["description"] = value })
}

// MinLength sets the minimum string length.
func MinLength(value int) SchemaOption {
	return newSchemaOption("MinLength", schemaKindString, func(node map[string]any) { node["minLength"] = value })
}

// MaxLength sets the maximum string length.
func MaxLength(value int) SchemaOption {
	return newSchemaOption("MaxLength", schemaKindString, func(node map[string]any) { node["maxLength"] = value })
}

// Minimum sets the inclusive lower bound for an integer or number Schema.
func Minimum(value int) SchemaOption {
	return newSchemaOption("Minimum", schemaKindNumeric, func(node map[string]any) { node["minimum"] = value })
}

// Maximum sets the inclusive upper bound for an integer or number Schema.
func Maximum(value int) SchemaOption {
	return newSchemaOption("Maximum", schemaKindNumeric, func(node map[string]any) { node["maximum"] = value })
}

// MaxItems sets the maximum number of elements accepted by an array Schema.
func MaxItems(value int) SchemaOption {
	return newSchemaOption("MaxItems", schemaKindArray, func(node map[string]any) { node["maxItems"] = value })
}

// Format sets the JSON Schema format annotation for a string Schema.
func Format(value string) SchemaOption {
	return newSchemaOption("Format", schemaKindString, func(node map[string]any) { node["format"] = value })
}

// With returns a new Schema with compatible options applied.
func (schema Schema) With(options ...SchemaOption) Schema {
	requireSchema(schema, "Schema.With receiver")
	node, ok := schema.node.(map[string]any)
	if !ok {
		panic("action: Schema.With cannot apply options to a boolean Schema")
	}
	node = cloneMap(node)
	applySchemaOptions(node, kindOfSchema(node), options)
	return sealSchema(node)
}

// JSON returns the immutable Schema as a Draft 7 JSON document.
func (schema Schema) JSON() json.RawMessage {
	requireSchema(schema, "Schema.JSON receiver")
	value := cloneSchemaValue(schema.node)
	if node, ok := value.(map[string]any); ok {
		node["$schema"] = draft7URI
	}
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal JSON Schema: %v", err))
	}
	return data
}

// ParseSchema parses and validates one Draft 7 JSON Schema into an immutable Schema.
func ParseSchema(data json.RawMessage) (Schema, error) {
	graph, err := decodeAndPrepareSchema(data)
	if err != nil {
		return Schema{}, fmt.Errorf("parse JSON Schema: %w", err)
	}
	if _, err := graph.Compile(); err != nil {
		return Schema{}, fmt.Errorf("schema is not a valid JSON Schema: %w", err)
	}
	document := graph.RootClone()
	if node, ok := document.(map[string]any); ok {
		delete(node, "$schema")
	}
	return sealSchemaValue(document), nil
}

func newSchemaOption(name string, allowed schemaKind, apply func(map[string]any)) SchemaOption {
	return SchemaOption{name: name, allowed: allowed, apply: apply}
}

func applySchemaOptions(node map[string]any, kind schemaKind, options []SchemaOption) {
	for index, option := range options {
		if option.name == "" || option.allowed == 0 || option.apply == nil {
			panic(fmt.Sprintf("action: SchemaOption %d is invalid", index))
		}
		if option.allowed&kind == 0 {
			panic(fmt.Sprintf("action: %s cannot be applied to %s Schema", option.name, kind))
		}
		option.apply(node)
	}
}

func requireSchema(schema Schema, context string) {
	if schema.node == nil {
		panic(fmt.Sprintf("action: %s received a zero Schema", context))
	}
}

func kindOfSchema(node map[string]any) schemaKind {
	switch node["type"] {
	case "object":
		return schemaKindObject
	case "string":
		return schemaKindString
	case "integer":
		return schemaKindInteger
	case "number":
		return schemaKindNumber
	case "boolean":
		return schemaKindBoolean
	case "array":
		return schemaKindArray
	}
	if _, ok := node["oneOf"]; ok {
		return schemaKindComposite
	}
	return schemaKindUnknown
}

// String returns the diagnostic name of a schema kind.
func (kind schemaKind) String() string {
	switch kind {
	case schemaKindObject:
		return "object"
	case schemaKindString:
		return "string"
	case schemaKindInteger:
		return "integer"
	case schemaKindNumber:
		return "number"
	case schemaKindBoolean:
		return "boolean"
	case schemaKindArray:
		return "array"
	case schemaKindComposite:
		return "composite"
	default:
		return "unknown"
	}
}

func sealSchema(node map[string]any) Schema {
	return sealSchemaValue(node)
}

func sealSchemaValue(value any) Schema {
	return Schema{node: cloneSchemaValue(value)}
}

func cloneMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = cloneSchemaValue(value)
	}
	return clone
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneSchemaValue(item)
		}
		return clone
	case []string:
		clone := make([]string, len(typed))
		copy(clone, typed)
		return clone
	default:
		return value
	}
}

// ValidateSchema reports whether schema is a supported JSON Schema document.
func ValidateSchema(schema json.RawMessage) error {
	_, err := CompileValidator(schema)
	return err
}

// CompileValidator validates schema and returns a reusable concurrency-safe Validator.
func CompileValidator(schema json.RawMessage) (*Validator, error) {
	if len(schema) == 0 {
		return nil, fmt.Errorf("schema is empty")
	}
	graph, err := decodeAndPrepareSchema(schema)
	if err != nil {
		return nil, err
	}
	compiled, err := graph.Compile()
	if err != nil {
		return nil, fmt.Errorf("schema is not a valid JSON Schema: %w", err)
	}
	return &Validator{compiled: compiled}, nil
}

// ValidateJSON compiles schema and validates one JSON input against it.
func ValidateJSON(schema, input json.RawMessage) error {
	compiled, err := CompileValidator(schema)
	if err != nil {
		return err
	}
	return compiled.Validate(input)
}

// Validate checks one complete JSON value against the compiled Schema.
func (validator *Validator) Validate(input json.RawMessage) error {
	if validator == nil || validator.compiled == nil {
		return fmt.Errorf("JSON Schema validator is not initialized")
	}
	value, err := decodeSingleJSON(input)
	if err != nil {
		if jsonvalue.IsLimit(err) {
			return NewError(CodeLimitExceeded, "input exceeds the Action JSON resource limits")
		}
		return NewError(CodeValidationFailed, "input is not valid JSON")
	}
	valid, err := validator.compiled.ValidateFlag(value)
	if err != nil {
		if errors.Is(err, frameworkschema.ErrEvaluationLimit) {
			return &Error{
				Code:    CodeLimitExceeded,
				Kind:    ErrorKindLimit,
				Message: "input exceeds the Action JSON Schema evaluation limits",
			}
		}
		return fmt.Errorf("validate JSON schema: %w", err)
	}
	if valid {
		return nil
	}
	return &Error{
		Code:    CodeValidationFailed,
		Kind:    ErrorKindValidation,
		Message: "input does not satisfy the Action schema",
		Cause:   errJSONSchemaMismatch,
	}
}

func decodeSingleJSON(data []byte) (any, error) {
	return jsonvalue.Decode(data, actionJSONLimits)
}

func decodeAndPrepareSchema(data []byte) (*frameworkschema.SchemaGraph, error) {
	document, err := decodeSingleJSON(data)
	if err != nil {
		return nil, fmt.Errorf("schema is not valid JSON: %w", err)
	}
	graph, err := frameworkschema.Prepare(document)
	if err != nil {
		return nil, err
	}
	return graph, nil
}

// prepareSchemaDocument is retained for descriptor canonicalization. Runtime
// compilation consumes SchemaGraph directly and does not rebuild this profile.
func prepareSchemaDocument(document any) error {
	graph, err := frameworkschema.Prepare(document)
	if err != nil {
		return err
	}
	prepared := graph.RootClone()
	target, targetOK := document.(map[string]any)
	source, sourceOK := prepared.(map[string]any)
	if targetOK && sourceOK {
		for name := range target {
			delete(target, name)
		}
		for name, value := range source {
			target[name] = value
		}
		if _, declared := target["$schema"]; !declared {
			target["$schema"] = draft7URI
		}
	}
	return nil
}
