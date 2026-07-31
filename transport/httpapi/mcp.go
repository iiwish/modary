package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/identity"
	frameworkschema "github.com/iiwish/modary/internal/jsonschema"
	"github.com/iiwish/modary/internal/jsonvalue"
	"github.com/iiwish/modary/internal/safeerr"
)

// MCP protocol and resource limits used when MCPOptions fields are zero.
const (
	MCPProtocolVersion                  = "2025-11-25"
	DefaultMCPMaxBodyBytes        int64 = 2 << 20
	MaximumMCPBodyBytes           int64 = 16 << 20
	DefaultMCPRequestTimeout            = 30 * time.Second
	DefaultMCPMaxConcurrentCalls        = 32
	MaximumMCPConcurrentCalls           = 4096
	maxMCPBearerBytes                   = 4096
	maxMCPWrapperSchemaNodes            = 128
	maxMCPWrapperJSONValueNodes         = 4_096
	maxMCPWrapperNumericWorkUnits       = 1 << 20
	mcpRefTargetsAnnotation             = "x-modary-mcp-ref-targets"
)

// MCPOptions configures the stateless JSON response profile of MCP Streamable
// HTTP. Browser origins are denied unless explicitly allowlisted; non-browser
// clients normally omit Origin.
type MCPOptions struct {
	AllowedOrigins     []string
	MaxBodyBytes       int64
	RequestTimeout     time.Duration
	MaxConcurrentCalls int
}

type mcpHandler struct {
	metadata       appkit.Metadata
	runtime        appkit.Runtime
	tokens         identity.TokenAuthenticator
	tools          []mcpTool
	toolByName     map[string]mcpTool
	allowedOrigins map[string]struct{}
	maxBodyBytes   int64
	requestTimeout time.Duration
	calls          chan struct{}
}

type mcpOperation string

const (
	mcpPreview mcpOperation = "preview"
	mcpExecute mcpOperation = "execute"
)

type mcpTool struct {
	Name         string            `json:"name"`
	Title        string            `json:"title,omitempty"`
	Description  string            `json:"description,omitempty"`
	InputSchema  json.RawMessage   `json:"inputSchema"`
	OutputSchema json.RawMessage   `json:"outputSchema"`
	Meta         map[string]string `json:"_meta,omitempty"`

	actionID            string
	operation           mcpOperation
	actionOutput        *action.Validator
	previewPolicy       action.PreviewPolicy
	requiresIdempotency bool
}

type mcpRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content           []mcpTextContent `json:"content"`
	StructuredContent any              `json:"structuredContent,omitempty"`
	IsError           bool             `json:"isError,omitempty"`
}

type mcpActionArguments struct {
	Input          json.RawMessage
	PlanHash       string
	IdempotencyKey string
}

