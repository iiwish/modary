package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iiwish/modary/action"
)

func TestProtocolEnvelopesDoNotConsumeActionJSONBudget(t *testing.T) {
	tests := []struct {
		name   string
		value  json.RawMessage
		within bool
	}{
		{name: "exact bytes", value: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-2) + `"`), within: true},
		{name: "bytes above", value: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-1) + `"`)},
		{name: "exact depth", value: protocolNestedJSON(action.MaxJSONNestingDepth), within: true},
		{name: "depth above", value: protocolNestedJSON(action.MaxJSONNestingDepth + 1)},
		{name: "exact nodes", value: protocolArrayJSON(action.MaxJSONValueNodes - 1), within: true},
		{name: "nodes above", value: protocolArrayJSON(action.MaxJSONValueNodes)},
		{name: "exact number", value: protocolNumberJSON(action.MaxJSONNumberBytes), within: true},
		{name: "number above", value: protocolNumberJSON(action.MaxJSONNumberBytes + 1)},
	}
	for _, test := range tests {
		for _, channel := range []string{"http", "mcp"} {
			t.Run(test.name+"/"+channel, func(t *testing.T) {
				var extracted json.RawMessage
				if channel == "http" {
					extracted = decodeHTTPBudgetInput(t, test.value)
				} else {
					extracted = decodeMCPBudgetInput(t, test.value)
				}
				err := action.ValidateJSONDocument(extracted)
				if test.within && err != nil {
					t.Fatalf("exact Action boundary rejected after %s extraction: %v", channel, err)
				}
				if !test.within && err == nil {
					t.Fatalf("Action value above boundary accepted after %s extraction", channel)
				}
			})
		}
	}
}

func decodeHTTPBudgetInput(t *testing.T, input json.RawMessage) json.RawMessage {
	t.Helper()
	body := append([]byte(`{"input":`), input...)
	body = append(body, '}')
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	var envelope struct {
		Input json.RawMessage `json:"input"`
	}
	if err := decodeRequestJSON(httptest.NewRecorder(), request, DefaultMaxBodyBytes, &envelope); err != nil {
		t.Fatalf("decode HTTP envelope (%d bytes): %v", len(body), err)
	}
	return envelope.Input
}

func decodeMCPBudgetInput(t *testing.T, input json.RawMessage) json.RawMessage {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"budget","arguments":{"input":%s}}}`, input))
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	rpc, status, rpcErr := decodeMCPRequest(httptest.NewRecorder(), request, DefaultMCPMaxBodyBytes)
	if rpcErr != nil || status != http.StatusOK {
		t.Fatalf("decode MCP envelope (%d bytes) = %d, %#v", len(body), status, rpcErr)
	}
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := decodeMCPParams(rpc.Params, &params); err != nil {
		t.Fatalf("decode MCP params: %v", err)
	}
	var arguments struct {
		Input json.RawMessage `json:"input"`
	}
	if err := decodeMCPParams(params.Arguments, &arguments); err != nil {
		t.Fatalf("decode MCP arguments: %v", err)
	}
	return arguments.Input
}

func protocolNestedJSON(depth int) json.RawMessage {
	return json.RawMessage(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
}

func protocolArrayJSON(values int) json.RawMessage {
	return json.RawMessage("[" + strings.TrimSuffix(strings.Repeat("0,", values), ",") + "]")
}

func protocolNumberJSON(bytes int) json.RawMessage {
	return json.RawMessage("1" + strings.Repeat("0", bytes-1))
}
