package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func FuzzProtocolJSONDecodersFailClosed(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{}`),
		[]byte(`{"input":null}`),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x","arguments":{}}}`),
		[]byte(`{"value":1,"value":2}`),
		[]byte{0xff},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8<<10 {
			return
		}

		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
		var envelope struct {
			Input json.RawMessage `json:"input"`
		}
		_ = decodeRequestJSON(httptest.NewRecorder(), request, 8<<10, &envelope)

		mcpRequest := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(data))
		_, _, _ = decodeMCPRequest(httptest.NewRecorder(), mcpRequest, 8<<10)
	})
}