// NewMCP constructs an explicitly mounted, bearer-authenticated MCP endpoint.
// It implements the non-streaming application/json response profile; GET and
// DELETE deliberately return 405 because this handler owns no SSE sessions.
func NewMCP(application *appkit.Application, options MCPOptions) (http.Handler, error) {
	if application == nil {
		return nil, fmt.Errorf("application is required")
	}
	if !application.Ready() {
		return nil, fmt.Errorf("MCP application: %w", appkit.ErrApplicationUnavailable)
	}
	if options.MaxBodyBytes < 0 {
		return nil, fmt.Errorf("MCP max body bytes cannot be negative")
	}
	if options.MaxBodyBytes == 0 {
		options.MaxBodyBytes = DefaultMCPMaxBodyBytes
	}
	if options.MaxBodyBytes > MaximumMCPBodyBytes {
		return nil, fmt.Errorf("MCP max body bytes cannot exceed %d", MaximumMCPBodyBytes)
	}
	if options.RequestTimeout < 0 {
		return nil, fmt.Errorf("MCP request timeout cannot be negative")
	}
	if options.RequestTimeout == 0 {
		options.RequestTimeout = DefaultMCPRequestTimeout
	}
	if options.MaxConcurrentCalls < 0 {
		return nil, fmt.Errorf("MCP max concurrent calls cannot be negative")
	}
	if options.MaxConcurrentCalls == 0 {
		options.MaxConcurrentCalls = DefaultMCPMaxConcurrentCalls
	}
	if options.MaxConcurrentCalls > MaximumMCPConcurrentCalls {
		return nil, fmt.Errorf("MCP max concurrent calls cannot exceed %d", MaximumMCPConcurrentCalls)
	}
	allowedOrigins, err := prepareMCPOrigins(options.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	tokens, err := application.Tokens()
	if err != nil {
		return nil, fmt.Errorf("MCP token authentication: %w", err)
	}
	tools, err := buildMCPTools(application.Catalog())
	if err != nil {
		return nil, err
	}
	byName := make(map[string]mcpTool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	if !application.Ready() {
		return nil, fmt.Errorf("application stopped during MCP construction: %w", appkit.ErrApplicationUnavailable)
	}
	return &mcpHandler{
		metadata: application.Metadata(), runtime: application.Runtime(), tokens: tokens,
		tools: tools, toolByName: byName, allowedOrigins: allowedOrigins,
		maxBodyBytes: options.MaxBodyBytes, requestTimeout: options.RequestTimeout,
		calls: make(chan struct{}, options.MaxConcurrentCalls),
	}, nil
}

// ServeHTTP enforces the MCP HTTP, origin, authentication, timeout, and panic
// boundaries before dispatching one JSON-RPC message.
func (handler *mcpHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	guarded := &trackedResponseWriter{ResponseWriter: writer}
	writer = guarded
	returned := false
	defer func() {
		if returned {
			return
		}
		_ = recover()
		containResponsePanic(guarded, func() {
			handler.writeRPC(writer, http.StatusInternalServerError, mcpRPCResponse{
				JSONRPC: "2.0", Error: &mcpRPCError{Code: -32603, Message: "internal server error"},
			})
		})
	}()
	handler.serveHTTP(writer, request)
	returned = true
}

func (handler *mcpHandler) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Cache-Control", "no-store")
	if !handler.originAllowed(request.Header.Values("Origin")) {
		handler.writeRPC(writer, http.StatusForbidden, mcpRPCResponse{
			JSONRPC: "2.0", Error: &mcpRPCError{Code: -32003, Message: "origin is not allowed"},
		})
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		handler.writeRPC(writer, http.StatusMethodNotAllowed, mcpRPCResponse{
			JSONRPC: "2.0", Error: &mcpRPCError{Code: -32600, Message: "MCP endpoint accepts POST requests only"},
		})
		return
	}
	if !mcpAccepts(request.Header.Values("Accept"), "application/json") || !mcpAccepts(request.Header.Values("Accept"), "text/event-stream") {
		handler.writeRPC(writer, http.StatusNotAcceptable, mcpRPCResponse{
			JSONRPC: "2.0", Error: &mcpRPCError{Code: -32600, Message: "Accept must include application/json and text/event-stream"},
		})
		return
	}
	if err := validateMCPContentType(request.Header.Values("Content-Type")); err != nil {
		handler.writeRPC(writer, http.StatusUnsupportedMediaType, mcpRPCResponse{
			JSONRPC: "2.0", Error: &mcpRPCError{Code: -32600, Message: err.Error()},
		})
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.requestTimeout)
	defer cancel()
	request = request.WithContext(ctx)
	releaseBody := bindRequestBody(ctx, request)
	defer releaseBody()
	token, ok := mcpBearerToken(request.Header.Values("Authorization"))
	if !ok {
		handler.writeUnauthorized(writer, "bearer authentication is required")
		return
	}
	actor, err := handler.tokens.AuthenticateToken(ctx, token)
	if err != nil {
		if status, message, ok := classifyContextFailure(ctx, err); ok {
			handler.writeRPC(writer, status, mcpRPCResponse{JSONRPC: "2.0", Error: &mcpRPCError{Code: -32000, Message: message}})
		} else if !isTypedNil(err) && safeerr.Is(err, identity.ErrAuthenticationFailed) {
			handler.writeUnauthorized(writer, "bearer credential is invalid or expired")
		} else {
			handler.writeRPC(writer, http.StatusInternalServerError, mcpRPCResponse{JSONRPC: "2.0", Error: &mcpRPCError{Code: -32603, Message: "internal server error"}})
		}
		return
	}
	if err := identity.ValidateActor(actor); err != nil {
		handler.writeRPC(writer, http.StatusInternalServerError, mcpRPCResponse{
			JSONRPC: "2.0", Error: &mcpRPCError{Code: -32603, Message: "internal server error"},
		})
		return
	}
	rpc, status, rpcErr := decodeMCPRequest(writer, request, handler.maxBodyBytes)
	if rpcErr != nil {
		handler.writeRPC(writer, status, mcpRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Error: rpcErr})
		return
	}
	if rpc.Method != "initialize" {
		versions := request.Header.Values("MCP-Protocol-Version")
		if len(versions) != 1 || versions[0] != MCPProtocolVersion {
			handler.writeRPC(writer, http.StatusBadRequest, mcpRPCResponse{
				JSONRPC: "2.0", ID: rpc.ID,
				Error: &mcpRPCError{Code: -32600, Message: "unsupported or missing MCP-Protocol-Version", Data: map[string]any{"supported": []string{MCPProtocolVersion}}},
			})
			return
		}
	}
	handler.dispatch(ctx, writer, rpc, actor)
}

