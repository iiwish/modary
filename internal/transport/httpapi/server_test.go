package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"modary/core/config"
	"modary/core/module"
	"modary/internal/app"
	"modary/internal/transport/httpapi"
	audit_module "modary/modules/audit"
	authz_basic "modary/modules/authz-basic"
	console_react "modary/modules/console-react"
	database_sqlite "modary/modules/database-sqlite"
	identity_local "modary/modules/identity-local"
	rulary_core "modary/modules/rulary-core"
)

func TestHTTPAuthenticationCSRFAndActionGateway(t *testing.T) {
	application, cfg := testApplication(t)
	defer application.Close()
	server := httptest.NewServer(httpapi.New(application))
	defer server.Close()

	client := server.Client()
	loginResponse := postJSON(t, client, server.URL+"/api/auth/login", map[string]any{"username": "admin", "password": cfg.DemoPassword}, "", nil)
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", loginResponse.StatusCode)
	}
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	decode(t, loginResponse, &session)
	cookies := loginResponse.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie = %#v", cookies)
	}
	secureRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"`+cfg.DemoPassword+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	secureRequest.Header.Set("Content-Type", "application/json")
	secureRequest.Header.Set("X-Forwarded-Proto", "https")
	secureResponse, err := client.Do(secureRequest)
	if err != nil {
		t.Fatal(err)
	}
	secureResponse.Body.Close()
	if len(secureResponse.Cookies()) != 1 || !secureResponse.Cookies()[0].Secure {
		t.Fatalf("HTTPS session cookie = %#v", secureResponse.Cookies())
	}

	actionResponse := postJSON(t, client, server.URL+"/api/actions/rulary.ruleset.list/execute", map[string]any{"input": map[string]any{"limit": 10}}, session.CSRF, cookies)
	if actionResponse.StatusCode != http.StatusOK {
		t.Fatalf("action status = %d", actionResponse.StatusCode)
	}
	var list map[string]any
	decode(t, actionResponse, &list)
	if list["request_id"] == "" {
		t.Fatalf("action response = %#v", list)
	}

	denied := postJSON(t, client, server.URL+"/api/actions/rulary.ruleset.list/execute", map[string]any{"input": map[string]any{}}, "", cookies)
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("missing CSRF status = %d", denied.StatusCode)
	}
}

func TestMCPListsAllowlistedToolsAndUsesRuntime(t *testing.T) {
	application, cfg := testApplication(t)
	defer application.Close()
	server := httptest.NewServer(httpapi.New(application))
	defer server.Close()

	client := server.Client()
	initialize := postJSON(t, client, server.URL+"/mcp", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"},
	}, "", nil, "Bearer "+cfg.AgentToken)
	if initialize.StatusCode != http.StatusOK {
		t.Fatalf("initialize status = %d", initialize.StatusCode)
	}
	var initialized map[string]any
	decode(t, initialize, &initialized)
	if initialized["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize = %#v", initialized)
	}

	list := postJSON(t, client, server.URL+"/mcp", map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{},
	}, "", nil, "Bearer "+cfg.AgentToken)
	var listed map[string]any
	decode(t, list, &listed)
	tools := listed["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("tools = %#v", tools)
	}

	call := postJSON(t, client, server.URL+"/mcp", map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": "rulary.ruleset.validate", "arguments": map[string]any{"operation": "execute", "input": map[string]any{"ruleset_id": "missing"}}},
	}, "", nil, "Bearer "+cfg.AgentToken)
	var called map[string]any
	decode(t, call, &called)
	if called["result"].(map[string]any)["isError"] != true {
		t.Fatalf("tools/call = %#v", called)
	}
}

