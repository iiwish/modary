package action

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateJSONDocumentEnforcesPublishedResourceBoundaries(t *testing.T) {
	exactBytes := []byte("0" + strings.Repeat(" ", int(MaxJSONDocumentBytes)-1))
	if err := ValidateJSONDocument(exactBytes); err != nil {
		t.Fatalf("exact byte limit: %v", err)
	}
	if err := ValidateJSONDocument(append(exactBytes, ' ')); err == nil {
		t.Fatal("document above published byte limit succeeded")
	}

	nested := func(depth int) []byte {
		return []byte(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
	}
	if err := ValidateJSONDocument(nested(MaxJSONNestingDepth)); err != nil {
		t.Fatalf("exact nesting limit: %v", err)
	}
	if err := ValidateJSONDocument(nested(MaxJSONNestingDepth + 1)); err == nil {
		t.Fatal("document above published nesting limit succeeded")
	}

	array := func(values int) []byte {
		return []byte("[" + strings.TrimSuffix(strings.Repeat("0,", values), ",") + "]")
	}
	if err := ValidateJSONDocument(array(MaxJSONValueNodes - 1)); err != nil {
		t.Fatalf("exact node limit: %v", err)
	}
	if err := ValidateJSONDocument(array(MaxJSONValueNodes)); err == nil {
		t.Fatal("document above published node limit succeeded")
	}

	exactNumber := []byte("1" + strings.Repeat("0", MaxJSONNumberBytes-1))
	if err := ValidateJSONDocument(exactNumber); err != nil {
		t.Fatalf("exact number-token limit: %v", err)
	}
	if err := ValidateJSONDocument(append(exactNumber, '0')); err == nil {
		t.Fatal("number token above published limit succeeded")
	}
}

func TestSchemaAndValidatorUseActionJSONResourceContract(t *testing.T) {
	if _, err := CompileValidator([]byte("{}" + strings.Repeat(" ", int(MaxJSONDocumentBytes)-2))); err != nil {
		t.Fatalf("schema at byte limit: %v", err)
	}
	if _, err := CompileValidator([]byte("{}" + strings.Repeat(" ", int(MaxJSONDocumentBytes)-1))); err == nil {
		t.Fatal("schema above byte limit compiled")
	}

	validator, err := CompileValidator([]byte(`{}`))
	if err != nil {
		t.Fatalf("CompileValidator: %v", err)
	}
	tooDeep := []byte(strings.Repeat("[", MaxJSONNestingDepth+1) + "0" + strings.Repeat("]", MaxJSONNestingDepth+1))
	if err := validator.Validate(tooDeep); !IsCode(err, CodeLimitExceeded) {
		t.Fatalf("Validate(too deep) = %v", err)
	}
}

func TestSchemaSourceEnforcesEveryPublishedJSONBoundary(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		schema := func(arrays int) []byte {
			return []byte(`{"default":` + strings.Repeat("[", arrays) + "0" + strings.Repeat("]", arrays) + "}")
		}
		if _, err := CompileValidator(schema(MaxJSONNestingDepth - 1)); err != nil {
			t.Fatalf("schema at depth limit: %v", err)
		}
		if _, err := CompileValidator(schema(MaxJSONNestingDepth)); err == nil {
			t.Fatal("schema above depth limit compiled")
		}
	})

	t.Run("value nodes", func(t *testing.T) {
		schema := func(values int) []byte {
			items := strings.TrimSuffix(strings.Repeat("0,", values), ",")
			return []byte(`{"default":[` + items + "]}")
		}
		if _, err := CompileValidator(schema(MaxJSONValueNodes - 2)); err != nil {
			t.Fatalf("schema at value-node limit: %v", err)
		}
		if _, err := CompileValidator(schema(MaxJSONValueNodes - 1)); err == nil {
			t.Fatal("schema above value-node limit compiled")
		}
	})

	t.Run("number token", func(t *testing.T) {
		schema := func(numberBytes int) []byte {
			return []byte(`{"default":` + "1" + strings.Repeat("0", numberBytes-1) + "}")
		}
		if _, err := CompileValidator(schema(MaxJSONNumberBytes)); err != nil {
			t.Fatalf("schema at number-token limit: %v", err)
		}
		if _, err := CompileValidator(schema(MaxJSONNumberBytes + 1)); err == nil {
			t.Fatal("schema above number-token limit compiled")
		}
	})
}

func TestPreparedDescriptorRejectsCanonicalSchemaExpansion(t *testing.T) {
	descriptor := Descriptor{
		ID: "json.expand", Version: "0.1.0", Title: "JSON expansion",
		InputSchema:  jsonSchemaWithDescription(strings.Repeat("<", int(MaxJSONDocumentBytes/5))),
		OutputSchema: json.RawMessage(`{}`), Permission: "json.expand",
		Preview: PreviewNone, AuditLevel: AuditMetadata, Channels: []Channel{ChannelCLI},
	}
	if int64(len(descriptor.InputSchema)) >= MaxJSONDocumentBytes {
		t.Fatal("test schema must fit before canonical escaping")
	}
	if _, err := PrepareDescriptor(descriptor); err == nil {
		t.Fatal("descriptor whose canonical schema exceeds the Action limit was accepted")
	}
}

func jsonSchemaWithDescription(description string) json.RawMessage {
	return json.RawMessage(`{"description":"` + description + `"}`)
}