func (handler *mcpHandler) dispatch(ctx context.Context, writer http.ResponseWriter, rpc mcpRPCRequest, actor identity.Actor) {
	notification := len(rpc.ID) == 0
	switch rpc.Method {
	case "initialize":
		if notification {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		handler.initialize(writer, rpc)
	case "notifications/initialized":
		if !notification {
			handler.invalidRequest(writer, rpc.ID, rpc.Method+" must be a notification")
			return
		}
		var params struct {
			Meta json.RawMessage `json:"_meta,omitempty"`
		}
		if err := decodeMCPParams(rpc.Params, &params); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	case "notifications/cancelled":
		if !notification {
			handler.invalidRequest(writer, rpc.ID, rpc.Method+" must be a notification")
			return
		}
		var params struct {
			RequestID json.RawMessage `json:"requestId"`
			Reason    string          `json:"reason,omitempty"`
			Meta      json.RawMessage `json:"_meta,omitempty"`
		}
		if err := decodeMCPParams(rpc.Params, &params); err != nil || !validMCPConcreteID(params.RequestID) {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	case "ping":
		if notification {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		var params struct {
			Meta json.RawMessage `json:"_meta,omitempty"`
		}
		if err := decodeMCPParams(rpc.Params, &params); err != nil {
			handler.invalidParams(writer, rpc.ID, "invalid ping parameters")
			return
		}
		handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Result: map[string]any{}})
	case "tools/list":
		if notification {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		handler.listTools(writer, rpc)
	case "tools/call":
		if notification {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		handler.callTool(ctx, writer, rpc, actor)
	default:
		if notification {
			writer.WriteHeader(http.StatusAccepted)
			return
		}
		handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{
			JSONRPC: "2.0", ID: rpc.ID, Error: &mcpRPCError{Code: -32601, Message: "method not found"},
		})
	}
}

func (handler *mcpHandler) initialize(writer http.ResponseWriter, rpc mcpRPCRequest) {
	var params struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Capabilities    json.RawMessage `json:"capabilities"`
		ClientInfo      json.RawMessage `json:"clientInfo"`
		Meta            json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeMCPParams(rpc.Params, &params); err != nil || params.ProtocolVersion == "" || !isJSONObject(params.Capabilities) || !validMCPImplementation(params.ClientInfo) {
		handler.invalidParams(writer, rpc.ID, "invalid initialize parameters")
		return
	}
	handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Result: map[string]any{
		"protocolVersion": MCPProtocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo": map[string]any{
			"name": handler.metadata.ID, "title": handler.metadata.Name, "version": handler.metadata.Version,
		},
	}})
}

func (handler *mcpHandler) listTools(writer http.ResponseWriter, rpc mcpRPCRequest) {
	var params struct {
		Cursor string          `json:"cursor,omitempty"`
		Meta   json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeMCPParams(rpc.Params, &params); err != nil || params.Cursor != "" {
		handler.invalidParams(writer, rpc.ID, "invalid tools/list parameters")
		return
	}
	handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{
		JSONRPC: "2.0", ID: rpc.ID, Result: map[string]any{"tools": handler.tools},
	})
}

func (handler *mcpHandler) callTool(ctx context.Context, writer http.ResponseWriter, rpc mcpRPCRequest, actor identity.Actor) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments,omitempty"`
		Meta      json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeMCPParams(rpc.Params, &params); err != nil || params.Name == "" {
		handler.invalidParams(writer, rpc.ID, "invalid tools/call parameters")
		return
	}
	tool, ok := handler.toolByName[params.Name]
	if !ok {
		handler.invalidParams(writer, rpc.ID, "unknown tool")
		return
	}
	if len(params.Arguments) == 0 {
		params.Arguments = json.RawMessage(`{}`)
	}
	arguments, err := decodeMCPActionArguments(tool, params.Arguments)
	if err != nil {
		handler.writeToolError(ctx, writer, rpc.ID, action.NewError(action.CodeValidationFailed, "tool arguments are invalid"))
		return
	}
	select {
	case handler.calls <- struct{}{}:
		defer func() { <-handler.calls }()
	case <-ctx.Done():
		handler.writeToolError(ctx, writer, rpc.ID, action.NewError(action.CodeUnavailable, "tool execution capacity is unavailable"))
		return
	}
	request := action.Request{
		Actor: actor, Channel: action.ChannelMCP, ActionID: tool.actionID, Scope: actor.Scope,
		Input: arguments.Input, PlanHash: arguments.PlanHash, IdempotencyKey: arguments.IdempotencyKey,
	}
	if tool.operation == mcpPreview {
		preview, err := handler.runtime.Preview(ctx, request)
		if err != nil {
			handler.writeToolError(ctx, writer, rpc.ID, err)
			return
		}
		if err := tool.actionOutput.Validate(preview.Summary); err != nil {
			handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Error: &mcpRPCError{Code: -32603, Message: "Action returned an invalid preview value"}})
			return
		}
		structured := map[string]any{"preview": preview}
		handler.writeToolSuccess(writer, rpc.ID, structured)
		return
	}
	result, err := handler.runtime.Execute(ctx, request)
	if err != nil {
		handler.writeToolError(ctx, writer, rpc.ID, err)
		return
	}
	data, err := decodeMCPValue(result.Data)
	if err != nil {
		handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Error: &mcpRPCError{Code: -32603, Message: "Action returned invalid structured content"}})
		return
	}
	if err := tool.actionOutput.Validate(result.Data); err != nil {
		handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{JSONRPC: "2.0", ID: rpc.ID, Error: &mcpRPCError{Code: -32603, Message: "Action returned invalid structured content"}})
		return
	}
	structured := map[string]any{"result": data}
	if result.Summary != "" {
		structured["summary"] = result.Summary
	}
	if len(result.References) > 0 {
		structured["references"] = result.References
	}
	handler.writeToolSuccess(writer, rpc.ID, structured)
}

