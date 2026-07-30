package action

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

func ValidateJSON(schema, input json.RawMessage) error {
	if len(schema) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(input, &value); err != nil {
		return NewError(CodeValidationFailed, "input is not valid JSON")
	}
	result, err := gojsonschema.Validate(gojsonschema.NewBytesLoader(schema), gojsonschema.NewGoLoader(value))
	if err != nil {
		return fmt.Errorf("validate JSON schema: %w", err)
	}
	if result.Valid() {
		return nil
	}
	messages := make([]string, 0, len(result.Errors()))
	for _, issue := range result.Errors() {
		messages = append(messages, issue.String())
	}
	return NewError(CodeValidationFailed, strings.Join(messages, "; "))
}

func ObjectSchema(properties string, required ...string) json.RawMessage {
	if required == nil {
		required = []string{}
	}
	requiredJSON, _ := json.Marshal(required)
	return json.RawMessage(fmt.Sprintf(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{%s},"required":%s,"additionalProperties":false}`, properties, requiredJSON))
}
