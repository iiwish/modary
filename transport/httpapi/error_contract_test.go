package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iiwish/modary/action"
)

func TestActionErrorPublicEnvelopeIsConsistentAcrossHTTPAndMCP(t *testing.T) {
	governed := &action.Error{
		Code:     "INVENTORY.VERSION_CONFLICT",
		Kind:     action.ErrorKindConflict,
		Message:  "inventory version changed",
		ActionID: "inventory.update",
	}

	httpResponse := httptest.NewRecorder()
	writeActionError(httpResponse, "request-http", context.Background(), governed)
	if httpResponse.Code != http.StatusConflict {
		t.Fatalf("HTTP status = %d; body=%s", httpResponse.Code, httpResponse.Body.String())
	}
	var httpEnvelope errorEnvelope
	decodeResponse(t, httpResponse, &httpEnvelope)
	assertPublicActionError(t, httpEnvelope.Error, governed.Code, governed.Kind, governed.Message)
	if httpEnvelope.RequestID != "request-http" || httpEnvelope.Error.RequestID != "request-http" || httpEnvelope.Error.ActionID != governed.ActionID {
		t.Fatalf("HTTP request context = %#v", httpEnvelope)
	}

	mcpResponse := httptest.NewRecorder()
	(&mcpHandler{}).writeToolError(context.Background(), mcpResponse, json.RawMessage(`7`), governed)
	if mcpResponse.Code != http.StatusOK {
		t.Fatalf("MCP status = %d; body=%s", mcpResponse.Code, mcpResponse.Body.String())
	}
	mcpDetail := decodeMCPToolErrorDetail(t, mcpResponse)
	assertPublicActionError(t, mcpDetail, governed.Code, governed.Kind, governed.Message)
	if mcpDetail.ActionID != governed.ActionID {
		t.Fatalf("MCP Action ID = %q, want %q", mcpDetail.ActionID, governed.ActionID)
	}
}

func TestActionErrorChannelsFailClosedOnInvalidPublicEnvelopes(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "built-in kind mismatch",
			err: &action.Error{
				Code: action.CodeValidationFailed, Kind: action.ErrorKindConflict,
				Message: "mismatch secret must not escape",
			},
		},
		{
			name: "invalid message",
			err: &action.Error{
				Code: "INVENTORY.VERSION_CONFLICT", Kind: action.ErrorKindConflict,
				Message: "line one\nmessage secret must not escape",
			},
		},
		{
			name: "oversized message",
			err: &action.Error{
				Code: "INVENTORY.VERSION_CONFLICT", Kind: action.ErrorKindConflict,
				Message: strings.Repeat("s", action.MaxErrorMessageRunes+1),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			httpResponse := httptest.NewRecorder()
			writeActionError(httpResponse, "request-http", context.Background(), test.err)
			if httpResponse.Code != http.StatusInternalServerError {
				t.Fatalf("HTTP status = %d; body=%s", httpResponse.Code, httpResponse.Body.String())
			}
			var httpEnvelope errorEnvelope
			decodeResponse(t, httpResponse, &httpEnvelope)
			assertPublicActionError(t, httpEnvelope.Error, action.CodeInternal, action.ErrorKindInternal, "internal server error")
			if strings.Contains(httpResponse.Body.String(), "secret") || len(httpResponse.Body.String()) > 1024 {
				t.Fatalf("HTTP fallback disclosed invalid envelope: %s", httpResponse.Body.String())
			}

			mcpResponse := httptest.NewRecorder()
			(&mcpHandler{}).writeToolError(context.Background(), mcpResponse, json.RawMessage(`8`), test.err)
			mcpDetail := decodeMCPToolErrorDetail(t, mcpResponse)
			assertPublicActionError(t, mcpDetail, action.CodeInternal, action.ErrorKindInternal, "tool execution failed")
			if strings.Contains(mcpResponse.Body.String(), "secret") || len(mcpResponse.Body.String()) > 1024 {
				t.Fatalf("MCP fallback disclosed invalid envelope: %s", mcpResponse.Body.String())
			}
		})
	}
}

func decodeMCPToolErrorDetail(t *testing.T, response *httptest.ResponseRecorder) publicError {
	t.Helper()
	var envelope struct {
		Result mcpToolResult `json:"result"`
		Error  *mcpRPCError  `json:"error"`
	}
	decodeResponse(t, response, &envelope)
	if envelope.Error != nil || !envelope.Result.IsError || len(envelope.Result.Content) != 1 {
		t.Fatalf("MCP tool error envelope = %#v; body=%s", envelope, response.Body.String())
	}
	var detail publicError
	if err := json.Unmarshal([]byte(envelope.Result.Content[0].Text), &detail); err != nil {
		t.Fatalf("decode MCP error detail: %v; text=%q", err, envelope.Result.Content[0].Text)
	}
	return detail
}

func assertPublicActionError(t *testing.T, got publicError, code string, kind action.ErrorKind, message string) {
	t.Helper()
	if got.Code != code || got.Kind != kind || got.Message != message {
		t.Fatalf("public Action error = %#v; want code=%q kind=%q message=%q", got, code, kind, message)
	}
}