func decodeMCPActionArguments(tool mcpTool, raw json.RawMessage) (mcpActionArguments, error) {
	var arguments mcpActionArguments
	switch {
	case tool.operation == mcpPreview:
		var value struct {
			Input json.RawMessage `json:"input"`
		}
		if err := decodeMCPParams(raw, &value); err != nil {
			return arguments, err
		}
		arguments.Input = value.Input
	case tool.previewPolicy == action.PreviewNone:
		var value struct {
			Input          json.RawMessage `json:"input"`
			IdempotencyKey *string         `json:"idempotency_key,omitempty"`
		}
		if err := decodeMCPParams(raw, &value); err != nil {
			return arguments, err
		}
		arguments.Input = value.Input
		if value.IdempotencyKey != nil {
			arguments.IdempotencyKey = *value.IdempotencyKey
		}
		if err := validateMCPIdempotencyKey(value.IdempotencyKey, tool.requiresIdempotency); err != nil {
			return mcpActionArguments{}, err
		}
	default:
		var value struct {
			Input          json.RawMessage `json:"input"`
			PlanHash       *string         `json:"plan_hash,omitempty"`
			IdempotencyKey *string         `json:"idempotency_key,omitempty"`
		}
		if err := decodeMCPParams(raw, &value); err != nil {
			return arguments, err
		}
		arguments.Input = value.Input
		if value.PlanHash != nil {
			arguments.PlanHash = *value.PlanHash
			if action.ValidatePlanHash(arguments.PlanHash) != nil {
				return mcpActionArguments{}, fmt.Errorf("plan_hash is invalid")
			}
		} else if tool.previewPolicy == action.PreviewRequired {
			return mcpActionArguments{}, fmt.Errorf("plan_hash is required")
		}
		if value.IdempotencyKey != nil {
			arguments.IdempotencyKey = *value.IdempotencyKey
		}
		if err := validateMCPIdempotencyKey(value.IdempotencyKey, tool.requiresIdempotency); err != nil {
			return mcpActionArguments{}, err
		}
	}
	if len(arguments.Input) == 0 {
		return mcpActionArguments{}, fmt.Errorf("input is required")
	}
	return arguments, nil
}

func validateMCPIdempotencyKey(value *string, required bool) error {
	if value == nil {
		if required {
			return fmt.Errorf("idempotency_key is required")
		}
		return nil
	}
	if err := action.ValidateIdempotencyKey(*value); err != nil {
		return err
	}
	return nil
}

func (handler *mcpHandler) writeToolSuccess(writer http.ResponseWriter, id json.RawMessage, structured any) {
	data, err := json.Marshal(structured)
	if err != nil {
		handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{JSONRPC: "2.0", ID: id, Error: &mcpRPCError{Code: -32603, Message: "encode tool result"}})
		return
	}
	handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{JSONRPC: "2.0", ID: id, Result: mcpToolResult{
		Content: []mcpTextContent{{Type: "text", Text: string(data)}}, StructuredContent: structured,
	}})
}

