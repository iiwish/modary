package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/iiwish/modary/action"
)

func TestCLIUsesPublishedActionJSONBudget(t *testing.T) {
	if DefaultMaxActionInputBytes != action.MaxJSONDocumentBytes || MaximumActionInputBytes != action.MaxJSONDocumentBytes {
		t.Fatalf("CLI Action byte limits = %d/%d, want %d", DefaultMaxActionInputBytes, MaximumActionInputBytes, action.MaxJSONDocumentBytes)
	}
	tests := []struct {
		name   string
		value  json.RawMessage
		within bool
	}{
		{name: "exact bytes", value: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-2) + `"`), within: true},
		{name: "bytes above", value: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-1) + `"`)},
		{name: "exact depth", value: cliNestedJSON(action.MaxJSONNestingDepth), within: true},
		{name: "depth above", value: cliNestedJSON(action.MaxJSONNestingDepth + 1)},
		{name: "exact nodes", value: cliArrayJSON(action.MaxJSONValueNodes - 1), within: true},
		{name: "nodes above", value: cliArrayJSON(action.MaxJSONValueNodes)},
		{name: "exact number", value: cliNumberJSON(action.MaxJSONNumberBytes), within: true},
		{name: "number above", value: cliNumberJSON(action.MaxJSONNumberBytes + 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, err := loadActionInput(
				context.Background(), "-", io.NopCloser(bytes.NewReader(test.value)), action.MaxJSONDocumentBytes,
			)
			if test.within {
				if err != nil || !bytes.Equal(input, test.value) {
					t.Fatalf("load exact boundary = %d bytes, %v", len(input), err)
				}
				return
			}
			if err == nil || input != nil {
				t.Fatalf("load value above boundary = %d bytes, %v", len(input), err)
			}
		})
	}
}

func cliNestedJSON(depth int) json.RawMessage {
	return json.RawMessage(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
}

func cliArrayJSON(values int) json.RawMessage {
	return json.RawMessage("[" + strings.TrimSuffix(strings.Repeat("0,", values), ",") + "]")
}

func cliNumberJSON(bytes int) json.RawMessage {
	return json.RawMessage("1" + strings.Repeat("0", bytes-1))
}
