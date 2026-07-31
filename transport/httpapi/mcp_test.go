package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/internal/testsupport"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/scope"
)

func TestNewMCPValidatesDependenciesAndOptions(t *testing.T) {
	if handler, err := NewMCP(nil, MCPOptions{}); err == nil || handler != nil {
		t.Fatalf("NewMCP(nil) = %#v, %v", handler, err)
	}

	withoutTokens := newHTTPTestApplication(t, true)
	if handler, err := NewMCP(withoutTokens.app, MCPOptions{}); err == nil || handler != nil || !errors.Is(err, appkit.ErrTokensUnavailable) {
		t.Fatalf("NewMCP(missing tokens) = %#v, %v", handler, err)
	}

	application := newMCPTestApplication(t)
	tests := []struct {
		name    string
		options MCPOptions
	}{
		{name: "negative body limit", options: MCPOptions{MaxBodyBytes: -1}},
		{name: "unbounded body limit", options: MCPOptions{MaxBodyBytes: MaximumMCPBodyBytes + 1}},
		{name: "negative timeout", options: MCPOptions{RequestTimeout: -time.Second}},
		{name: "negative concurrency", options: MCPOptions{MaxConcurrentCalls: -1}},
		{name: "unbounded concurrency", options: MCPOptions{MaxConcurrentCalls: MaximumMCPConcurrentCalls + 1}},
		{name: "origin with path", options: MCPOptions{AllowedOrigins: []string{"https://example.test/"}}},
		{name: "origin with credentials", options: MCPOptions{AllowedOrigins: []string{"https://user@example.test"}}},
		{name: "canonical duplicate origin", options: MCPOptions{AllowedOrigins: []string{"https://EXAMPLE.test", "https://example.test:443"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if handler, err := NewMCP(application.app, test.options); err == nil || handler != nil {
				t.Fatalf("NewMCP(%#v) = %#v, %v", test.options, handler, err)
			}
		})
	}

}

func TestAuthenticatedTransportsRejectStoppedApplication(t *testing.T) {
	application := newMCPTestApplication(t)
	if err := application.app.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if handler, err := NewMCP(application.app, MCPOptions{}); err == nil || handler != nil || !errors.Is(err, appkit.ErrApplicationUnavailable) {
		t.Fatalf("NewMCP(stopped) = %#v, %v", handler, err)
	}
	if handler, err := NewAPI(application.app, APIOptions{}); err == nil || handler != nil || !errors.Is(err, appkit.ErrApplicationUnavailable) {
		t.Fatalf("NewAPI(stopped) = %#v, %v", handler, err)
	}
}

func TestRetainedMCPHandlerReportsApplicationShutdownAsUnavailable(t *testing.T) {
	const ping = `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	t.Run("revoked before request", func(t *testing.T) {
		application := newMCPTestApplication(t)
		handler := mustNewMCP(t, application.app, MCPOptions{})
		if err := application.app.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertUnavailableMCPResponse(t, performMCP(t, handler, ping))
	})

	t.Run("in flight authentication canceled by shutdown", func(t *testing.T) {
		application := newMCPTestApplication(t)
		handler := mustNewMCP(t, application.app, MCPOptions{})
		application.tokens.setBlock(true)

		response := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			defer close(done)
			handler.ServeHTTP(response, newMCPRequest(http.MethodPost, ping))
		}()
		application.tokens.waitForBlockedAuthentication(t)
		if err := application.app.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("MCP request did not return after lifecycle cancellation")
		}
		assertUnavailableMCPResponse(t, response)
	})
}

func assertUnavailableMCPResponse(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var envelope mcpRPCResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode unavailable MCP response: %v; body=%s", err, response.Body.String())
	}
	if response.Code != http.StatusServiceUnavailable || envelope.Error == nil || envelope.Error.Code != -32000 {
		t.Fatalf("unavailable MCP response = %d %#v", response.Code, envelope)
	}
}

func TestMCPHTTPBoundaryAuthenticationAndInitialization(t *testing.T) {
	application := newMCPTestApplication(t)
	handler := mustNewMCP(t, application.app, MCPOptions{AllowedOrigins: []string{"https://client.example"}})
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`

	tests := []struct {
		name    string
		method  string
		body    string
		mutate  func(*http.Request)
		status  int
		message string
	}{
		{name: "method", method: http.MethodGet, body: "", status: http.StatusMethodNotAllowed},
		{name: "missing accept", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header.Del("Accept") }, status: http.StatusNotAcceptable},
		{name: "event stream q zero", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header.Set("Accept", "application/json, text/event-stream;q=0") }, status: http.StatusNotAcceptable},
		{name: "specific JSON exclusion overrides wildcard", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header.Set("Accept", "application/json;q=0, text/event-stream, */*;q=1") }, status: http.StatusNotAcceptable},
		{name: "specific event exclusion overrides wildcard", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header.Set("Accept", "application/json, text/event-stream;q=0, */*;q=1") }, status: http.StatusNotAcceptable},
		{name: "content type", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, status: http.StatusUnsupportedMediaType},
		{name: "duplicate content type", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header["Content-Type"] = []string{"application/json", "application/json"} }, status: http.StatusUnsupportedMediaType},
		{name: "missing bearer", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header.Del("Authorization") }, status: http.StatusUnauthorized},
		{name: "duplicate bearer", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header["Authorization"] = []string{"Bearer mcp-token", "Bearer mcp-token"} }, status: http.StatusUnauthorized},
		{name: "invalid bearer", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong-secret") }, status: http.StatusUnauthorized, message: "invalid or expired"},
		{name: "disallowed origin", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header.Set("Origin", "https://other.example") }, status: http.StatusForbidden},
		{name: "malformed origin", method: http.MethodPost, body: initialize, mutate: func(r *http.Request) { r.Header.Set("Origin", "null") }, status: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newMCPRequest(test.method, test.body)
			if test.mutate != nil {
				test.mutate(request)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status || (test.message != "" && !strings.Contains(response.Body.String(), test.message)) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			assertMCPSecurityHeaders(t, response)
			if strings.Contains(response.Body.String(), "wrong-secret") {
				t.Fatalf("credential leaked: %s", response.Body.String())
			}
		})
	}
	application.tokens.setError(errors.New("token database password=secret unavailable"))
	failedAuth := performMCP(t, handler, initialize)
	if failedAuth.Code != http.StatusInternalServerError || strings.Contains(failedAuth.Body.String(), "database") || strings.Contains(failedAuth.Body.String(), "secret") {
		t.Fatalf("operational authentication failure = %d %s", failedAuth.Code, failedAuth.Body.String())
	}
	application.tokens.setError(nil)

	timeoutHandler := mustNewMCP(t, application.app, MCPOptions{AllowedOrigins: []string{"https://client.example"}, RequestTimeout: 10 * time.Millisecond})
	application.tokens.setBlock(true)
	timedOut := performMCP(t, timeoutHandler, initialize)
	application.tokens.setBlock(false)
	if timedOut.Code != http.StatusGatewayTimeout || !strings.Contains(timedOut.Body.String(), "timed out") {
		t.Fatalf("authentication timeout = %d %s", timedOut.Code, timedOut.Body.String())
	}

	request := newMCPRequest(http.MethodPost, initialize)
	request.Header.Set("Origin", "https://CLIENT.example:443")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("initialize = %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Title   string `json:"title"`
				Version string `json:"version"`
			} `json:"serverInfo"`
			Capabilities struct {
				Tools struct {
					ListChanged bool `json:"listChanged"`
				} `json:"tools"`
			} `json:"capabilities"`
		} `json:"result"`
	}
	decodeResponse(t, response, &envelope)
	if envelope.Result.ProtocolVersion != MCPProtocolVersion || envelope.Result.ServerInfo.Name != "consumer-app" || envelope.Result.ServerInfo.Title != "Consumer Application" || envelope.Result.ServerInfo.Version != "2.3.4" || envelope.Result.Capabilities.Tools.ListChanged {
		t.Fatalf("initialize result = %#v", envelope.Result)
	}
}