func (handler *mcpHandler) writeToolError(ctx context.Context, writer http.ResponseWriter, id json.RawMessage, err error) {
	detail := publicError{Code: action.CodeInternal, Kind: action.ErrorKindInternal, Message: "tool execution failed"}
	switch {
	case ctx != nil && ctx.Err() == context.DeadlineExceeded:
		detail = publicError{Code: action.CodeUnavailable, Kind: action.ErrorKindUnavailable, Message: "tool execution timed out"}
	case ctx != nil && ctx.Err() == context.Canceled:
		detail = publicError{Code: action.CodeUnavailable, Kind: action.ErrorKindUnavailable, Message: "tool execution was canceled"}
	default:
		if normalized, ok := normalizedPublicActionError(err); ok {
			detail = normalized
		}
	}
	data, marshalErr := json.Marshal(detail)
	if marshalErr != nil {
		data = []byte(`{"error_code":"INTERNAL_ERROR","error_kind":"internal","human_readable_reason":"tool execution failed"}`)
	}
	handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{JSONRPC: "2.0", ID: id, Result: mcpToolResult{
		Content: []mcpTextContent{{Type: "text", Text: string(data)}}, IsError: true,
	}})
}

func (handler *mcpHandler) invalidRequest(writer http.ResponseWriter, id json.RawMessage, message string) {
	handler.writeRPC(writer, http.StatusBadRequest, mcpRPCResponse{JSONRPC: "2.0", ID: id, Error: &mcpRPCError{Code: -32600, Message: message}})
}

func (handler *mcpHandler) invalidParams(writer http.ResponseWriter, id json.RawMessage, message string) {
	handler.writeRPC(writer, http.StatusOK, mcpRPCResponse{JSONRPC: "2.0", ID: id, Error: &mcpRPCError{Code: -32602, Message: message}})
}

func (handler *mcpHandler) writeUnauthorized(writer http.ResponseWriter, message string) {
	writer.Header().Set("WWW-Authenticate", `Bearer realm="mcp"`)
	handler.writeRPC(writer, http.StatusUnauthorized, mcpRPCResponse{JSONRPC: "2.0", Error: &mcpRPCError{Code: -32001, Message: message}})
}

func (handler *mcpHandler) writeRPC(writer http.ResponseWriter, status int, response mcpRPCResponse) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(response)
}

