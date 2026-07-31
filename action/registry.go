package action

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/mod/semver"
)

const (
	maxDescriptorTitleRunes       = 160
	maxDescriptorDescriptionRunes = 2048
	maxDescriptorChannels         = 32
	maxActionVersionRunes         = 128
	maxChannelRunes               = 64
	maxActionIdentifierBytes      = 127
)

var (
	actionIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}(?:\.[a-z][a-z0-9_-]{0,62})*$`)
	contractHashPattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ValidIdentifier reports whether value is a canonical Action identifier or
// permission. Dot-separated segments are URL-path safe and deterministic
// across HTTP, CLI, MCP, generated code, and storage adapters.
func ValidIdentifier(value string) bool {
	return len(value) <= maxActionIdentifierBytes && actionIdentifierPattern.MatchString(value)
}

// CatalogEntry is the read-only discovery view of an Action. It intentionally
// omits the Handler so callers cannot bypass the governed Runtime.
type CatalogEntry struct {
	Descriptor   Descriptor `json:"descriptor"`
	ModuleID     string     `json:"module_id"`
	ContractHash string     `json:"contract_hash"`
}

func cloneDescriptor(descriptor Descriptor) Descriptor {
	clone := descriptor
	clone.InputSchema = append([]byte(nil), descriptor.InputSchema...)
	clone.PreviewSchema = append([]byte(nil), descriptor.PreviewSchema...)
	clone.OutputSchema = append([]byte(nil), descriptor.OutputSchema...)
	clone.Channels = append([]Channel(nil), descriptor.Channels...)
	clone.Errors = append([]ErrorSpec(nil), descriptor.Errors...)
	return clone
}

type compiledDescriptor struct {
	input   *Validator
	preview *Validator
	output  *Validator
}

// PreparedDescriptor is an immutable, precompiled Action contract. Its
// validators are intentionally opaque so a Module Host can validate before
// startup and later bind a Handler without recompiling schemas.
type PreparedDescriptor struct {
	descriptor   Descriptor
	compiled     compiledDescriptor
	contractHash string
}

// PrepareDescriptor validates and compiles an Action contract without creating
// or invoking a Handler.
func PrepareDescriptor(descriptor Descriptor) (PreparedDescriptor, error) {
	for _, schema := range []struct {
		name  string
		value json.RawMessage
	}{
		{name: "input", value: descriptor.InputSchema},
		{name: "preview", value: descriptor.PreviewSchema},
		{name: "output", value: descriptor.OutputSchema},
	} {
		if int64(len(schema.value)) > MaxJSONDocumentBytes {
			return PreparedDescriptor{}, fmt.Errorf("Action %s schema exceeds %d bytes", schema.name, MaxJSONDocumentBytes)
		}
	}
	descriptor = cloneDescriptor(descriptor)
	sort.Slice(descriptor.Channels, func(i, j int) bool {
		return descriptor.Channels[i] < descriptor.Channels[j]
	})
	sort.Slice(descriptor.Errors, func(i, j int) bool { return descriptor.Errors[i].Code < descriptor.Errors[j].Code })
	compiled, err := compileDescriptor(descriptor)
	if err != nil {
		return PreparedDescriptor{}, err
	}
	contractHash, err := hashDescriptorContract(descriptor)
	if err != nil {
		return PreparedDescriptor{}, err
	}
	return PreparedDescriptor{descriptor: cloneDescriptor(descriptor), compiled: compiled, contractHash: contractHash}, nil
}

// Descriptor returns a defensive copy of the prepared static contract.
func (prepared PreparedDescriptor) Descriptor() Descriptor {
	return cloneDescriptor(prepared.descriptor)
}

// ContractHash returns the canonical hash of the prepared Action contract.
func (prepared PreparedDescriptor) ContractHash() string { return prepared.contractHash }

// Valid reports whether the value is a complete descriptor produced by
// PrepareDescriptor rather than a zero or partially copied value.
func (prepared PreparedDescriptor) Valid() bool {
	return prepared.descriptor.ID != "" && prepared.compiled.input != nil &&
		prepared.compiled.output != nil && contractHashPattern.MatchString(prepared.contractHash) &&
		((prepared.descriptor.Preview == PreviewNone && prepared.compiled.preview == nil) ||
			(prepared.descriptor.Preview != PreviewNone && prepared.compiled.preview != nil))
}

// ValidateInput validates one canonical input value against the prepared
// Action contract without recompiling its schema.
func (prepared PreparedDescriptor) ValidateInput(value []byte) error {
	if prepared.compiled.input == nil {
		return fmt.Errorf("prepared Action descriptor is invalid")
	}
	return prepared.compiled.input.Validate(value)
}

// ValidatePreview validates one canonical Preview summary against the prepared
// Action contract without recompiling its schema.
func (prepared PreparedDescriptor) ValidatePreview(value []byte) error {
	if prepared.descriptor.Preview == PreviewNone {
		if prepared.compiled.preview != nil {
			return fmt.Errorf("prepared Action descriptor is invalid")
		}
		return nil
	}
	if prepared.compiled.preview == nil {
		return fmt.Errorf("prepared Action descriptor is invalid")
	}
	return prepared.compiled.preview.Validate(value)
}

// ValidateOutput validates one canonical result value against the prepared
// Action contract without recompiling its schema.
func (prepared PreparedDescriptor) ValidateOutput(value []byte) error {
	if prepared.compiled.output == nil {
		return fmt.Errorf("prepared Action descriptor is invalid")
	}
	return prepared.compiled.output.Validate(value)
}

// ValidateDescriptor compiles every schema and validates the static Action
// contract without creating or invoking a Handler.
func ValidateDescriptor(descriptor Descriptor) error {
	_, err := PrepareDescriptor(descriptor)
	return err
}

func compileDescriptor(descriptor Descriptor) (compiledDescriptor, error) {
	if !ValidIdentifier(descriptor.ID) {
		return compiledDescriptor{}, fmt.Errorf("action id %q must match %s", descriptor.ID, actionIdentifierPattern.String())
	}
	if err := validateDescriptorText("version", descriptor.Version, true, maxActionVersionRunes); err != nil {
		return compiledDescriptor{}, fmt.Errorf("action %s has invalid %w", descriptor.ID, err)
	}
	if !validActionVersion(descriptor.Version) {
		return compiledDescriptor{}, fmt.Errorf("action %s version %q is not valid Semantic Versioning 2.0.0", descriptor.ID, descriptor.Version)
	}
	if err := validateDescriptorText("title", descriptor.Title, true, maxDescriptorTitleRunes); err != nil {
		return compiledDescriptor{}, fmt.Errorf("action %s has invalid %w", descriptor.ID, err)
	}
	if err := validateDescriptorText("description", descriptor.Description, false, maxDescriptorDescriptionRunes); err != nil {
		return compiledDescriptor{}, fmt.Errorf("action %s has invalid %w", descriptor.ID, err)
	}
	if !ValidIdentifier(descriptor.Permission) {
		return compiledDescriptor{}, fmt.Errorf("action %s permission %q must match %s", descriptor.ID, descriptor.Permission, actionIdentifierPattern.String())
	}
	if descriptor.Preview != PreviewNone && descriptor.Preview != PreviewOptional && descriptor.Preview != PreviewRequired {
		return compiledDescriptor{}, fmt.Errorf("action %s has invalid preview policy %q", descriptor.ID, descriptor.Preview)
	}
	if descriptor.AuditLevel != AuditMetadata && descriptor.AuditLevel != AuditDetailed {
		return compiledDescriptor{}, fmt.Errorf("action %s must declare a valid audit level", descriptor.ID)
	}
	input, err := CompileValidator(descriptor.InputSchema)
	if err != nil {
		return compiledDescriptor{}, fmt.Errorf("action %s must declare a valid input schema: %w", descriptor.ID, err)
	}
	var preview *Validator
	if descriptor.Preview != PreviewNone {
		if len(descriptor.PreviewSchema) == 0 {
			return compiledDescriptor{}, fmt.Errorf("action %s must declare a preview summary schema", descriptor.ID)
		}
		preview, err = CompileValidator(descriptor.PreviewSchema)
		if err != nil {
			return compiledDescriptor{}, fmt.Errorf("action %s must declare a valid preview summary schema: %w", descriptor.ID, err)
		}
	} else if len(descriptor.PreviewSchema) > 0 {
		return compiledDescriptor{}, fmt.Errorf("action %s declares a preview summary schema with preview policy none", descriptor.ID)
	}
	output, err := CompileValidator(descriptor.OutputSchema)
	if err != nil {
		return compiledDescriptor{}, fmt.Errorf("action %s must declare a valid output schema: %w", descriptor.ID, err)
	}
	if len(descriptor.Channels) == 0 {
		return compiledDescriptor{}, fmt.Errorf("action %s must declare at least one channel", descriptor.ID)
	}
	if len(descriptor.Channels) > maxDescriptorChannels {
		return compiledDescriptor{}, fmt.Errorf("action %s declares more than %d channels", descriptor.ID, maxDescriptorChannels)
	}
	seenChannels := make(map[Channel]struct{}, len(descriptor.Channels))
	for _, channel := range descriptor.Channels {
		if err := validateDescriptorText("channel", string(channel), true, maxChannelRunes); err != nil {
			return compiledDescriptor{}, fmt.Errorf("action %s has invalid %w", descriptor.ID, err)
		}
		if _, exists := seenChannels[channel]; exists {
			return compiledDescriptor{}, fmt.Errorf("action %s declares channel %q more than once", descriptor.ID, channel)
		}
		seenChannels[channel] = struct{}{}
	}
	if err := validateErrorSpecs(descriptor.Errors); err != nil {
		return compiledDescriptor{}, fmt.Errorf("action %s has invalid error contract: %w", descriptor.ID, err)
	}
	return compiledDescriptor{input: input, preview: preview, output: output}, nil
}

func validateErrorSpecs(specs []ErrorSpec) error {
	if len(specs) > maxDescriptorErrors {
		return fmt.Errorf("cannot declare more than %d errors", maxDescriptorErrors)
	}
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if _, builtin := BuiltinErrorKind(spec.Code); builtin {
			return fmt.Errorf("error code %q is owned by the framework", spec.Code)
		}
		if !ValidCustomErrorCode(spec.Code) {
			return fmt.Errorf("error code %q must match %s and cannot exceed %d bytes", spec.Code, customErrorCodePattern.String(), maxErrorCodeBytes)
		}
		if !spec.Kind.Valid() {
			return fmt.Errorf("error code %q has invalid kind %q", spec.Code, spec.Kind)
		}
		if spec.Kind == ErrorKindInternal {
			return fmt.Errorf("error code %q cannot declare the framework-owned internal kind", spec.Code)
		}
		if _, exists := seen[spec.Code]; exists {
			return fmt.Errorf("error code %q is declared more than once", spec.Code)
		}
		seen[spec.Code] = struct{}{}
	}
	return nil
}

func validActionVersion(version string) bool {
	coreVersion := version
	if index := strings.IndexAny(coreVersion, "-+"); index >= 0 {
		coreVersion = coreVersion[:index]
	}
	return strings.Count(coreVersion, ".") == 2 && semver.IsValid("v"+version)
}

func hashDescriptorContract(descriptor Descriptor) (string, error) {
	canonicalSchema := func(name string, schema json.RawMessage) (json.RawMessage, error) {
		if len(schema) == 0 {
			return nil, nil
		}
		value, err := decodeSingleJSON(schema)
		if err != nil {
			return nil, fmt.Errorf("canonicalize %s schema: %w", name, err)
		}
		if err := prepareSchemaDocument(value); err != nil {
			return nil, fmt.Errorf("canonicalize %s schema: %w", name, err)
		}
		canonical, err := canonicalizeJSONValue(value)
		if err != nil {
			return nil, fmt.Errorf("canonicalize %s schema: %w", name, err)
		}
		encoded, err := json.Marshal(canonical)
		if err != nil {
			return nil, fmt.Errorf("canonicalize %s schema: %w", name, err)
		}
		if err := ValidateJSONDocument(encoded); err != nil {
			return nil, fmt.Errorf("canonicalize %s schema: %w", name, err)
		}
		return encoded, nil
	}
	input, err := canonicalSchema("input", descriptor.InputSchema)
	if err != nil {
		return "", err
	}
	preview, err := canonicalSchema("preview", descriptor.PreviewSchema)
	if err != nil {
		return "", err
	}
	output, err := canonicalSchema("output", descriptor.OutputSchema)
	if err != nil {
		return "", err
	}
	material := struct {
		ID                  string          `json:"id"`
		Version             string          `json:"version"`
		InputSchema         json.RawMessage `json:"input_schema"`
		PreviewSchema       json.RawMessage `json:"preview_schema,omitempty"`
		OutputSchema        json.RawMessage `json:"output_schema"`
		Permission          string          `json:"permission"`
		Preview             PreviewPolicy   `json:"preview"`
		AuditLevel          AuditLevel      `json:"audit_level"`
		Channels            []Channel       `json:"channels"`
		Errors              []ErrorSpec     `json:"errors,omitempty"`
		RequiresIdempotency bool            `json:"requires_idempotency"`
	}{
		ID: descriptor.ID, Version: descriptor.Version, InputSchema: input, PreviewSchema: preview,
		OutputSchema: output, Permission: descriptor.Permission, Preview: descriptor.Preview,
		AuditLevel: descriptor.AuditLevel, Channels: descriptor.Channels, Errors: descriptor.Errors,
		RequiresIdempotency: descriptor.RequiresIdempotency,
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", fmt.Errorf("marshal Action contract: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

func validateDescriptorText(field, value string, required bool, maxRunes int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if required && value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s cannot contain surrounding whitespace", field)
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("%s cannot exceed %d characters", field, maxRunes)
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s cannot contain control characters", field)
	}
	return nil
}
