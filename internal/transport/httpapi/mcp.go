package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"modary/core/action"
	"modary/core/identity"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) mcp(writer http.ResponseWriter, request *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		writeJSON(writer, http.StatusUnauthorized, jsonRPCResponse{JSONRPC: "2.0", Error: &jsonRPCError{Code: -32001, Message: "agent authentication is required"}})
		return
	}
	actor, err := s.app.Identity.ResolveAgentToken(request.Context(), token)
	if err != nil {
		writeJSON(writer, http.StatusUnauthorized, jsonRPCResponse{JSONRPC: "2.0", Error: &jsonRPCError{Code: -32001, Message: "agent token is invalid or expired"}})
		return
	}
	var rpc jsonRPCRequest
	if err := decodeBody(request, &rpc); err != nil || rpc.JSONRPC != "2.0" {
		writeJSON(writer, http.StatusBadRequest, jsonRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Error: &jsonRPCError{Code: -32600, Message: "invalid JSON-RPC request"}})
		return
	}
	switch rpc.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(rpc.Params, &params)
		if params.ProtocolVersion == "" {
			params.ProtocolVersion = "2025-06-18"
		}
		writeJSON(writer, http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Result: map[string]any{
			"protocolVersion": params.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "modary-rulary", "version": "0.1.0"},
		}})
	case "notifications/initialized":
		writer.WriteHeader(http.StatusNoContent)
	case "tools/list":
		writeJSON(writer, http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Result: map[string]any{"tools": s.mcpTools(actor)}})
	case "tools/call":
		s.callMCPTool(writer, request, rpc, actor)
	default:
		writeJSON(writer, http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Error: &jsonRPCError{Code: -32601, Message: "method not found"}})
	}
}

func (s *Server) mcpTools(actor identity.Actor) []map[string]any {
	tools := make([]map[string]any, 0)
	for _, item := range s.app.Registry.List() {
		descriptor := item.Descriptor
		if !slices.Contains(descriptor.Channels, "mcp") || !slices.Contains(actor.AllowedActions, descriptor.ID) {
			continue
		}
		var inputSchema any
		_ = json.Unmarshal(descriptor.InputSchema, &inputSchema)
		operations := []string{"execute"}
		if descriptor.Preview != action.PreviewNone {
			operations = []string{"preview", "execute"}
		}
		tools = append(tools, map[string]any{
			"name":        descriptor.ID,
			"title":       descriptor.Title,
			"description": descriptor.Description,
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"operation":       map[string]any{"type": "string", "enum": operations},
					"input":           inputSchema,
					"plan_hash":       map[string]any{"type": "string"},
					"idempotency_key": map[string]any{"type": "string"},
				},
				"required":             []string{"operation", "input"},
				"additionalProperties": false,
			},
			"outputSchema": map[string]any{"type": "object"},
		})
	}
	return tools
}

func (s *Server) callMCPTool(writer http.ResponseWriter, request *http.Request, rpc jsonRPCRequest, actor identity.Actor) {
	var params struct {
		Name      string `json:"name"`
		Arguments struct {
			Operation      string          `json:"operation"`
			Input          json.RawMessage `json:"input"`
			PlanHash       string          `json:"plan_hash"`
			IdempotencyKey string          `json:"idempotency_key"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(rpc.Params, &params); err != nil {
		writeJSON(writer, http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Error: &jsonRPCError{Code: -32602, Message: "invalid tools/call parameters"}})
		return
	}
	actionRequest := action.Request{
		RequestID: fmt.Sprintf("mcp_%s", strings.Trim(string(rpc.ID), `"`)), Actor: actor, Channel: "mcp",
		ActionID: params.Name, WorkspaceID: actor.WorkspaceID, Input: params.Arguments.Input,
		PlanHash: params.Arguments.PlanHash, IdempotencyKey: params.Arguments.IdempotencyKey,
	}
	if !slices.Contains(actor.AllowedActions, params.Name) {
		permission := ""
		if registered, ok := s.app.Registry.Resolve(params.Name); ok {
			permission = registered.Descriptor.Permission
		}
		err := action.WithRequest(action.NewError(action.CodeAuthzDenied, "action is not in the agent grant allowlist"), actionRequest, permission)
		writeJSON(writer, http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Result: mcpExecutionError(err)})
		return
	}
	var structured any
	var err error
	switch params.Arguments.Operation {
	case "preview":
		structured, err = s.app.Runtime.Preview(request.Context(), actionRequest)
	case "execute":
		var result action.Result
		result, err = s.app.Runtime.Execute(request.Context(), actionRequest)
		if err == nil {
			_ = json.Unmarshal(result.Data, &structured)
		}
	default:
		err = action.NewError(action.CodeValidationFailed, "operation must be preview or execute")
	}
	if err != nil {
		writeJSON(writer, http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Result: mcpExecutionError(err)})
		return
	}
	text, _ := json.Marshal(structured)
	writeJSON(writer, http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Result: map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(text)}},
		"structuredContent": structured,
		"isError":           false,
	}})
}

func mcpExecutionError(err error) map[string]any {
	message := err.Error()
	var structured any
	if actionErr, ok := err.(*action.Error); ok {
		message = actionErr.Code + ": " + actionErr.Message
		structured = map[string]any{"error": actionErr}
	}
	result := map[string]any{
		"content": []map[string]any{{"type": "text", "text": message}},
		"isError": true,
	}
	if structured != nil {
		result["structuredContent"] = structured
	}
	return result
}