func buildMCPTools(catalog []action.CatalogEntry) ([]mcpTool, error) {
	tools := make([]mcpTool, 0, len(catalog)*2)
	seen := make(map[string]string)
	for _, entry := range catalog {
		descriptor := entry.Descriptor
		if !containsMCPChannel(descriptor.Channels) {
			continue
		}
		operations := []mcpOperation{mcpExecute}
		if descriptor.Preview != action.PreviewNone {
			operations = append([]mcpOperation{mcpPreview}, operations...)
		}
		for _, operation := range operations {
			tool, err := buildMCPTool(entry, operation)
			if err != nil {
				return nil, err
			}
			if owner, exists := seen[tool.Name]; exists {
				return nil, fmt.Errorf("MCP tool name collision between %s and %s", owner, descriptor.ID)
			}
			seen[tool.Name] = descriptor.ID
			tools = append(tools, tool)
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

func buildMCPTool(entry action.CatalogEntry, operation mcpOperation) (mcpTool, error) {
	descriptor := entry.Descriptor
	inputSchema, err := mcpToolInputSchema(descriptor, operation)
	if err != nil {
		return mcpTool{}, fmt.Errorf("build MCP input for Action %s: %w", descriptor.ID, err)
	}
	if err := compileMCPToolSchema(inputSchema); err != nil {
		return mcpTool{}, fmt.Errorf("compile MCP input for Action %s: %w", descriptor.ID, err)
	}
	outputSchema, err := mcpToolOutputSchema(descriptor, operation)
	if err != nil {
		return mcpTool{}, fmt.Errorf("build MCP output for Action %s: %w", descriptor.ID, err)
	}
	if err := compileMCPToolSchema(outputSchema); err != nil {
		return mcpTool{}, fmt.Errorf("compile MCP output for Action %s: %w", descriptor.ID, err)
	}
	actionOutputSchema := descriptor.OutputSchema
	if operation == mcpPreview {
		actionOutputSchema = descriptor.PreviewSchema
	}
	actionOutput, err := action.CompileValidator(actionOutputSchema)
	if err != nil {
		return mcpTool{}, fmt.Errorf("compile Action output for MCP Action %s: %w", descriptor.ID, err)
	}
	title := descriptor.Title
	description := descriptor.Description
	if operation == mcpPreview {
		title += " preview"
		if description != "" {
			description = "Preview: " + description
		}
	}
	return mcpTool{
		Name: mcpToolName(descriptor.ID, operation), Title: title, Description: description,
		InputSchema: inputSchema, OutputSchema: outputSchema,
		Meta: map[string]string{
			"modary/actionId": descriptor.ID, "modary/actionVersion": descriptor.Version,
			"modary/contractHash": entry.ContractHash, "modary/operation": string(operation),
		},
		actionID: descriptor.ID, operation: operation, actionOutput: actionOutput,
		previewPolicy: descriptor.Preview, requiresIdempotency: descriptor.RequiresIdempotency,
	}, nil
}

func compileMCPToolSchema(schema json.RawMessage) error {
	decodeLimits := protocolJSONLimits(MaximumMCPBodyBytes)
	// Rebase keeps literal data intact and may add one disjoint copy of hidden
	// local-reference targets. This is a compile-only protocol allowance.
	decodeLimits.MaxNodes += maxMCPWrapperJSONValueNodes
	document, err := jsonvalue.Decode(schema, decodeLimits)
	if err != nil {
		return fmt.Errorf("schema is not valid protocol JSON: %w", err)
	}
	compileLimits := frameworkschema.DefaultCompileLimits()
	// Every embedded Action schema already satisfies the public profile. The
	// protocol profile adds room only for Modary's fixed wrapper structure;
	// collection, literal, regex, and same-instance limits stay equal. Numeric
	// work gets a separate fixed allowance for framework-owned wrapper tokens.
	compileLimits.MaxSchemaNodes += maxMCPWrapperSchemaNodes
	compileLimits.MaxNumericCompileWorkUnits += maxMCPWrapperNumericWorkUnits
	if _, err := frameworkschema.CompileWithLimits(document, compileLimits); err != nil {
		return fmt.Errorf("schema is not valid JSON Schema: %w", err)
	}
	return nil
}

func mcpToolInputSchema(descriptor action.Descriptor, operation mcpOperation) (json.RawMessage, error) {
	input, err := rebaseMCPActionSchema(descriptor.InputSchema, "#/properties/input")
	if err != nil {
		return nil, fmt.Errorf("embed Action input schema: %w", err)
	}
	properties := map[string]any{"input": input}
	required := []string{"input"}
	if operation == mcpExecute {
		if descriptor.Preview != action.PreviewNone {
			properties["plan_hash"] = map[string]any{"type": "string", "pattern": `^sha256:[0-9a-f]{64}$`}
			if descriptor.Preview == action.PreviewRequired {
				required = append(required, "plan_hash")
			}
		}
		properties["idempotency_key"] = map[string]any{
			"type": "string", "minLength": 1, "maxLength": action.MaxIdempotencyKeyBytes,
			"pattern": action.IdempotencyKeyPattern,
		}
		if descriptor.RequiresIdempotency {
			required = append(required, "idempotency_key")
		}
	}
	return json.Marshal(map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#", "type": "object",
		"properties": properties, "required": required, "additionalProperties": false,
	})
}

func mcpToolOutputSchema(descriptor action.Descriptor, operation mcpOperation) (json.RawMessage, error) {
	if operation == mcpPreview {
		summary, err := rebaseMCPActionSchema(descriptor.PreviewSchema, "#/properties/preview/properties/summary")
		if err != nil {
			return nil, fmt.Errorf("embed Action preview schema: %w", err)
		}
		return json.Marshal(map[string]any{
			"$schema": "http://json-schema.org/draft-07/schema#", "type": "object",
			"properties": map[string]any{"preview": map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"plan_hash": map[string]any{"type": "string", "pattern": `^sha256:[0-9a-f]{64}$`}, "summary": summary,
					"impact": map[string]any{
						"type": "object", "additionalProperties": false,
						"properties": map[string]any{
							"rows": map[string]any{"type": "integer", "minimum": 0},
							"resources": map[string]any{
								"type": "array", "maxItems": audit.MaxResources, "uniqueItems": true,
								"items": map[string]any{"type": "string", "minLength": 1, "maxLength": audit.MaxResourceRunes},
							},
						},
					},
					"expires_at": map[string]any{"type": "string", "format": "date-time"},
				},
				"required": []string{"plan_hash", "summary", "impact", "expires_at"},
			}},
			"required": []string{"preview"}, "additionalProperties": false,
		})
	}
	result, err := rebaseMCPActionSchema(descriptor.OutputSchema, "#/properties/result")
	if err != nil {
		return nil, fmt.Errorf("embed Action output schema: %w", err)
	}
	return json.Marshal(map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#", "type": "object",
		"properties": map[string]any{
			"result": result, "summary": map[string]any{"type": "string", "maxLength": audit.MaxSummaryRunes},
			"references": map[string]any{
				"type": "array", "maxItems": audit.MaxReferences, "uniqueItems": true,
				"items": map[string]any{
					"type": "object", "properties": map[string]any{
						"kind": map[string]any{"type": "string", "minLength": 1, "maxLength": audit.MaxKindRunes},
						"id":   map[string]any{"type": "string", "minLength": 1, "maxLength": audit.MaxIDRunes},
					},
					"required": []string{"kind", "id"}, "additionalProperties": false,
				}},
		},
		"required": []string{"result"}, "additionalProperties": false,
	})
}