func TestMCPRejectsInvalidAuthenticatedActorsBeforeDiscovery(t *testing.T) {
	application := newMCPTestApplication(t)
	handler := mustNewMCP(t, application.app, MCPOptions{})
	valid := application.actor
	tests := []struct {
		name   string
		mutate func(*identity.Actor)
	}{
		{name: "missing id", mutate: func(actor *identity.Actor) { actor.ID = "" }},
		{name: "invalid type", mutate: func(actor *identity.Actor) { actor.Type = " agent" }},
		{name: "invalid display name", mutate: func(actor *identity.Actor) { actor.DisplayName = "agent\nsecret" }},
		{name: "invalid scope", mutate: func(actor *identity.Actor) { actor.Scope = scope.Execution{} }},
	}
	methods := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`,
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actor := valid
			test.mutate(&actor)
			application.tokens.setActor(actor)
			defer application.tokens.setActor(valid)
			for _, body := range methods {
				response := performMCP(t, handler, body)
				if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
					t.Fatalf("invalid actor discovery response = %d %s", response.Code, response.Body.String())
				}
			}
		})
	}
}

func TestMCPAuthenticationErrorClassificationDoesNotInvokeHostileMethods(t *testing.T) {
	application := newMCPTestApplication(t)
	handler := mustNewMCP(t, application.app, MCPOptions{})
	hostile := &hostileHTTPBoundaryError{entered: make(chan struct{})}
	application.tokens.setError(hostile)
	defer application.tokens.setError(nil)

	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(response, newMCPRequest(http.MethodPost, `{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("MCP authentication error classification blocked")
	}
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("hostile authentication error response = %d %s", response.Code, response.Body.String())
	}
	select {
	case <-hostile.entered:
		t.Fatal("hostile error Unwrap was invoked")
	default:
	}
}