func TestHTTPAndMCPExecutePublishedRuleThroughAdapters(t *testing.T) {
	application, cfg := testApplication(t)
	defer application.Close()
	server := httptest.NewServer(httpapi.New(application))
	defer server.Close()
	client := server.Client()

	loginResponse := postJSON(t, client, server.URL+"/api/auth/login", map[string]any{"username": "admin", "password": cfg.DemoPassword}, "", nil)
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	decode(t, loginResponse, &session)
	cookies := loginResponse.Cookies()
	versionID := publishVersionOverHTTP(t, client, server.URL, session.CSRF, cookies)
	runInput := map[string]any{
		"ruleset_version_id": versionID,
		"source":             map[string]any{"table": "company_license"},
		"target":             map[string]any{"table": "company_address_labels"},
		"limit":              20,
	}

	previewResponse := postJSON(t, client, server.URL+"/mcp", map[string]any{
		"jsonrpc": "2.0", "id": "preview", "method": "tools/call",
		"params": map[string]any{"name": "rulary.run.execute", "arguments": map[string]any{"operation": "preview", "input": runInput}},
	}, "", nil, "Bearer "+cfg.AgentToken)
	var previewBody map[string]any
	decode(t, previewResponse, &previewBody)
	previewResult := previewBody["result"].(map[string]any)
	if previewResult["isError"] != false {
		t.Fatalf("MCP preview = %#v", previewResult)
	}
	preview := previewResult["structuredContent"].(map[string]any)
	planHash := preview["plan_hash"].(string)
	if preview["impact"].(map[string]any)["rows"] != float64(20) {
		t.Fatalf("MCP preview impact = %#v", preview["impact"])
	}

	executeResponse := postJSON(t, client, server.URL+"/mcp", map[string]any{
		"jsonrpc": "2.0", "id": "execute", "method": "tools/call",
		"params": map[string]any{"name": "rulary.run.execute", "arguments": map[string]any{
			"operation": "execute", "input": runInput, "plan_hash": planHash, "idempotency_key": "mcp-adapter-run",
		}},
	}, "", nil, "Bearer "+cfg.AgentToken)
	var executeBody map[string]any
	decode(t, executeResponse, &executeBody)
	executeResult := executeBody["result"].(map[string]any)
	if executeResult["isError"] != false {
		t.Fatalf("MCP execute = %#v", executeResult)
	}
	run := executeResult["structuredContent"].(map[string]any)["run"].(map[string]any)
	if run["matched_rows"] != float64(20) || run["written_rows"] != float64(20) {
		t.Fatalf("MCP run = %#v", run)
	}

	var previewAudit, executeAudit int
	if err := application.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM modary_audit_log
		WHERE channel = 'mcp' AND action_id = 'rulary.run.execute' AND decision = 'previewed'`).Scan(&previewAudit); err != nil {
		t.Fatal(err)
	}
	if err := application.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM modary_audit_log
		WHERE channel = 'mcp' AND action_id = 'rulary.run.execute' AND decision = 'allowed'`).Scan(&executeAudit); err != nil {
		t.Fatal(err)
	}
	if previewAudit != 1 || executeAudit != 1 {
		t.Fatalf("MCP audit counts: preview=%d execute=%d", previewAudit, executeAudit)
	}
}

func TestActionDenialsAreStructuredAcrossHTTPAndMCP(t *testing.T) {
	application, cfg := testApplication(t)
	defer application.Close()
	server := httptest.NewServer(httpapi.New(application))
	defer server.Close()

	client := server.Client()
	loginResponse := postJSON(t, client, server.URL+"/api/auth/login", map[string]any{"username": "author", "password": cfg.DemoPassword}, "", nil)
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	decode(t, loginResponse, &session)
	denied := postJSON(t, client, server.URL+"/api/actions/rulary.ruleset.publish/preview", map[string]any{
		"input": map[string]any{"ruleset_id": "not-reached"},
	}, session.CSRF, loginResponse.Cookies())
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("HTTP denial status = %d", denied.StatusCode)
	}
	var deniedBody map[string]any
	decode(t, denied, &deniedBody)
	assertStructuredDenial(t, deniedBody["error"].(map[string]any))

	mcp := postJSON(t, client, server.URL+"/mcp", map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "tools/call",
		"params": map[string]any{"name": "rulary.ruleset.publish", "arguments": map[string]any{
			"operation": "preview", "input": map[string]any{"ruleset_id": "not-reached"},
		}},
	}, "", nil, "Bearer "+cfg.AgentToken)
	var mcpBody map[string]any
	decode(t, mcp, &mcpBody)
	result := mcpBody["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("MCP denial = %#v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	assertStructuredDenial(t, structured["error"].(map[string]any))
}

func TestUIAndAPIWorkWithoutAgentMCP(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Runtime{
		DataDir: dataDir, DatabasePath: filepath.Join(dataDir, "modary.db"), ListenAddress: "127.0.0.1:0",
		DemoPassword: "no-agent-password", AgentToken: "unused-agent-token",
	}
	application, err := app.BootstrapWithRegistrar(context.Background(), cfg, func(host *module.Host) error {
		return host.Register(
			database_sqlite.Module(), identity_local.Module(), authz_basic.Module(), audit_module.Module(),
			console_react.Module(), rulary_core.Module(),
		)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	if application.Host.HasModule("agent-mcp") {
		t.Fatal("agent-mcp should not be registered")
	}
	server := httptest.NewServer(httpapi.New(application))
	defer server.Close()

	ui, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer ui.Body.Close()
	if ui.StatusCode != http.StatusOK {
		t.Fatalf("UI status = %d", ui.StatusCode)
	}
	loginResponse := postJSON(t, server.Client(), server.URL+"/api/auth/login", map[string]any{"username": "admin", "password": cfg.DemoPassword}, "", nil)
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	decode(t, loginResponse, &session)
	apiResponse := postJSON(t, server.Client(), server.URL+"/api/actions/rulary.ruleset.list/execute", map[string]any{"input": map[string]any{"limit": 10}}, session.CSRF, loginResponse.Cookies())
	if apiResponse.StatusCode != http.StatusOK {
		t.Fatalf("API status = %d", apiResponse.StatusCode)
	}
	apiResponse.Body.Close()
	mcpResponse := postJSON(t, server.Client(), server.URL+"/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"}, "", nil, "Bearer unused")
	defer mcpResponse.Body.Close()
	if mcpResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("MCP status without module = %d", mcpResponse.StatusCode)
	}
}