func rebaseMCPActionSchema(raw json.RawMessage, pointer string) (any, error) {
	document, err := jsonvalue.Decode(raw, jsonvalue.Limits{
		MaxBytes:       action.MaxJSONDocumentBytes,
		MaxDepth:       action.MaxJSONNestingDepth,
		MaxNodes:       action.MaxJSONValueNodes,
		MaxNumberBytes: action.MaxJSONNumberBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("decode Action schema: %w", err)
	}
	graph, err := frameworkschema.Prepare(document)
	if err != nil {
		return nil, err
	}
	if _, err := graph.Compile(); err != nil {
		return nil, fmt.Errorf("compile Action schema: %w", err)
	}
	return graph.Rebase(pointer, mcpRefTargetsAnnotation)
}

func mcpToolName(actionID string, operation mcpOperation) string {
	slug := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			return r
		}
		return '-'
	}, actionID)
	if len(slug) > 48 {
		slug = slug[:48]
	}
	digest := sha256.Sum256([]byte(actionID))
	return "modary." + slug + "." + string(operation) + "." + hex.EncodeToString(digest[:12])
}

func prepareMCPOrigins(values []string) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		canonical, err := canonicalMCPOrigin(value)
		if err != nil {
			return nil, fmt.Errorf("invalid MCP allowed origin %q: %w", value, err)
		}
		if _, exists := result[canonical]; exists {
			return nil, fmt.Errorf("MCP allowed origin %q is duplicated", value)
		}
		result[canonical] = struct{}{}
	}
	return result, nil
}

func canonicalMCPOrigin(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
		return "", fmt.Errorf("origin must be a non-empty, trimmed UTF-8 value")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawPath != "" || parsed.Opaque != "" || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("origin must contain only an http(s) scheme and authority")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" || strings.ContainsFunc(hostname, unicode.IsSpace) {
		return "", fmt.Errorf("origin host is invalid")
	}
	port := parsed.Port()
	if port != "" {
		number, parseErr := strconv.ParseUint(port, 10, 16)
		if parseErr != nil || number == 0 {
			return "", fmt.Errorf("origin port is invalid")
		}
		if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
			port = ""
		}
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	return strings.ToLower(parsed.Scheme) + "://" + host, nil
}

func (handler *mcpHandler) originAllowed(values []string) bool {
	if len(values) == 0 {
		return true
	}
	if len(values) != 1 {
		return false
	}
	canonical, err := canonicalMCPOrigin(values[0])
	if err != nil {
		return false
	}
	_, ok := handler.allowedOrigins[canonical]
	return ok
}

func mcpBearerToken(values []string) (string, bool) {
	if len(values) != 1 || len(values[0]) > maxMCPBearerBytes+16 {
		return "", false
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) == 0 || len(parts[1]) > maxMCPBearerBytes {
		return "", false
	}
	if strings.ContainsFunc(parts[1], unicode.IsControl) || !utf8.ValidString(parts[1]) {
		return "", false
	}
	return parts[1], true
}

func mcpAccepts(values []string, wanted string) bool {
	items, ok := splitHTTPList(values)
	if !ok {
		return false
	}
	wantedType, _, ok := strings.Cut(strings.ToLower(wanted), "/")
	if !ok {
		return false
	}
	bestTypeSpecificity := -1
	bestParameterSpecificity := -1
	bestQuality := 0.0
	for _, item := range items {
		mediaType, parameters, err := mime.ParseMediaType(item)
		if err != nil {
			return false
		}
		quality := 1.0
		if raw, present := parameters["q"]; present {
			quality, err = parseHTTPQuality(raw)
			if err != nil {
				return false
			}
			delete(parameters, "q")
		}
		if !matchesMCPAcceptParameters(parameters) {
			continue
		}
		typeSpecificity := -1
		switch strings.ToLower(mediaType) {
		case strings.ToLower(wanted):
			typeSpecificity = 2
		case wantedType + "/*":
			typeSpecificity = 1
		case "*/*":
			typeSpecificity = 0
		}
		if typeSpecificity < 0 {
			continue
		}
		parameterSpecificity := len(parameters)
		if typeSpecificity > bestTypeSpecificity ||
			(typeSpecificity == bestTypeSpecificity && parameterSpecificity > bestParameterSpecificity) {
			bestTypeSpecificity = typeSpecificity
			bestParameterSpecificity = parameterSpecificity
			bestQuality = quality
		} else if typeSpecificity == bestTypeSpecificity && parameterSpecificity == bestParameterSpecificity && quality > bestQuality {
			bestQuality = quality
		}
	}
	return bestTypeSpecificity >= 0 && bestQuality > 0
}

func matchesMCPAcceptParameters(parameters map[string]string) bool {
	for name, value := range parameters {
		if !strings.EqualFold(name, "charset") || !strings.EqualFold(value, "utf-8") {
			return false
		}
	}
	return true
}