func TestMCPProtocolVersionAndStrictJSONRPC(t *testing.T) {
	application := newMCPTestApplication(t)
	handler := mustNewMCP(t, application.app, MCPOptions{MaxBodyBytes: 256})

	tests := []struct {
		name   string
		body   string
		mutate func(*http.Request)
		status int
		code   int
		id     string
	}{
		{name: "missing protocol version", body: `{"jsonrpc":"2.0","id":1,"method":"ping"}`, mutate: func(r *http.Request) { r.Header.Del("MCP-Protocol-Version") }, status: http.StatusBadRequest, code: -32600, id: "1"},
		{name: "duplicate protocol version", body: `{"jsonrpc":"2.0","id":1,"method":"ping"}`, mutate: func(r *http.Request) {
			r.Header.Add("MCP-Protocol-Version", MCPProtocolVersion)
		}, status: http.StatusBadRequest, code: -32600, id: "1"},
		{name: "batch", body: `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, status: http.StatusBadRequest, code: -32600},
		{name: "duplicate member", body: `{"jsonrpc":"2.0","id":1,"method":"ping","method":"tools/list"}`, status: http.StatusBadRequest, code: -32700},
		{name: "unknown member", body: `{"jsonrpc":"2.0","id":1,"method":"ping","secret":true}`, status: http.StatusBadRequest, code: -32600, id: "1"},
		{name: "wrong field type", body: `{"jsonrpc":"2.0","id":1,"method":7}`, status: http.StatusBadRequest, code: -32600, id: "1"},
		{name: "invalid id type", body: `{"jsonrpc":"2.0","id":true,"method":"ping"}`, status: http.StatusBadRequest, code: -32600},
		{name: "null id", body: `{"jsonrpc":"2.0","id":null,"method":"ping"}`, status: http.StatusBadRequest, code: -32600},
		{name: "fractional id", body: `{"jsonrpc":"2.0","id":1.5,"method":"ping"}`, status: http.StatusBadRequest, code: -32600},
		{name: "exponent id", body: `{"jsonrpc":"2.0","id":1e3,"method":"ping"}`, status: http.StatusBadRequest, code: -32600},
		{name: "scalar", body: `7`, status: http.StatusBadRequest, code: -32600},
		{name: "malformed", body: `{"jsonrpc":`, status: http.StatusBadRequest, code: -32700},
		{name: "invalid UTF-8", body: `{"jsonrpc":"2.0","id":1,"method":"p` + string([]byte{0xff}) + `ing"}`, status: http.StatusBadRequest, code: -32700},
		{name: "trailing value", body: `{"jsonrpc":"2.0","id":1,"method":"ping"}{}`, status: http.StatusBadRequest, code: -32700},
		{name: "large body", body: `{"jsonrpc":"2.0","id":1,"method":"` + strings.Repeat("x", 300) + `"}`, status: http.StatusRequestEntityTooLarge, code: -32600},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newMCPRequest(http.MethodPost, test.body)
			if test.mutate != nil {
				test.mutate(request)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
			var envelope struct {
				Error *mcpRPCError `json:"error"`
			}
			body := append([]byte(nil), response.Body.Bytes()...)
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatalf("decode response: %v; body=%s", err, body)
			}
			if envelope.Error == nil || envelope.Error.Code != test.code {
				t.Fatalf("error = %#v, want code %d", envelope.Error, test.code)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Fatalf("decode response fields: %v", err)
			}
			if got := string(fields["id"]); got != test.id {
				t.Fatalf("id = %q, want %q; body=%s", got, test.id, response.Body.String())
			}
		})
	}
}

func TestMCPToolDiscoverySchemasAndGovernedExecution(t *testing.T) {
	application := newMCPTestApplication(t)
	handler := mustNewMCP(t, application.app, MCPOptions{})

	listed := performMCP(t, handler, `{"jsonrpc":"2.0","id":"list","method":"tools/list","params":{}}`)
	if listed.Code != http.StatusOK {
		t.Fatalf("tools/list = %d %s", listed.Code, listed.Body.String())
	}
	var listEnvelope struct {
		Result struct {
			Tools []mcpTool `json:"tools"`
		} `json:"result"`
	}
	decodeResponse(t, listed, &listEnvelope)
	if len(listEnvelope.Result.Tools) != 3 {
		t.Fatalf("tools = %#v", listEnvelope.Result.Tools)
	}
	names := make([]string, 0, len(listEnvelope.Result.Tools))
	byOperation := make(map[string]mcpTool)
	for _, tool := range listEnvelope.Result.Tools {
		names = append(names, tool.Name)
		if len(tool.Name) > 128 || tool.Meta["modary/actionId"] == "" || tool.Meta["modary/contractHash"] == "" {
			t.Fatalf("invalid tool metadata: %#v", tool)
		}

		if _, err := action.CompileValidator(tool.InputSchema); err != nil {
			t.Fatalf("compile input schema for %s: %v", tool.Name, err)
		}
		if _, err := action.CompileValidator(tool.OutputSchema); err != nil {
			t.Fatalf("compile output schema for %s: %v", tool.Name, err)
		}
		byOperation[tool.Meta["modary/actionId"]+":"+tool.Meta["modary/operation"]] = tool
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("tool order is not deterministic: %#v", names)
	}
	if _, exists := byOperation["consumer.http-only:execute"]; exists {
		t.Fatal("non-MCP Action was published")
	}
	previewTool := byOperation["consumer.echo:preview"]
	executeTool := byOperation["consumer.echo:execute"]
	readTool := byOperation["consumer.read:execute"]
	if previewTool.Name == "" || executeTool.Name == "" || readTool.Name == "" {
		t.Fatalf("discovered operations = %#v", byOperation)
	}
	assertSchemaRequired(t, executeTool.InputSchema, "input", "plan_hash", "idempotency_key")
	assertSchemaRequired(t, readTool.InputSchema, "input")
	if bytes.Contains(readTool.InputSchema, []byte(`"plan_hash"`)) {
		t.Fatalf("preview-none execute schema exposes plan_hash: %s", readTool.InputSchema)
	}

	preview := performMCP(t, handler, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":%q,"arguments":{"input":{"message":"hello"}}}}`, previewTool.Name))
	if preview.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", preview.Code, preview.Body.String())
	}
	var previewEnvelope struct {
		Result struct {
			Content           []mcpTextContent `json:"content"`
			StructuredContent struct {
				Preview action.Preview `json:"preview"`
			} `json:"structuredContent"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *mcpRPCError `json:"error"`
	}
	decodeResponse(t, preview, &previewEnvelope)
	if previewEnvelope.Error != nil || previewEnvelope.Result.IsError || previewEnvelope.Result.StructuredContent.Preview.PlanHash == "" || len(previewEnvelope.Result.Content) != 1 {
		t.Fatalf("preview envelope = %#v", previewEnvelope)
	}
	assertTextMirrorsStructured(t, previewEnvelope.Result.Content[0].Text, previewEnvelope.Result.StructuredContent)

	execute := performMCP(t, handler, fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":%q,"arguments":{"input":{"message":"hello"},"plan_hash":%q,"idempotency_key":"mcp-once"}}}`, executeTool.Name, previewEnvelope.Result.StructuredContent.Preview.PlanHash))
	var executeEnvelope struct {
		Result struct {
			Content           []mcpTextContent `json:"content"`
			StructuredContent map[string]any   `json:"structuredContent"`
			IsError           bool             `json:"isError"`
		} `json:"result"`
		Error *mcpRPCError `json:"error"`
	}
	decodeResponse(t, execute, &executeEnvelope)
	if execute.Code != http.StatusOK || executeEnvelope.Error != nil || executeEnvelope.Result.IsError || executeEnvelope.Result.StructuredContent["summary"] != "echoed" {
		t.Fatalf("execute = %d %#v; body=%s", execute.Code, executeEnvelope, execute.Body.String())
	}
	assertTextMirrorsStructured(t, executeEnvelope.Result.Content[0].Text, executeEnvelope.Result.StructuredContent)
	plan := application.actionHandler.executedPlan()
	if plan.Channel != action.ChannelMCP || plan.Scope != application.actor.Scope || plan.ActorID != application.actor.ID || plan.ActorType != application.actor.Type {
		t.Fatalf("governed execution plan = %#v", plan)
	}

	invalid := performMCP(t, handler, fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":%q,"arguments":{"input":{"message":"hello"}}}}`, executeTool.Name))
	var invalidEnvelope struct {
		Result mcpToolResult `json:"result"`
		Error  *mcpRPCError  `json:"error"`
	}
	decodeResponse(t, invalid, &invalidEnvelope)
	if invalidEnvelope.Error != nil || !invalidEnvelope.Result.IsError || len(invalidEnvelope.Result.Content) != 1 || !strings.Contains(invalidEnvelope.Result.Content[0].Text, action.CodeValidationFailed) {
		t.Fatalf("invalid tool result = %#v; body=%s", invalidEnvelope, invalid.Body.String())
	}

	unknown := performMCP(t, handler, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"missing","arguments":{}}}`)
	var unknownEnvelope struct {
		Error *mcpRPCError `json:"error"`
	}
	decodeResponse(t, unknown, &unknownEnvelope)
	if unknownEnvelope.Error == nil || unknownEnvelope.Error.Code != -32602 {
		t.Fatalf("unknown tool response = %s", unknown.Body.String())
	}
}

func TestMCPActionInputPresenceAndNumberClassification(t *testing.T) {
	application := newMCPTestApplication(t)
	handler := mustNewMCP(t, application.app, MCPOptions{})
	toolName := mcpToolName("consumer.read", mcpExecute)

	call := func(id, arguments string) *httptest.ResponseRecorder {
		t.Helper()
		return performMCP(t, handler, fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%q,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
			id, toolName, arguments,
		))
	}

	missing := call("missing", `{}`)
	if !strings.Contains(missing.Body.String(), action.CodeValidationFailed) {
		t.Fatalf("missing input response = %s", missing.Body.String())
	}
	if events := application.audit.snapshot(); len(events) != 0 {
		t.Fatalf("missing MCP input reached Runtime audit: %#v", events)
	}

	for _, test := range []struct {
		name     string
		input    json.RawMessage
		wantCode string
		wantHash bool
	}{
		{name: "null", input: json.RawMessage(`null`), wantCode: action.CodeValidationFailed, wantHash: true},
		{name: "empty object", input: json.RawMessage(`{}`), wantCode: action.CodeValidationFailed, wantHash: true},
		{name: "exact number", input: protocolNumberJSON(action.MaxJSONNumberBytes), wantCode: action.CodeValidationFailed, wantHash: true},
		{name: "number above", input: protocolNumberJSON(action.MaxJSONNumberBytes + 1), wantCode: action.CodeLimitExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := call(test.name, `{"input":`+string(test.input)+`}`)
			if !strings.Contains(response.Body.String(), test.wantCode) {
				t.Fatalf("response = %s, want %s", response.Body.String(), test.wantCode)
			}
			event, ok := application.audit.last()
			if !ok || event.Decision != "rejected" || event.ErrorCode != test.wantCode || (event.InputHash != "") != test.wantHash {
				t.Fatalf("audit = %#v, present=%t", event, ok)
			}
		})
	}
	if events := application.audit.snapshot(); len(events) != 4 {
		t.Fatalf("MCP Runtime audit events = %d, want 4: %#v", len(events), events)
	}
}

func TestMCPActionJSONBoundaryMatrixReachesRuntimeWithoutProtocolBudgetLeakage(t *testing.T) {
	for _, boundary := range transportActionJSONBoundaries() {
		t.Run(boundary.name, func(t *testing.T) {
			application := newMCPTestApplicationWithInputSchema(t, json.RawMessage(`true`))
			handler := mustNewMCP(t, application.app, MCPOptions{})
			toolName := mcpToolName("consumer.read", mcpExecute)

			call := func(id string, input json.RawMessage) (*httptest.ResponseRecorder, struct {
				Result mcpToolResult `json:"result"`
				Error  *mcpRPCError  `json:"error"`
			}) {
				t.Helper()
				body := append([]byte(`{"jsonrpc":"2.0","id":`+fmt.Sprintf("%q", id)+`,"method":"tools/call","params":{"name":`+fmt.Sprintf("%q", toolName)+`,"arguments":{"input":`), input...)
				body = append(body, '}', '}', '}')
				response := performMCP(t, handler, string(body))
				var envelope struct {
					Result mcpToolResult `json:"result"`
					Error  *mcpRPCError  `json:"error"`
				}
				decodeResponse(t, response, &envelope)
				return response, envelope
			}

			exact, exactEnvelope := call("exact", boundary.exact)
			if exact.Code != http.StatusOK || exactEnvelope.Error != nil || exactEnvelope.Result.IsError {
				t.Fatalf("exact Action boundary = %d %#v; body=%s", exact.Code, exactEnvelope, exact.Body.String())
			}
			if got := application.actionHandler.executionCount(); got != 1 {
				t.Fatalf("exact Action boundary Execute calls = %d, want 1", got)
			}

			above, aboveEnvelope := call("above", boundary.above)
			if above.Code != http.StatusOK || aboveEnvelope.Error != nil || !aboveEnvelope.Result.IsError ||
				len(aboveEnvelope.Result.Content) != 1 || !strings.Contains(aboveEnvelope.Result.Content[0].Text, action.CodeLimitExceeded) {
				t.Fatalf("above Action boundary = %d %#v; body=%s", above.Code, aboveEnvelope, above.Body.String())
			}
			if got := application.actionHandler.executionCount(); got != 1 {
				t.Fatalf("above Action boundary reached Handler.Execute: calls=%d", got)
			}

			events := application.audit.snapshot()
			if len(events) != 2 {
				t.Fatalf("Runtime audit events = %d, want exact and above: %#v", len(events), events)
			}
			if events[0].Decision != "allowed" || events[0].ErrorCode != "" || events[0].InputHash == "" {
				t.Fatalf("exact Action audit = %#v", events[0])
			}
			if events[1].Decision != "rejected" || events[1].ErrorCode != action.CodeLimitExceeded || events[1].InputHash != "" {
				t.Fatalf("above Action audit = %#v", events[1])
			}
		})
	}
}

func TestMCPToolSchemasPreserveActionLocalReferenceRoots(t *testing.T) {
	descriptor := testActionDescriptor("consumer.local-refs", []action.Channel{action.ChannelMCP}, action.PreviewRequired)
	descriptor.InputSchema = json.RawMessage(`{
		"definitions":{"node":{
			"type":"object",
			"properties":{
				"value":{"const":"input-ok"},
				"next":{"oneOf":[{"type":"null"},{"$ref":"#"}]}
			},
			"required":["value"],
			"additionalProperties":false
		}},
		"$ref":"#/definitions/node"
	}`)
	descriptor.PreviewSchema = json.RawMessage(`{
		"definitions":{"row/count":{"type":"integer","minimum":1}},
		"$ref":"#/definitions/row~1count"
	}`)
	descriptor.OutputSchema = json.RawMessage(`{
		"definitions":{"result~value":{"type":"string","const":"output-ok"}},
		"$ref":"#/definitions/result~0value"
	}`)
	prepared, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatalf("PrepareDescriptor() error = %v", err)
	}
	entry := action.CatalogEntry{Descriptor: prepared.Descriptor(), ContractHash: prepared.ContractHash()}

	previewTool, err := buildMCPTool(entry, mcpPreview)
	if err != nil {
		t.Fatalf("build preview MCP tool: %v", err)
	}
	executeTool, err := buildMCPTool(entry, mcpExecute)
	if err != nil {
		t.Fatalf("build execute MCP tool: %v", err)
	}

	planHash := "sha256:" + strings.Repeat("0", 64)
	inputByOperation := map[mcpOperation]json.RawMessage{
		mcpPreview: json.RawMessage(`{"input":{"value":"input-ok","next":{"value":"input-ok","next":null}}}`),
		mcpExecute: json.RawMessage(fmt.Sprintf(`{"input":{"value":"input-ok","next":{"value":"input-ok","next":null}},"plan_hash":%q}`, planHash)),
	}
	invalidInputByOperation := map[mcpOperation]json.RawMessage{
		mcpPreview: json.RawMessage(`{"input":{"value":"wrong"}}`),
		mcpExecute: json.RawMessage(fmt.Sprintf(`{"input":{"value":"wrong"},"plan_hash":%q}`, planHash)),
	}
	for _, tool := range []mcpTool{previewTool, executeTool} {
		validator, err := action.CompileValidator(tool.InputSchema)
		if err != nil {
			t.Fatalf("compile %s input schema: %v", tool.operation, err)
		}
		if err := validator.Validate(inputByOperation[tool.operation]); err != nil {
			t.Fatalf("%s input rejected recursive local ref: %v; schema=%s", tool.operation, err, tool.InputSchema)
		}
		if err := validator.Validate(invalidInputByOperation[tool.operation]); action.ErrorCode(err) != action.CodeValidationFailed {
			t.Fatalf("%s invalid input error = %v", tool.operation, err)
		}
	}

	previewOutput, err := action.CompileValidator(previewTool.OutputSchema)
	if err != nil {
		t.Fatal(err)
	}
	validPreview := json.RawMessage(fmt.Sprintf(`{"preview":{"plan_hash":%q,"summary":1,"impact":{},"expires_at":"2026-07-31T00:00:00Z"}}`, planHash))
	if err := previewOutput.Validate(validPreview); err != nil {
		t.Fatalf("preview output rejected local ref: %v; schema=%s", err, previewTool.OutputSchema)
	}
	invalidPreview := json.RawMessage(fmt.Sprintf(`{"preview":{"plan_hash":%q,"summary":0,"impact":{},"expires_at":"2026-07-31T00:00:00Z"}}`, planHash))
	if err := previewOutput.Validate(invalidPreview); action.ErrorCode(err) != action.CodeValidationFailed {
		t.Fatalf("invalid preview output error = %v", err)
	}

	executeOutput, err := action.CompileValidator(executeTool.OutputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if err := executeOutput.Validate(json.RawMessage(`{"result":"output-ok"}`)); err != nil {
		t.Fatalf("execute output rejected local ref: %v; schema=%s", err, executeTool.OutputSchema)
	}
	if err := executeOutput.Validate(json.RawMessage(`{"result":"wrong"}`)); action.ErrorCode(err) != action.CodeValidationFailed {
		t.Fatalf("invalid execute output error = %v", err)
	}
}

func TestMCPActionSchemaRebasingDoesNotRewriteLiteralReferenceData(t *testing.T) {
	embedded, err := rebaseMCPActionSchema(json.RawMessage(`{
		"definitions":{"value":{"type":"object","properties":{
			"literal":{"const":{"$ref":"#"}}
		},"required":["literal"],"additionalProperties":false}},
		"$ref":"#/definitions/value"
	}`), "#/properties/input")
	if err != nil {
		t.Fatal(err)
	}
	root := embedded.(map[string]any)
	if got := root["$ref"]; got != "#/properties/input/definitions/value" {
		t.Fatalf("schema $ref = %#v", got)
	}
	definition := root["definitions"].(map[string]any)["value"].(map[string]any)
	literal := definition["properties"].(map[string]any)["literal"].(map[string]any)
	if got := literal["const"].(map[string]any)["$ref"]; got != "#" {
		t.Fatalf("literal $ref data was rewritten to %#v", got)
	}
}

func TestMCPActionSchemaRebasingPreservesHiddenMultiHopTargets(t *testing.T) {
	targetName := "percent%field space/\u96ea~"
	sourcePointer := "/" + strings.ReplaceAll(strings.ReplaceAll(targetName, "~", "~0"), "/", "~1")
	sourceReference := (&url.URL{Fragment: sourcePointer}).String()
	source := mustMarshalMCPTestSchema(t, map[string]any{
		"$ref": "#/const",
		"const": map[string]any{
			"$ref": sourceReference,
		},
		targetName:              map[string]any{"type": "string"},
		mcpRefTargetsAnnotation: "literal collision",
	})
	sourceValidator, err := action.CompileValidator(source)
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := rebaseMCPActionSchema(source, "#/properties/input")
	if err != nil {
		t.Fatal(err)
	}
	root := embedded.(map[string]any)
	if got := root["const"].(map[string]any)["$ref"]; got != sourceReference {
		t.Fatalf("dual-role literal changed from %q to %#v", sourceReference, got)
	}
	if root[mcpRefTargetsAnnotation] != "literal collision" {
		t.Fatal("framework hidden-target annotation overwrote source data")
	}
	generatedName := mcpRefTargetsAnnotation + "-1"
	if _, ok := root[generatedName].(map[string]any); !ok {
		t.Fatalf("generated hidden-target annotation %q missing", generatedName)
	}
	rebasedReference, ok := root["$ref"].(string)
	if !ok || !strings.Contains(rebasedReference, generatedName) {
		t.Fatalf("root hidden reference was not rebased: %#v", root["$ref"])
	}
	wrapper := mustMarshalMCPTestSchema(t, map[string]any{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"type":                 "object",
		"properties":           map[string]any{"input": embedded},
		"required":             []string{"input"},
		"additionalProperties": false,
	})
	if err := compileMCPToolSchema(wrapper); err != nil {
		t.Fatalf("compile hidden-target MCP wrapper: %v", err)
	}
	wrapperValidator, err := action.CompileValidator(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		value   json.RawMessage
		wrapped json.RawMessage
	}{
		{value: json.RawMessage(`"ok"`), wrapped: json.RawMessage(`{"input":"ok"}`)},
		{value: json.RawMessage(`1`), wrapped: json.RawMessage(`{"input":1}`)},
	} {
		sourceErr := sourceValidator.Validate(test.value)
		wrapperErr := wrapperValidator.Validate(test.wrapped)
		if (sourceErr == nil) != (wrapperErr == nil) {
			t.Fatalf("Action error = %v, MCP error = %v", sourceErr, wrapperErr)
		}
	}
}

func TestMCPActionSchemaRebasingEncodesURIFragmentPointers(t *testing.T) {
	targetName := "percent%field space/\u96ea~"
	sourcePointer := "/definitions/" + strings.ReplaceAll(strings.ReplaceAll(targetName, "~", "~0"), "/", "~1")
	sourceReference := (&url.URL{Fragment: sourcePointer}).String()
	source := mustMarshalMCPTestSchema(t, map[string]any{
		"definitions": map[string]any{
			targetName: map[string]any{"type": "string"},
		},
		"$ref": sourceReference,
	})
	embedded, err := rebaseMCPActionSchema(source, "#/properties/input")
	if err != nil {
		t.Fatal(err)
	}
	reference := embedded.(map[string]any)["$ref"].(string)
	for _, escaped := range []string{"%25", "%20", "%E9%9B%AA", "~0", "~1"} {
		if !strings.Contains(reference, escaped) {
			t.Fatalf("rebased reference %q does not contain escaped token %q", reference, escaped)
		}
	}
	wrapper := mustMarshalMCPTestSchema(t, map[string]any{
		"type":       "object",
		"properties": map[string]any{"input": embedded},
		"required":   []string{"input"},
	})
	validator, err := action.CompileValidator(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.Validate(json.RawMessage(`{"input":"ok"}`)); err != nil {
		t.Fatalf("percent-encoded pointer rejected valid input: %v", err)
	}
	if err := validator.Validate(json.RawMessage(`{"input":1}`)); action.ErrorCode(err) != action.CodeValidationFailed {
		t.Fatalf("percent-encoded pointer invalid input error = %v", err)
	}
}

func TestMCPActionSchemaRebasingPreservesCompleteRegisteredSchemaLanguage(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  json.RawMessage
		want any
	}{
		{name: "true boolean", raw: json.RawMessage(`true`), want: true},
		{name: "false boolean", raw: json.RawMessage(`false`), want: false},
		{name: "unconstrained object", raw: json.RawMessage(`{}`), want: map[string]any{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			embedded, err := rebaseMCPActionSchema(test.raw, "#/properties/input")
			if err != nil {
				t.Fatalf("rebase valid Action schema: %v", err)
			}
			if !reflect.DeepEqual(embedded, test.want) {
				t.Fatalf("embedded schema = %#v, want %#v", embedded, test.want)
			}
		})
	}
}

func TestMCPRuntimeValidationLeavesFullByteBudgetForEmbeddedActionInput(t *testing.T) {
	descriptor := testActionDescriptor("budget.value", []action.Channel{action.ChannelMCP}, action.PreviewOptional)
	descriptor.InputSchema = json.RawMessage(`{"type":"string"}`)
	prepared, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := buildMCPTool(action.CatalogEntry{Descriptor: prepared.Descriptor(), ContractHash: prepared.ContractHash()}, mcpPreview)
	if err != nil {
		t.Fatal(err)
	}
	input := json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-2) + `"`)
	arguments := append([]byte(`{"input":`), input...)
	arguments = append(arguments, '}')

	// The MCP wrapper is larger than one Action document. Runtime validates the
	// extracted value, not the wrapper, against the Action byte budget.
	if validator, err := action.CompileValidator(tool.InputSchema); err != nil {
		t.Fatal(err)
	} else if err := validator.Validate(arguments); !action.IsCode(err, action.CodeLimitExceeded) {
		t.Fatalf("wrapper validation error = %v", err)
	}
	decoded, err := decodeMCPActionArguments(tool, arguments)
	if err != nil || !bytes.Equal(decoded.Input, input) {
		t.Fatalf("decode full-budget Action input = %d bytes, %v", len(decoded.Input), err)
	}

	tooLarge := append(append([]byte(`{"input":"`), bytes.Repeat([]byte{'x'}, int(action.MaxJSONDocumentBytes)-1)...), '"', '}')
	decoded, err = decodeMCPActionArguments(tool, tooLarge)
	if err != nil || len(decoded.Input) != int(action.MaxJSONDocumentBytes)+1 {
		t.Fatalf("protocol decode oversized Action input = %d bytes, %v", len(decoded.Input), err)
	}

	application := newMCPTestApplication(t)
	handler := mustNewMCP(t, application.app, MCPOptions{})
	response := performMCP(t, handler, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":"budget","method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		mcpToolName("consumer.echo", mcpPreview), tooLarge,
	))
	if !bytes.Contains(response.Body.Bytes(), []byte(action.CodeLimitExceeded)) {
		t.Fatalf("oversized Action input response = %s", response.Body.String())
	}
	event, ok := application.audit.last()
	if !ok || event.Decision != "rejected" || event.ErrorCode != action.CodeLimitExceeded {
		t.Fatalf("oversized Action input audit = %#v, present=%t", event, ok)
	}
}

func TestMCPWrapperSchemasUseProtocolBudgetOutsideActionBudget(t *testing.T) {
	byteDescription := strings.Repeat("x", int(action.MaxJSONDocumentBytes)-96)
	deepSchema := json.RawMessage(`{"type":"string"}`)
	for range action.MaxJSONNestingDepth - 1 {
		deepSchema = append(append([]byte(`{"type":"array","items":`), deepSchema...), '}')
	}
	var nodeDefaults strings.Builder
	for value := range action.MaxJSONValueNodes - 3 {
		if value > 0 {
			nodeDefaults.WriteByte(',')
		}
		nodeDefaults.WriteString("true")
	}

	tests := map[string]json.RawMessage{
		"bytes": json.RawMessage(`{"type":"string","description":"` + byteDescription + `"}`),
		"depth": deepSchema,
		"nodes": json.RawMessage(`{"default":[` + nodeDefaults.String() + `]}`),
	}
	for name, inputSchema := range tests {
		t.Run(name, func(t *testing.T) {
			descriptor := testActionDescriptor("budget.schema-"+name, []action.Channel{action.ChannelMCP}, action.PreviewNone)
			descriptor.InputSchema = inputSchema
			prepared, err := action.PrepareDescriptor(descriptor)
			if err != nil {
				t.Fatalf("prepare boundary Action schema: %v", err)
			}
			tool, err := buildMCPTool(action.CatalogEntry{
				Descriptor:   prepared.Descriptor(),
				ContractHash: prepared.ContractHash(),
			}, mcpExecute)
			if err != nil {
				t.Fatalf("build MCP wrapper at Action %s boundary: %v", name, err)
			}
			if err := action.ValidateJSONDocument(tool.InputSchema); err == nil {
				t.Fatalf("MCP wrapper unexpectedly remained inside Action %s budget", name)
			}
			if err := compileMCPToolSchema(tool.InputSchema); err != nil {
				t.Fatalf("compile MCP wrapper with protocol budget: %v", err)
			}
		})
	}
}

func TestMCPCompileBudgetAccountsForCopiedHiddenReferenceTargets(t *testing.T) {
	var literalNodes strings.Builder
	for index := 0; index < action.MaxJSONValueNodes-6; index++ {
		if index > 0 {
			literalNodes.WriteByte(',')
		}
		literalNodes.WriteString("true")
	}
	inputSchema := json.RawMessage(`{"$ref":"#/default","default":{"type":"string","x-data":[` + literalNodes.String() + `]}}`)
	if err := action.ValidateJSONDocument(inputSchema); err != nil {
		t.Fatalf("hidden-target Action boundary is invalid: %v", err)
	}
	descriptor := testActionDescriptor("budget.hidden-ref-copy", []action.Channel{action.ChannelMCP}, action.PreviewNone)
	descriptor.InputSchema = inputSchema
	prepared, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatalf("prepare hidden-target Action schema: %v", err)
	}
	tool, err := buildMCPTool(action.CatalogEntry{
		Descriptor:   prepared.Descriptor(),
		ContractHash: prepared.ContractHash(),
	}, mcpExecute)
	if err != nil {
		t.Fatalf("build MCP wrapper with copied hidden target: %v", err)
	}
	if err := compileMCPToolSchema(tool.InputSchema); err != nil {
		t.Fatalf("compile MCP wrapper with copied hidden target: %v", err)
	}
}

func TestMCPWrapperProfileAdmitsEveryExactBoundaryActionSchema(t *testing.T) {
	tests := map[string]json.RawMessage{
		"schema nodes": mcpSchemaNodeBoundary(t, action.MaxSchemaNodes),
		"collection entries": mustMarshalMCPTestSchema(t, map[string]any{
			"properties": mcpBooleanProperties(action.MaxSchemaCollectionEntries),
		}),
		"enum values": mustMarshalMCPTestSchema(t, map[string]any{
			"enum": mcpEnumValues(action.MaxSchemaEnumValues),
		}),
		"literal bytes": mustMarshalMCPTestSchema(t, map[string]any{
			"const": strings.Repeat("x", action.MaxSchemaLiteralBytes-2),
		}),
		"pattern bytes": mustMarshalMCPTestSchema(t, map[string]any{
			"pattern": strings.Repeat("a", action.MaxSchemaPatternBytes),
		}),
		"same instance visits": mcpSameInstanceBoundary(t),
		"numeric compile work": mcpNumericWorkBoundary(t),
	}
	for name, inputSchema := range tests {
		t.Run(name, func(t *testing.T) {
			descriptor := testActionDescriptor("budget.profile-"+strings.ReplaceAll(name, " ", "-"), []action.Channel{action.ChannelMCP}, action.PreviewNone)
			descriptor.InputSchema = inputSchema
			prepared, err := action.PrepareDescriptor(descriptor)
			if err != nil {
				t.Fatalf("prepare exact-boundary Action schema: %v", err)
			}
			tool, err := buildMCPTool(action.CatalogEntry{
				Descriptor:   prepared.Descriptor(),
				ContractHash: prepared.ContractHash(),
			}, mcpExecute)
			if err != nil {
				t.Fatalf("build MCP wrapper around exact-boundary Action schema: %v", err)
			}
			if err := compileMCPToolSchema(tool.InputSchema); err != nil {
				t.Fatalf("compile bounded MCP wrapper: %v", err)
			}
			if name == "schema nodes" {
				if _, err := action.CompileValidator(tool.InputSchema); err == nil || !strings.Contains(err.Error(), "schema nodes") {
					t.Fatalf("wrapper did not prove separate node profile: %v", err)
				}
			}
		})
	}
}

func mcpNumericWorkBoundary(t *testing.T) json.RawMessage {
	t.Helper()
	return mustMarshalMCPTestSchema(t, map[string]any{
		"allOf": []any{
			map[string]any{"maximum": json.Number("1e8191")},
			map[string]any{"maximum": json.Number(strings.Repeat("9", 127))},
			map[string]any{"maximum": json.Number(strings.Repeat("9", 14))},
			map[string]any{"maximum": json.Number("9999")},
			map[string]any{"maxItems": json.Number("0")},
		},
	})
}

func mcpSchemaNodeBoundary(t *testing.T, nodes int) json.RawMessage {
	t.Helper()
	root := map[string]any{"properties": map[string]any{}}
	properties := root["properties"].(map[string]any)
	remaining := nodes - 1
	for index := 0; remaining > 0; index++ {
		child := map[string]any{"properties": map[string]any{}}
		properties[fmt.Sprintf("p%03d", index)] = child
		remaining--
		nested := child["properties"].(map[string]any)
		for childIndex := 0; childIndex < 3 && remaining > 0; childIndex++ {
			nested[fmt.Sprintf("n%d", childIndex)] = true
			remaining--
		}
	}
	return mustMarshalMCPTestSchema(t, root)
}

func mcpBooleanProperties(entries int) map[string]any {
	properties := make(map[string]any, entries)
	for index := 0; index < entries; index++ {
		properties[fmt.Sprintf("p%03d", index)] = true
	}
	return properties
}

func mcpEnumValues(entries int) []any {
	values := make([]any, entries)
	for index := range values {
		values[index] = fmt.Sprintf("v%03d", index)
	}
	return values
}

func mcpSameInstanceBoundary(t *testing.T) json.RawMessage {
	t.Helper()
	branches := make([]any, action.MaxSchemaCollectionEntries)
	for index := range branches {
		branches[index] = map[string]any{}
	}
	nestedCount := action.MaxSchemaSameInstanceVisits - 1 - action.MaxSchemaCollectionEntries
	nested := make([]any, nestedCount)
	for index := range nested {
		nested[index] = map[string]any{}
	}
	branches[0] = map[string]any{"allOf": nested}
	return mustMarshalMCPTestSchema(t, map[string]any{"allOf": branches})
}

func mustMarshalMCPTestSchema(t *testing.T, schema any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestMCPIdempotencySchemaAndRuntimeSharePortableGrammar(t *testing.T) {
	descriptor := testActionDescriptor("budget.idempotency", []action.Channel{action.ChannelMCP}, action.PreviewNone)
	descriptor.RequiresIdempotency = true
	prepared, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := buildMCPTool(action.CatalogEntry{
		Descriptor:   prepared.Descriptor(),
		ContractHash: prepared.ContractHash(),
	}, mcpExecute)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := action.CompileValidator(tool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	arguments := func(key string) json.RawMessage {
		encoded, err := json.Marshal(map[string]any{
			"input":           map[string]any{"message": "hello"},
			"idempotency_key": key,
		})
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	valid := arguments("retry-01.scope/value+next")
	if err := validator.Validate(valid); err != nil {
		t.Fatalf("schema rejected valid key: %v", err)
	}
	if _, err := decodeMCPActionArguments(tool, valid); err != nil {
		t.Fatalf("runtime rejected valid key: %v", err)
	}
	for _, key := range []string{
		" leading",
		"trailing ",
		"line\nbreak",
		"unicode-\u00e9",
		strings.Repeat("x", action.MaxIdempotencyKeyBytes+1),
	} {
		raw := arguments(key)
		if err := validator.Validate(raw); err == nil {
			t.Errorf("schema accepted invalid key %q", key)
		}
		if _, err := decodeMCPActionArguments(tool, raw); err == nil {
			t.Errorf("runtime accepted invalid key %q", key)
		}
	}
}

func TestMCPOutputSchemasPublishRuntimeMetadataBounds(t *testing.T) {
	descriptor := testActionDescriptor("budget.output", []action.Channel{action.ChannelMCP}, action.PreviewOptional)
	prepared, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	entry := action.CatalogEntry{Descriptor: prepared.Descriptor(), ContractHash: prepared.ContractHash()}
	previewTool, err := buildMCPTool(entry, mcpPreview)
	if err != nil {
		t.Fatal(err)
	}
	executeTool, err := buildMCPTool(entry, mcpExecute)
	if err != nil {
		t.Fatal(err)
	}
	previewValidator, err := action.CompileValidator(previewTool.OutputSchema)
	if err != nil {
		t.Fatal(err)
	}
	executeValidator, err := action.CompileValidator(executeTool.OutputSchema)
	if err != nil {
		t.Fatal(err)
	}
	planHash := "sha256:" + strings.Repeat("0", 64)
	resources := make([]string, audit.MaxResources+1)
	for index := range resources {
		resources[index] = fmt.Sprintf("resource-%d", index)
	}
	invalidPreview, err := json.Marshal(map[string]any{"preview": map[string]any{
		"plan_hash":  planHash,
		"summary":    map[string]any{"matched_rows": 1},
		"impact":     map[string]any{"rows": 1, "resources": resources},
		"expires_at": "2026-07-31T00:00:00Z",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := previewValidator.Validate(invalidPreview); err == nil {
		t.Fatal("preview schema accepted resources above the Runtime limit")
	}
	invalidExecute, err := json.Marshal(map[string]any{
		"result":  map[string]any{"echo": "ok"},
		"summary": strings.Repeat("x", audit.MaxSummaryRunes+1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := executeValidator.Validate(invalidExecute); err == nil {
		t.Fatal("execute schema accepted summary above the Runtime limit")
	}
}

func TestMCPNotificationsPingPanicRecoveryAndErrorRedaction(t *testing.T) {
	application := newMCPTestApplication(t)
	handler := mustNewMCP(t, application.app, MCPOptions{})

	for _, body := range []string{
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{"_meta":{"trace":"one"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1,"reason":"done","_meta":{}}}`,
		`{"jsonrpc":"2.0","method":"unknown/notification","params":{}}`,
	} {
		response := performMCP(t, handler, body)
		if response.Code != http.StatusAccepted || response.Body.Len() != 0 {
			t.Fatalf("notification = %d %q", response.Code, response.Body.String())
		}
	}
	for _, body := range []string{
		`{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{"unknown":true}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":null}}`,
		`{"jsonrpc":"2.0","method":"ping"}`,
		`{"jsonrpc":"2.0","method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{}}`,
	} {
		response := performMCP(t, handler, body)
		if response.Code != http.StatusBadRequest || response.Body.Len() != 0 {
			t.Fatalf("rejected notification = %d %q", response.Code, response.Body.String())
		}
	}
	ping := performMCP(t, handler, `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"_meta":{"trace":"one"}}}`)
	if ping.Code != http.StatusOK || !strings.Contains(ping.Body.String(), `"result":{}`) {
		t.Fatalf("ping = %d %s", ping.Code, ping.Body.String())
	}

	response := httptest.NewRecorder()
	server := handler.(*mcpHandler)
	server.writeToolError(context.Background(), response, json.RawMessage(`1`), &action.Error{Code: action.CodeInternal, Message: "database password=secret", Cause: errors.New("driver secret")})
	if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "database") || !strings.Contains(response.Body.String(), action.CodeInternal) {
		t.Fatalf("internal tool error leaked: %s", response.Body.String())
	}

	application.tokens.setPanic(true)
	panicked := performMCP(t, handler, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	if panicked.Code != http.StatusInternalServerError || strings.Contains(panicked.Body.String(), "token panic secret") {
		t.Fatalf("panic response = %d %s", panicked.Code, panicked.Body.String())
	}
}

type mcpTestApplication struct {
	app           *appkit.Application
	actor         identity.Actor
	tokens        *mcpTestTokens
	actionHandler *testActionHandler
	audit         *mcpAuditRecorder
}

func newMCPTestApplication(t *testing.T) *mcpTestApplication {
	return newMCPTestApplicationWithInputSchema(t, nil)
}

func newMCPTestApplicationWithInputSchema(t *testing.T, inputSchema json.RawMessage) *mcpTestApplication {
	t.Helper()
	executionScope := scope.Must("tenant", "mcp-test")
	actor := identity.Actor{ID: "agent-1", Type: "agent", DisplayName: "Test Agent", Scope: executionScope}
	tokens := &mcpTestTokens{actor: actor, start: make(chan struct{})}
	actionHandler := &testActionHandler{}
	auditRecorder := &mcpAuditRecorder{}
	manifest := module.Manifest{
		SchemaVersion: module.SchemaVersion, ID: "consumer-owned-module", Version: "1.0.0", Type: module.ModuleTypeFeature,
		Provides: []module.Capability{
			module.CapabilityDatabase,
			module.CapabilityAuthorization,
			module.CapabilityAudit,
			module.CapabilityIdentity,
			"consumer",
		},
	}
	start := func(_ context.Context, install module.Scope) error {
		providers := []func() error{
			func() error {
				return moduleassembly.ProvideActionPersistence(install, testsupport.NewMemoryPlanStore(), testsupport.NewMemoryIdempotencyStore(), testsupport.DirectTransactions{})
			},
			func() error { return module.Provide(install, module.Authorizer(), authz.Authorizer(testAuthorizer{})) },
			func() error {
				return module.Provide(install, module.AuditHook(), audit.Hook(auditRecorder))
			},
			func() error {
				return module.Provide(install, module.TokenAuthenticator(), identity.TokenAuthenticator(tokens))
			},
		}
		for _, provide := range providers {
			if err := provide(); err != nil {
				return err
			}
		}
		return nil
	}
	echo := testActionDescriptor("consumer.echo", []action.Channel{action.ChannelMCP, action.ChannelHTTP}, action.PreviewRequired)
	echo.RequiresIdempotency = true
	readDescriptor := testActionDescriptor("consumer.read", []action.Channel{action.ChannelMCP}, action.PreviewNone)
	if inputSchema != nil {
		readDescriptor.InputSchema = append(json.RawMessage(nil), inputSchema...)
	}
	bindings := []module.ActionBinding{
		{Descriptor: echo, NewHandler: func(context.Context, module.Resolver) (action.Handler, error) { return actionHandler, nil }},
		{Descriptor: readDescriptor, NewHandler: func(context.Context, module.Resolver) (action.Handler, error) { return actionHandler, nil }},
		{Descriptor: testActionDescriptor("consumer.http-only", []action.Channel{action.ChannelHTTP}, action.PreviewNone), NewHandler: func(context.Context, module.Resolver) (action.Handler, error) { return actionHandler, nil }},
	}
	application, err := appkit.Start(context.Background(), appkit.Definition{
		Metadata: appkit.Metadata{ID: "consumer-app", Name: "Consumer Application", Version: "2.3.4"},
		Modules:  []module.Registration{module.Register(manifest, start, bindings...)},
	}, appkit.Options{})
	if err != nil {
		t.Fatalf("appkit.Start() error = %v", err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	return &mcpTestApplication{app: application, actor: actor, tokens: tokens, actionHandler: actionHandler, audit: auditRecorder}
}

type mcpAuditRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (recorder *mcpAuditRecorder) Record(_ context.Context, event audit.Event) error {
	recorder.mu.Lock()
	recorder.events = append(recorder.events, event)
	recorder.mu.Unlock()
	return nil
}

func (recorder *mcpAuditRecorder) last() (audit.Event, bool) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.events) == 0 {
		return audit.Event{}, false
	}
	return recorder.events[len(recorder.events)-1], true
}

func (recorder *mcpAuditRecorder) snapshot() []audit.Event {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]audit.Event(nil), recorder.events...)
}

type mcpTestTokens struct {
	mu        sync.Mutex
	actor     identity.Actor
	panic     bool
	err       error
	block     bool
	start     chan struct{}
	startOnce sync.Once
}

func (tokens *mcpTestTokens) AuthenticateToken(ctx context.Context, token string) (identity.Actor, error) {
	tokens.mu.Lock()
	panicCall := tokens.panic
	err := tokens.err
	block := tokens.block
	actor := tokens.actor
	tokens.mu.Unlock()
	if panicCall {
		panic("token panic secret")
	}
	if block {
		tokens.startOnce.Do(func() { close(tokens.start) })
		<-ctx.Done()
		return identity.Actor{}, ctx.Err()
	}
	if err != nil {
		return identity.Actor{}, err
	}
	if token != "mcp-token" {
		return identity.Actor{}, identity.ErrAuthenticationFailed
	}
	return actor, nil
}

func (tokens *mcpTestTokens) setPanic(value bool) {
	tokens.mu.Lock()
	tokens.panic = value
	tokens.mu.Unlock()
}

func (tokens *mcpTestTokens) setError(err error) {
	tokens.mu.Lock()
	tokens.err = err
	tokens.mu.Unlock()
}

func (tokens *mcpTestTokens) setActor(actor identity.Actor) {
	tokens.mu.Lock()
	tokens.actor = actor
	tokens.mu.Unlock()
}

func (tokens *mcpTestTokens) setBlock(value bool) {
	tokens.mu.Lock()
	tokens.block = value
	tokens.mu.Unlock()
}

func (tokens *mcpTestTokens) waitForBlockedAuthentication(t *testing.T) {
	t.Helper()
	select {
	case <-tokens.start:
	case <-time.After(time.Second):
		t.Fatal("token authentication did not start")
	}
}

func mustNewMCP(t *testing.T, application *appkit.Application, options MCPOptions) http.Handler {
	t.Helper()
	handler, err := NewMCP(application, options)
	if err != nil {
		t.Fatalf("NewMCP() error = %v", err)
	}
	return handler
}

func newMCPRequest(method, body string) *http.Request {
	request := httptest.NewRequest(method, "/mcp", strings.NewReader(body))
	if body == "" {
		request.Body = http.NoBody
		request.ContentLength = 0
	}
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer mcp-token")
	request.Header.Set("MCP-Protocol-Version", MCPProtocolVersion)
	return request
}

func performMCP(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newMCPRequest(http.MethodPost, body))
	return response
}

func assertMCPSecurityHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	for name, wanted := range map[string]string{
		"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff",
	} {
		if got := response.Header().Get(name); got != wanted {
			t.Errorf("%s = %q, want %q", name, got, wanted)
		}
	}
}

type hostileHTTPBoundaryError struct{ entered chan struct{} }

func (*hostileHTTPBoundaryError) Error() string { panic("hostile Error invoked") }
func (*hostileHTTPBoundaryError) Is(error) bool { panic("hostile Is invoked") }
func (*hostileHTTPBoundaryError) As(any) bool   { panic("hostile As invoked") }
func (err *hostileHTTPBoundaryError) Unwrap() error {
	close(err.entered)
	select {}
}

func assertSchemaRequired(t *testing.T, raw json.RawMessage, names ...string) {
	t.Helper()
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if !reflect.DeepEqual(schema.Required, names) {
		t.Fatalf("required = %#v, want %#v; schema=%s", schema.Required, names, raw)
	}
}

func assertTextMirrorsStructured(t *testing.T, textContent string, structured any) {
	t.Helper()
	var textValue any
	if err := json.Unmarshal([]byte(textContent), &textValue); err != nil {
		t.Fatalf("text content is not JSON: %v", err)
	}
	structuredData, err := json.Marshal(structured)
	if err != nil {
		t.Fatal(err)
	}
	var structuredValue any
	if err := json.Unmarshal(structuredData, &structuredValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(textValue, structuredValue) {
		t.Fatalf("text content %#v does not mirror structured content %#v", textValue, structuredValue)
	}
}