func TestConsoleRouteIsOwnedByConsoleModule(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Runtime{
		DataDir: dataDir, DatabasePath: filepath.Join(dataDir, "modary.db"), ListenAddress: "127.0.0.1:0",
		DemoPassword: "headless-password", AgentToken: "headless-agent-token",
	}
	application, err := app.BootstrapWithRegistrar(context.Background(), cfg, func(host *module.Host) error {
		return host.Register(
			database_sqlite.Module(), identity_local.Module(), authz_basic.Module(), audit_module.Module(), rulary_core.Module(),
		)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	server := httptest.NewServer(httpapi.New(application))
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("headless root status = %d", response.StatusCode)
	}
}

func publishVersionOverHTTP(t *testing.T, client *http.Client, baseURL, csrf string, cookies []*http.Cookie) string {
	t.Helper()
	spec := map[string]any{
		"schema_version": "rulary.ruleset.f0", "id": "company-address", "name": "Address labels",
		"source":   map[string]any{"table": "company_license", "primary_key": "company_id", "field": "license_address"},
		"operator": map[string]any{"type": "rulary.address.extract_v1", "filing_marker": "经营地址备案", "parenthetical_note_target": "address_note"},
		"output":   map[string]any{"table": "company_address_labels", "unique_key": "company_id"},
	}
	createdResponse := postJSON(t, client, baseURL+"/api/actions/rulary.ruleset.create/execute", map[string]any{
		"input": map[string]any{"name": "Adapter flow", "spec": spec}, "idempotency_key": "adapter-create",
	}, csrf, cookies)
	var created map[string]any
	decode(t, createdResponse, &created)
	rulesetID := created["result"].(map[string]any)["ruleset"].(map[string]any)["id"].(string)
	input := map[string]any{"ruleset_id": rulesetID}
	validated := postJSON(t, client, baseURL+"/api/actions/rulary.ruleset.validate/execute", map[string]any{
		"input": input, "idempotency_key": "adapter-validate",
	}, csrf, cookies)
	validated.Body.Close()
	publishPreviewResponse := postJSON(t, client, baseURL+"/api/actions/rulary.ruleset.publish/preview", map[string]any{"input": input}, csrf, cookies)
	var publishPreview map[string]any
	decode(t, publishPreviewResponse, &publishPreview)
	planHash := publishPreview["preview"].(map[string]any)["plan_hash"].(string)
	publishedResponse := postJSON(t, client, baseURL+"/api/actions/rulary.ruleset.publish/execute", map[string]any{
		"input": input, "plan_hash": planHash, "idempotency_key": "adapter-publish",
	}, csrf, cookies)
	var published map[string]any
	decode(t, publishedResponse, &published)
	return published["result"].(map[string]any)["version"].(map[string]any)["id"].(string)
}

func assertStructuredDenial(t *testing.T, detail map[string]any) {
	t.Helper()
	required := []string{"error_code", "action_id", "required_permission", "actor_id", "workspace_id", "human_readable_reason", "request_id"}
	for _, field := range required {
		if value, ok := detail[field]; !ok || value == "" {
			t.Fatalf("structured denial missing %s: %#v", field, detail)
		}
	}
}

func testApplication(t *testing.T) (*app.Application, config.Runtime) {
	t.Helper()
	dataDir := t.TempDir()
	cfg := config.Runtime{
		DataDir: dataDir, DatabasePath: filepath.Join(dataDir, "modary.db"), ListenAddress: "127.0.0.1:0",
		DemoPassword: "http-test-password", AgentToken: "http-test-agent-token",
	}
	application, err := app.Bootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return application, cfg
}

func postJSON(t *testing.T, client *http.Client, url string, value any, csrf string, cookies []*http.Cookie, authorization ...string) *http.Response {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	if len(authorization) > 0 {
		request.Header.Set("Authorization", authorization[0])
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decode(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