func validateMCPContentType(values []string) error {
	if len(values) != 1 {
		return fmt.Errorf("Content-Type must be application/json")
	}
	mediaType, params, err := mime.ParseMediaType(values[0])
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("Content-Type must be application/json")
	}
	for name, value := range params {
		if !strings.EqualFold(name, "charset") || !strings.EqualFold(value, "utf-8") {
			return fmt.Errorf("Content-Type parameters are not supported")
		}
	}
	return nil
}

func decodeMCPRequest(writer http.ResponseWriter, request *http.Request, limit int64) (mcpRPCRequest, int, *mcpRPCError) {
	var rpc mcpRPCRequest
	if request.ContentLength > limit {
		return rpc, http.StatusRequestEntityTooLarge, &mcpRPCError{Code: -32600, Message: "request body is too large"}
	}
	body := http.MaxBytesReader(writer, request.Body, limit)
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		if _, ok := safeerr.Find[*http.MaxBytesError](err); ok {
			return rpc, http.StatusRequestEntityTooLarge, &mcpRPCError{Code: -32600, Message: "request body is too large"}
		}
		if contextErr := request.Context().Err(); contextErr == context.DeadlineExceeded {
			return rpc, http.StatusGatewayTimeout, &mcpRPCError{Code: -32000, Message: "request timed out"}
		} else if contextErr == context.Canceled {
			return rpc, http.StatusServiceUnavailable, &mcpRPCError{Code: -32000, Message: "request was canceled"}
		}
		return rpc, http.StatusBadRequest, &mcpRPCError{Code: -32700, Message: "read JSON-RPC request"}
	}
	data = bytes.TrimSpace(data)
	if err := validateMCPJSON(data, limit); err != nil {
		return rpc, http.StatusBadRequest, &mcpRPCError{Code: -32700, Message: "invalid JSON-RPC request"}
	}
	if !isJSONObject(data) {
		return rpc, http.StatusBadRequest, &mcpRPCError{Code: -32600, Message: "invalid JSON-RPC request"}
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return rpc, http.StatusBadRequest, &mcpRPCError{Code: -32700, Message: "invalid JSON-RPC request"}
	}
	var responseID json.RawMessage
	if candidate, exists := members["id"]; exists && validMCPID(candidate) {
		responseID = append(json.RawMessage(nil), candidate...)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&rpc); err != nil {
		return mcpRPCRequest{ID: responseID}, http.StatusBadRequest, &mcpRPCError{Code: -32600, Message: "invalid JSON-RPC request"}
	}
	if err := requireMCPEOF(decoder); err != nil || rpc.JSONRPC != "2.0" || rpc.Method == "" || !validMCPID(rpc.ID) {
		if !validMCPID(rpc.ID) {
			rpc.ID = nil
		}
		return rpc, http.StatusBadRequest, &mcpRPCError{Code: -32600, Message: "invalid JSON-RPC request"}
	}
	return rpc, http.StatusOK, nil
}

func decodeMCPParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if !isJSONObject(raw) || validateMCPJSON(raw, MaximumMCPBodyBytes) != nil {
		return fmt.Errorf("parameters must be an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return requireMCPEOF(decoder)
}

func decodeMCPValue(raw json.RawMessage) (any, error) {
	if err := action.ValidateJSONDocument(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := requireMCPEOF(decoder); err != nil {
		return nil, err
	}
	return value, nil
}

func requireMCPEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}'
}

func validMCPImplementation(raw json.RawMessage) bool {
	if !isJSONObject(raw) || validateMCPJSON(raw, MaximumMCPBodyBytes) != nil {
		return false
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || requireMCPEOF(decoder) != nil {
		return false
	}
	name, nameOK := value["name"].(string)
	version, versionOK := value["version"].(string)
	return nameOK && versionOK && name != "" && version != ""
}

func validMCPID(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || requireMCPEOF(decoder) != nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return true
	case json.Number:
		return validMCPInteger(string(typed))
	default:
		return false
	}
}

func validMCPConcreteID(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && validMCPID(trimmed)
}

func validMCPInteger(value string) bool {
	if value == "" {
		return false
	}
	if value[0] == '-' {
		value = value[1:]
		if value == "" {
			return false
		}
	}
	if value == "0" {
		return true
	}
	if value[0] < '1' || value[0] > '9' {
		return false
	}
	for index := 1; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func validateMCPJSON(data []byte, maxBytes int64) error {
	return jsonvalue.Validate(data, protocolJSONLimits(maxBytes))
}

func containsMCPChannel(channels []action.Channel) bool {
	for _, channel := range channels {
		if channel == action.ChannelMCP {
			return true
		}
	}
	return false
}
