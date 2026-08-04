package httpapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestNewAPIValidatesBoundaryAndOptions(t *testing.T) {
	if handler, err := NewAPI(nil, APIOptions{}); err == nil || handler != nil {
		t.Fatalf("NewAPI(nil) = %#v, %v", handler, err)
	}
	application := newHTTPTestApplication(t, true)
	for _, test := range []struct {
		name    string
		options APIOptions
	}{
		{name: "cookie", options: APIOptions{CookieName: "bad cookie"}},
		{name: "negative body", options: APIOptions{MaxBodyBytes: -1}},
		{name: "unbounded body", options: APIOptions{MaxBodyBytes: MaximumBodyBytes + 1}},
		{name: "timeout", options: APIOptions{Timeout: -time.Second}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if handler, err := NewAPI(application.app, test.options); err == nil || handler != nil {
				t.Fatalf("NewAPI() = %#v, %v", handler, err)
			}
		})
	}

	withoutSessions := newHTTPTestApplication(t, false)
	if handler, err := NewAPI(withoutSessions.app, APIOptions{ResolveScope: fixedScope(scope.Must("tenant", "http-test"))}); err == nil || handler != nil || !errors.Is(err, appkit.ErrSessionsUnavailable) {
		t.Fatalf("NewAPI(missing sessions) = %#v, %v", handler, err)
	}

	var pointer *testAuthenticator
	if !isTypedNil(identity.SessionManager(pointer)) || isTypedNil(testAuthorizer{}) {
		t.Fatal("typed-nil dependency detection is not fail-closed")
	}
}

func TestRetainedAPIHandlerReportsApplicationShutdownAsUnavailable(t *testing.T) {
	t.Run("revoked before request", func(t *testing.T) {
		application := newHTTPTestApplication(t, true)
		handler := mustNewAPI(t, application.app, APIOptions{})
		cookie, _ := loginForTest(t, handler)
		if err := application.app.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}

		response := performRequest(handler, http.MethodGet, "/api/auth/session", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
		assertUnavailableAPIResponse(t, response)
	})

	t.Run("in flight authentication canceled by shutdown", func(t *testing.T) {
		application := newHTTPTestApplication(t, true)
		handler := mustNewAPI(t, application.app, APIOptions{})
		cookie, _ := loginForTest(t, handler)
		application.sessions.setBlockSession(true)

		response := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			defer close(done)
			request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
			request.Header.Set("Accept", "application/json")
			request.AddCookie(cookie)
			handler.ServeHTTP(response, request)
		}()
		application.sessions.waitForBlockedSession(t)
		if err := application.app.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("API request did not return after lifecycle cancellation")
		}
		assertUnavailableAPIResponse(t, response)
	})
}

func assertUnavailableAPIResponse(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var envelope errorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode unavailable response: %v; body=%s", err, response.Body.String())
	}
	if response.Code != http.StatusServiceUnavailable || envelope.Error.Code != action.CodeUnavailable || envelope.Error.Kind != action.ErrorKindUnavailable {
		t.Fatalf("unavailable response = %d %#v", response.Code, envelope)
	}
}

func TestWriteJSONMarshalFailureUsesCompletePublicErrorEnvelope(t *testing.T) {
	for _, test := range []struct {
		name      string
		value     any
		requestID string
	}{
		{name: "ordinary response", value: map[string]any{"unsupported": make(chan struct{})}},
		{
			name:      "prepared response preserves request id",
			value:     map[string]any{"unsupported": make(chan struct{})},
			requestID: "request-fallback",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			if test.requestID != "" {
				response.Header().Set("X-Request-ID", test.requestID)
			}
			writeJSON(response, http.StatusOK, test.value)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
			}
			var envelope errorEnvelope
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode fallback: %v; body=%q", err, response.Body.String())
			}
			if envelope.Error.Code != action.CodeInternal || envelope.Error.Kind != action.ErrorKindInternal || envelope.Error.Message != "internal server error" {
				t.Fatalf("fallback error = %#v", envelope.Error)
			}
			if envelope.RequestID != test.requestID || envelope.Error.RequestID != test.requestID {
				t.Fatalf("fallback request ids = %q, %q; want %q", envelope.RequestID, envelope.Error.RequestID, test.requestID)
			}
		})
	}
}

func TestAPIAuthenticationCookieAndCatalogContract(t *testing.T) {
	application := newHTTPTestApplication(t, true)
	handler := mustNewAPI(t, application.app, APIOptions{})

	for _, test := range []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		accept      string
		status      int
	}{
		{name: "method", method: http.MethodGet, path: "/api/auth/login", status: http.StatusMethodNotAllowed},
		{name: "content type", method: http.MethodPost, path: "/api/auth/login", body: `{}`, status: http.StatusUnsupportedMediaType},
		{name: "accept", method: http.MethodPost, path: "/api/auth/login", body: `{}`, contentType: "application/json", accept: "text/html", status: http.StatusNotAcceptable},
		{name: "unknown field", method: http.MethodPost, path: "/api/auth/login", body: `{"username":"admin","password":"secret","admin":true}`, contentType: "application/json", status: http.StatusBadRequest},
		{name: "non-canonical field", method: http.MethodPost, path: "/api/auth/login", body: `{"Username":"admin","password":"secret"}`, contentType: "application/json", status: http.StatusBadRequest},
		{name: "duplicate field", method: http.MethodPost, path: "/api/auth/login", body: `{"username":"admin","username":"other","password":"secret"}`, contentType: "application/json", status: http.StatusBadRequest},
		{name: "trailing value", method: http.MethodPost, path: "/api/auth/login", body: `{"username":"admin","password":"secret"} {}`, contentType: "application/json", status: http.StatusBadRequest},
		{name: "root array", method: http.MethodPost, path: "/api/auth/login", body: `[]`, contentType: "application/json", status: http.StatusBadRequest},
		{name: "query", method: http.MethodPost, path: "/api/auth/login?debug=true", body: `{}`, contentType: "application/json", status: http.StatusBadRequest},
		{name: "route", method: http.MethodGet, path: "/api/internal/modules", accept: "application/json", status: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := performRequest(handler, test.method, test.path, test.body, test.contentType, test.accept, nil, nil, false)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
			assertSecurityHeaders(t, response)
		})
	}

	denied := performRequest(handler, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"wrong"}`, "application/json", "application/json", nil, nil, false)
	if denied.Code != http.StatusUnauthorized || len(denied.Result().Cookies()) != 0 || strings.Contains(denied.Body.String(), "wrong") {
		t.Fatalf("denied login = %d %s", denied.Code, denied.Body.String())
	}
	application.sessions.setLoginError(errors.New("identity store password=secret unavailable"))
	failedLogin := performRequest(handler, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"secret"}`, "application/json", "application/json", nil, nil, false)
	if failedLogin.Code != http.StatusInternalServerError || len(failedLogin.Result().Cookies()) != 0 || strings.Contains(failedLogin.Body.String(), "identity store") || strings.Contains(failedLogin.Body.String(), "secret") {
		t.Fatalf("failed login = %d %s", failedLogin.Code, failedLogin.Body.String())
	}
	application.sessions.setLoginError(nil)

	login := performRequest(handler, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"secret"}`, "application/json; charset=utf-8", "application/json", nil, map[string]string{"X-Request-ID": "client-request-1"}, false)
	if login.Code != http.StatusOK || login.Header().Get("X-Request-ID") != "client-request-1" {
		t.Fatalf("login = %d %s", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookies = %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != DefaultCookieName || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("session cookie = %#v", cookie)
	}
	if strings.Contains(login.Body.String(), cookie.Value) || strings.Contains(login.Body.String(), "secret") {
		t.Fatalf("login response leaked credential: %s", login.Body.String())
	}
	var authenticated loginResponse
	decodeResponse(t, login, &authenticated)
	if authenticated.CSRFToken == "" || authenticated.Actor != application.actor {
		t.Fatalf("login response = %#v", authenticated)
	}
	currentSession := performRequest(handler, http.MethodGet, "/api/auth/session", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if currentSession.Code != http.StatusOK || strings.Contains(currentSession.Body.String(), cookie.Value) {
		t.Fatalf("session = %d %s", currentSession.Code, currentSession.Body.String())
	}
	var current loginResponse
	decodeResponse(t, currentSession, &current)
	if current.Actor != application.actor || current.CSRFToken != authenticated.CSRFToken || !current.ExpiresAt.Equal(authenticated.ExpiresAt) {
		t.Fatalf("session response = %#v", current)
	}

	unauthorized := performRequest(handler, http.MethodGet, "/api/actions", "", "", "application/json", nil, nil, false)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized catalog = %d %s", unauthorized.Code, unauthorized.Body.String())
	}

	duplicate := performRequest(handler, http.MethodGet, "/api/actions", "", "", "application/json", []*http.Cookie{cookie, cookie}, nil, false)
	if duplicate.Code != http.StatusUnauthorized {
		t.Fatalf("duplicate cookie status = %d", duplicate.Code)
	}

	catalog := performRequest(handler, http.MethodGet, "/api/actions", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if catalog.Code != http.StatusOK {
		t.Fatalf("catalog = %d %s", catalog.Code, catalog.Body.String())
	}
	var listed struct {
		Actions []action.CatalogEntry `json:"actions"`
	}
	decodeResponse(t, catalog, &listed)
	if len(listed.Actions) != 1 || listed.Actions[0].Descriptor.ID != "example.echo" || listed.Actions[0].Descriptor.Version != "1.2.3" || listed.Actions[0].ContractHash == "" {
		t.Fatalf("HTTP catalog = %#v", listed.Actions)
	}
	if listed.Actions[0].ModuleID != "unrelated-module-name" {
		t.Fatalf("catalog rewrote owner: %#v", listed.Actions[0])
	}

	hidden := performRequest(handler, http.MethodGet, "/api/actions/example.agent/schema", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if hidden.Code != http.StatusNotFound {
		t.Fatalf("non-HTTP schema status = %d", hidden.Code)
	}
	visible := performRequest(handler, http.MethodGet, "/api/actions/example.echo/schema", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if visible.Code != http.StatusOK || !strings.Contains(visible.Body.String(), `"contract_hash":"sha256:`) {
		t.Fatalf("HTTP schema = %d %s", visible.Code, visible.Body.String())
	}
}

func TestAPIInsecureCookieRequiresExplicitOptIn(t *testing.T) {
	application := newHTTPTestApplication(t, true)
	handler := mustNewAPI(t, application.app, APIOptions{AllowInsecureCookie: true})
	login := performRequest(handler, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"secret"}`, "application/json", "application/json", nil, nil, false)
	if login.Code != http.StatusOK || len(login.Result().Cookies()) != 1 || login.Result().Cookies()[0].Secure {
		t.Fatalf("insecure development cookie = %d %#v", login.Code, login.Result().Cookies())
	}
	cookie := login.Result().Cookies()[0]
	var body loginResponse
	decodeResponse(t, login, &body)
	logout := performRequest(handler, http.MethodPost, "/api/auth/logout", `{}`, "application/json", "application/json", []*http.Cookie{cookie}, map[string]string{"X-CSRF-Token": body.CSRFToken}, false)
	if logout.Code != http.StatusNoContent || len(logout.Result().Cookies()) != 1 || logout.Result().Cookies()[0].Secure {
		t.Fatalf("insecure development cookie deletion = %d %#v", logout.Code, logout.Result().Cookies())
	}
}

func TestAPIPreviewExecuteCSRFAndScope(t *testing.T) {
	application := newHTTPTestApplication(t, true)
	handler := mustNewAPI(t, application.app, APIOptions{})
	cookie, csrf := loginForTest(t, handler)

	for _, headers := range []map[string]string{nil, {"X-CSRF-Token": "wrong"}} {
		response := performRequest(handler, http.MethodPost, "/api/actions/example.echo/preview", `{"input":{"message":"hello"}}`, "application/json", "application/json", []*http.Cookie{cookie}, headers, false)
		if response.Code != http.StatusForbidden {
			t.Fatalf("invalid CSRF status = %d", response.Code)
		}
	}
	duplicateCSRF := httptest.NewRequest(http.MethodPost, "/api/actions/example.echo/preview", strings.NewReader(`{"input":{"message":"hello"}}`))
	duplicateCSRF.Header.Set("Content-Type", "application/json")
	duplicateCSRF.Header["X-Csrf-Token"] = []string{csrf, csrf}
	duplicateCSRF.AddCookie(cookie)
	duplicateCSRFResponse := httptest.NewRecorder()
	handler.ServeHTTP(duplicateCSRFResponse, duplicateCSRF)
	if duplicateCSRFResponse.Code != http.StatusForbidden {
		t.Fatalf("duplicate CSRF status = %d", duplicateCSRFResponse.Code)
	}

	preview := performRequest(handler, http.MethodPost, "/api/actions/example.echo/preview", `{"input":{"message":"hello"}}`, "application/json", "application/json", []*http.Cookie{cookie}, map[string]string{"X-CSRF-Token": csrf}, false)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", preview.Code, preview.Body.String())
	}
	var previewBody struct {
		Preview action.Preview `json:"preview"`
	}
	decodeResponse(t, preview, &previewBody)
	if previewBody.Preview.PlanHash == "" {
		t.Fatalf("preview response = %#v", previewBody)
	}

	invalid := performRequest(handler, http.MethodPost, "/api/actions/example.echo/execute", `{"input":{}}`, "application/json", "application/json", []*http.Cookie{cookie}, map[string]string{"X-CSRF-Token": csrf}, false)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("schema-invalid input = %d %s", invalid.Code, invalid.Body.String())
	}
	duplicateInput := performRequest(handler, http.MethodPost, "/api/actions/example.echo/execute", `{"input":{"message":"one","message":"two"}}`, "application/json", "application/json", []*http.Cookie{cookie}, map[string]string{"X-CSRF-Token": csrf}, false)
	if duplicateInput.Code != http.StatusBadRequest {
		t.Fatalf("duplicate nested input = %d %s", duplicateInput.Code, duplicateInput.Body.String())
	}
	hidden := performRequest(handler, http.MethodPost, "/api/actions/example.agent/execute", `{"input":{"message":"hello"}}`, "application/json", "application/json", []*http.Cookie{cookie}, map[string]string{"X-CSRF-Token": csrf}, false)
	if hidden.Code != http.StatusNotFound {
		t.Fatalf("non-HTTP execute = %d %s", hidden.Code, hidden.Body.String())
	}

	executeBody := fmt.Sprintf(`{"input":{"message":"hello"},"plan_hash":%q,"idempotency_key":"http-once"}`, previewBody.Preview.PlanHash)
	executed := performRequest(handler, http.MethodPost, "/api/actions/example.echo/execute", executeBody, "application/json", "application/json", []*http.Cookie{cookie}, map[string]string{"X-CSRF-Token": csrf, "X-Request-ID": "execute-request-1"}, false)
	if executed.Code != http.StatusOK || !strings.Contains(executed.Body.String(), `"echo":"ok"`) || !strings.Contains(executed.Body.String(), `"kind":"example"`) {
		t.Fatalf("execute = %d %s", executed.Code, executed.Body.String())
	}
	plan := application.handler.executedPlan()
	if plan.Channel != action.ChannelHTTP || plan.Scope != application.executionScope || plan.ActorID != application.actor.ID || plan.ActorType != application.actor.Type {
		t.Fatalf("Runtime plan boundary = %#v", plan)
	}

	wrongMethod := performRequest(handler, http.MethodGet, "/api/actions/example.echo/execute", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("wrong execute method = %d Allow=%q", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}
}

func TestHTTPActionInputPresenceAndNumberClassification(t *testing.T) {
	application := newHTTPTestApplication(t, true)
	handler := mustNewAPI(t, application.app, APIOptions{})
	cookie, csrf := loginForTest(t, handler)
	headers := map[string]string{"X-CSRF-Token": csrf}

	assertFailure := func(body string, wantCode string) *httptest.ResponseRecorder {
		t.Helper()
		response := performRequest(
			handler, http.MethodPost, "/api/actions/example.echo/preview", body,
			"application/json", "application/json", []*http.Cookie{cookie}, headers, false,
		)
		var envelope errorEnvelope
		decodeResponse(t, response, &envelope)
		if envelope.Error.Code != wantCode {
			t.Fatalf("body %d bytes error = %#v; response=%s", len(body), envelope.Error, response.Body.String())
		}
		return response
	}

	assertFailure(`{}`, action.CodeValidationFailed)
	if events := application.audit.snapshot(); len(events) != 0 {
		t.Fatalf("missing protocol input reached Runtime audit: %#v", events)
	}

	assertFailure(`{"input":null}`, action.CodeValidationFailed)
	assertFailure(`{"input":{}}`, action.CodeValidationFailed)
	exact := protocolNumberJSON(action.MaxJSONNumberBytes)
	assertFailure(`{"input":`+string(exact)+`}`, action.CodeValidationFailed)
	above := protocolNumberJSON(action.MaxJSONNumberBytes + 1)
	assertFailure(`{"input":`+string(above)+`}`, action.CodeLimitExceeded)

	events := application.audit.snapshot()
	if len(events) != 4 {
		t.Fatalf("Runtime audit events = %d, want 4: %#v", len(events), events)
	}
	for index, event := range events {
		wantCode := action.CodeValidationFailed
		if index == len(events)-1 {
			wantCode = action.CodeLimitExceeded
		}
		if event.Decision != "rejected" || event.ErrorCode != wantCode {
			t.Fatalf("audit event %d = %#v", index, event)
		}
		if index == len(events)-1 && event.InputHash != "" {
			t.Fatalf("above-limit number retained input hash: %#v", event)
		}
	}
}

func TestHTTPActionJSONBoundaryMatrixReachesRuntimeWithoutProtocolBudgetLeakage(t *testing.T) {
	for _, boundary := range transportActionJSONBoundaries() {
		t.Run(boundary.name, func(t *testing.T) {
			application := newHTTPTestApplicationWithInputSchema(t, true, json.RawMessage(`true`))
			handler := mustNewAPI(t, application.app, APIOptions{})
			cookie, csrf := loginForTest(t, handler)
			headers := map[string]string{"X-CSRF-Token": csrf}

			call := func(input json.RawMessage) (*httptest.ResponseRecorder, errorEnvelope) {
				t.Helper()
				body := append([]byte(`{"input":`), input...)
				body = append(body, '}')
				response := performRequest(
					handler, http.MethodPost, "/api/actions/example.echo/preview", string(body),
					"application/json", "application/json", []*http.Cookie{cookie}, headers, false,
				)
				var envelope errorEnvelope
				if response.Code != http.StatusOK {
					decodeResponse(t, response, &envelope)
				}
				return response, envelope
			}

			exact, exactEnvelope := call(boundary.exact)
			if exact.Code != http.StatusOK || exactEnvelope.Error.Code != "" {
				t.Fatalf("exact Action boundary = %d %#v; body=%s", exact.Code, exactEnvelope, exact.Body.String())
			}
			if got := application.handler.planCount(); got != 1 {
				t.Fatalf("exact Action boundary Plan calls = %d, want 1", got)
			}

			above, aboveEnvelope := call(boundary.above)
			if above.Code != http.StatusUnprocessableEntity || aboveEnvelope.Error.Code != action.CodeLimitExceeded || aboveEnvelope.Error.Kind != action.ErrorKindLimit {
				t.Fatalf("above Action boundary = %d %#v; body=%s", above.Code, aboveEnvelope, above.Body.String())
			}
			if got := application.handler.planCount(); got != 1 {
				t.Fatalf("above Action boundary reached Handler.Plan: calls=%d", got)
			}

			events := application.audit.snapshot()
			if len(events) != 2 {
				t.Fatalf("Runtime audit events = %d, want exact and above: %#v", len(events), events)
			}
			if events[0].Decision != "previewed" || events[0].ErrorCode != "" || events[0].InputHash == "" {
				t.Fatalf("exact Action audit = %#v", events[0])
			}
			if events[1].Decision != "rejected" || events[1].ErrorCode != action.CodeLimitExceeded || events[1].InputHash != "" {
				t.Fatalf("above Action audit = %#v", events[1])
			}
		})
	}
}

type transportActionJSONBoundary struct {
	name  string
	exact json.RawMessage
	above json.RawMessage
}

func transportActionJSONBoundaries() []transportActionJSONBoundary {
	return []transportActionJSONBoundary{
		{
			name:  "bytes",
			exact: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-2) + `"`),
			above: json.RawMessage(`"` + strings.Repeat("x", int(action.MaxJSONDocumentBytes)-1) + `"`),
		},
		{
			name:  "depth",
			exact: protocolNestedJSON(action.MaxJSONNestingDepth),
			above: protocolNestedJSON(action.MaxJSONNestingDepth + 1),
		},
		{
			name:  "nodes",
			exact: protocolArrayJSON(action.MaxJSONValueNodes - 1),
			above: protocolArrayJSON(action.MaxJSONValueNodes),
		},
		{
			name:  "number",
			exact: protocolNumberJSON(action.MaxJSONNumberBytes),
			above: protocolNumberJSON(action.MaxJSONNumberBytes + 1),
		},
	}
}

func TestAPIBodyLimitSessionLogoutFailureAndRecovery(t *testing.T) {
	application := newHTTPTestApplication(t, true)
	handler := mustNewAPI(t, application.app, APIOptions{MaxBodyBytes: 64, Timeout: 10 * time.Millisecond})

	tooLarge := performRequest(handler, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"`+strings.Repeat("x", 100)+`"}`, "application/json", "application/json", nil, nil, false)
	if tooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large body = %d %s", tooLarge.Code, tooLarge.Body.String())
	}

	cookie, csrf := loginForTest(t, handler)
	for _, body := range []string{`{"unexpected":true}`, `{} {}`} {
		invalidLogout := performRequest(handler, http.MethodPost, "/api/auth/logout", body, "application/json", "application/json", []*http.Cookie{cookie}, map[string]string{"X-CSRF-Token": csrf}, false)
		if invalidLogout.Code != http.StatusBadRequest || len(invalidLogout.Result().Cookies()) != 0 {
			t.Fatalf("invalid logout = %d %s cookies=%#v", invalidLogout.Code, invalidLogout.Body.String(), invalidLogout.Result().Cookies())
		}
	}
	application.sessions.setLogoutError(errors.New("database password=secret"))
	failedLogout := performRequest(handler, http.MethodPost, "/api/auth/logout", `{}`, "application/json", "application/json", []*http.Cookie{cookie}, map[string]string{"X-CSRF-Token": csrf}, false)
	if failedLogout.Code != http.StatusInternalServerError || len(failedLogout.Result().Cookies()) != 0 || strings.Contains(failedLogout.Body.String(), "database") || strings.Contains(failedLogout.Body.String(), "secret") {
		t.Fatalf("failed logout = %d %s cookies=%#v", failedLogout.Code, failedLogout.Body.String(), failedLogout.Result().Cookies())
	}

	application.sessions.setLogoutError(nil)
	logout := performRequest(handler, http.MethodPost, "/api/auth/logout", `{}`, "application/json", "application/json", []*http.Cookie{cookie}, map[string]string{"X-CSRF-Token": csrf}, false)
	if logout.Code != http.StatusNoContent || len(logout.Result().Cookies()) != 1 || logout.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("logout = %d cookies=%#v", logout.Code, logout.Result().Cookies())
	}

	application.sessions.setSessionError(errors.New("store unavailable"))
	failedSession := performRequest(handler, http.MethodGet, "/api/auth/session", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if failedSession.Code != http.StatusInternalServerError || len(failedSession.Result().Cookies()) != 0 || strings.Contains(failedSession.Body.String(), "store") {
		t.Fatalf("failed session = %d %s cookies=%#v", failedSession.Code, failedSession.Body.String(), failedSession.Result().Cookies())
	}

	application.sessions.setSessionError(identity.ErrSessionInvalid)
	invalidSession := performRequest(handler, http.MethodGet, "/api/auth/session", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if invalidSession.Code != http.StatusUnauthorized || len(invalidSession.Result().Cookies()) != 1 || invalidSession.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("invalid session = %d cookies=%#v", invalidSession.Code, invalidSession.Result().Cookies())
	}

	application.sessions.setSessionError(nil)
	application.sessions.setBlockSession(true)
	timedOut := performRequest(handler, http.MethodGet, "/api/auth/session", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if timedOut.Code != http.StatusGatewayTimeout {
		t.Fatalf("session timeout = %d %s", timedOut.Code, timedOut.Body.String())
	}
	application.sessions.setBlockSession(false)

	application.sessions.setPanicSession(true)
	panicked := performRequest(handler, http.MethodGet, "/api/auth/session", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if panicked.Code != http.StatusInternalServerError || strings.Contains(panicked.Body.String(), "identity panic") {
		t.Fatalf("panic response = %d %s", panicked.Code, panicked.Body.String())
	}
}

func TestSessionContractViolationsAndInvalidAuthenticationAreSeparated(t *testing.T) {
	application := newHTTPTestApplication(t, true)
	handler := mustNewAPI(t, application.app, APIOptions{})
	cookie, _ := loginForTest(t, handler)

	application.sessions.mutateSession(cookie.Value, func(session *identity.Session) {
		session.Actor.ID = ""
	})
	malformed := performRequest(handler, http.MethodGet, "/api/auth/session", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if malformed.Code != http.StatusInternalServerError || len(malformed.Result().Cookies()) != 0 ||
		!strings.Contains(malformed.Body.String(), `"human_readable_reason":"internal server error"`) || strings.Contains(malformed.Body.String(), "actor") {
		t.Fatalf("malformed dependency session = %d %s cookies=%#v", malformed.Code, malformed.Body.String(), malformed.Result().Cookies())
	}

	application.sessions.mutateSession(cookie.Value, func(session *identity.Session) {
		session.Actor = application.actor
		session.ExpiresAt = time.Now().Add(-time.Minute)
	})
	expired := performRequest(handler, http.MethodGet, "/api/auth/session", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if expired.Code != http.StatusUnauthorized || len(expired.Result().Cookies()) != 1 || expired.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("expired session = %d %s cookies=%#v", expired.Code, expired.Body.String(), expired.Result().Cookies())
	}

	application.sessions.deleteSession(cookie.Value)
	unknown := performRequest(handler, http.MethodGet, "/api/auth/session", "", "", "application/json", []*http.Cookie{cookie}, nil, false)
	if unknown.Code != http.StatusUnauthorized || len(unknown.Result().Cookies()) != 1 || unknown.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("unknown session = %d %s cookies=%#v", unknown.Code, unknown.Body.String(), unknown.Result().Cookies())
	}
}

func TestActionErrorHTTPMappingAndCauseRedaction(t *testing.T) {
	for _, test := range []struct {
		code   string
		status int
	}{
		{action.CodeValidationFailed, http.StatusBadRequest},
		{action.CodeAuthzDenied, http.StatusForbidden},
		{action.CodeActionNotFound, http.StatusNotFound},
		{action.CodePlanNotFound, http.StatusNotFound},
		{action.CodePreconditionFailed, http.StatusPreconditionFailed},
		{action.CodePlanRequired, http.StatusPreconditionRequired},
		{action.CodeIdempotencyRequired, http.StatusPreconditionRequired},
		{action.CodePlanStale, http.StatusConflict},
		{action.CodeIdempotencyConflict, http.StatusConflict},
		{action.CodeIdempotencyProgress, http.StatusConflict},
		{action.CodeLimitExceeded, http.StatusUnprocessableEntity},
		{action.CodeUnavailable, http.StatusServiceUnavailable},
	} {
		t.Run(test.code, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeActionError(response, "request-1", context.Background(), action.NewError(test.code, "safe reason"))
			if response.Code != test.status || !strings.Contains(response.Body.String(), "safe reason") {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}

	for name, err := range map[string]error{
		"internal code":    &action.Error{Code: action.CodeInternal, Message: "database password=secret", Cause: errors.New("driver secret")},
		"unknown code":     action.NewError("CUSTOM_SECRET", "private detail"),
		"non-action error": errors.New("raw database secret"),
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeActionError(response, "request-1", context.Background(), err)
			if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), "database") {
				t.Fatalf("internal error leaked: %d %s", response.Code, response.Body.String())
			}
		})
	}
	var nilActionError *action.Error
	typedNilResponse := httptest.NewRecorder()
	writeActionError(typedNilResponse, "request-1", context.Background(), nilActionError)
	if typedNilResponse.Code != http.StatusInternalServerError {
		t.Fatalf("typed-nil Action error status = %d", typedNilResponse.Code)
	}
	for name, test := range map[string]struct {
		ctx    context.Context
		err    error
		status int
	}{
		"deadline": {ctx: expiredContext(t), err: errors.New("runtime stopped"), status: http.StatusGatewayTimeout},
		"canceled": {ctx: canceledContext(t), err: errors.New("runtime stopped"), status: http.StatusServiceUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeActionError(response, "request-1", test.ctx, test.err)
			if response.Code != test.status || strings.Contains(response.Body.String(), "wrapped") {
				t.Fatalf("context error response = %d %s", response.Code, response.Body.String())
			}
		})
	}
	for _, injected := range []error{context.Canceled, context.DeadlineExceeded} {
		response := httptest.NewRecorder()
		writeActionError(response, "request-1", context.Background(), fmt.Errorf("dependency: %w", injected))
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("dependency context sentinel status = %d", response.Code)
		}
	}
}

func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func expiredContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	t.Cleanup(cancel)
	return ctx
}

func TestJSONAndNegotiationHelpersAreStrict(t *testing.T) {
	for _, source := range []string{
		`{"outer":{"value":1,"value":2}}`,
		`{"items":[{"value":1,"value":2}]}`,
		`{"value":1} {"value":2}`,
	} {
		if err := validateSingleJSON([]byte(source)); err == nil {
			t.Fatalf("validateSingleJSON(%s) succeeded", source)
		}
	}
	if err := validateSingleJSON([]byte(`{"items":[{"value":1}],"ok":true}`)); err != nil {
		t.Fatalf("valid JSON rejected: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header["Content-Type"] = []string{"application/json", "application/json"}
	if hasJSONContentType(request) {
		t.Fatal("duplicate Content-Type was accepted")
	}
	request.Header.Set("Accept", "application/json;q=0, text/html")
	if acceptsJSON(request) {
		t.Fatal("q=0 JSON Accept was accepted")
	}
	request.Header.Set("Accept", "application/*;q=0.5")
	if !acceptsJSON(request) {
		t.Fatal("application wildcard was rejected")
	}
	request.Header.Set("Accept", "application/json;q=0, */*;q=1")
	if acceptsJSON(request) {
		t.Fatal("specific JSON exclusion was overridden by a wildcard")
	}
	request.Header.Set("Accept", `text/html; profile="a,b", application/json`)
	if !acceptsJSON(request) {
		t.Fatal("quoted comma in Accept header was parsed as a separator")
	}
	if validSessionToken("unsafe;token") || !validSessionToken("safe-token_123") {
		t.Fatal("session token cookie validation is not strict")
	}
	if !constantTimeTokenEqual("csrf", "csrf") || constantTimeTokenEqual("csrf", "other") {
		t.Fatal("CSRF comparison returned an invalid result")
	}
	invalidUTF8 := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte{'{', '"', 'u', 's', 'e', 'r', 'n', 'a', 'm', 'e', '"', ':', '"', 0xff, '"', '}'}))
	if err := decodeRequestJSON(httptest.NewRecorder(), invalidUTF8, 1024, &loginRequest{}); err == nil {
		t.Fatal("invalid UTF-8 JSON was accepted")
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header["X-Request-Id"] = []string{"first", "second"}
	response := httptest.NewRecorder()
	requestID := prepareResponse(response, request)
	if requestID == "first" || requestID == "second" || !requestIDPattern.MatchString(requestID) {
		t.Fatalf("duplicate caller request IDs produced %q", requestID)
	}
}

type httpTestApplication struct {
	app            *appkit.Application
	actor          identity.Actor
	sessions       *testAuthenticator
	handler        *testActionHandler
	audit          *mcpAuditRecorder
	executionScope scope.Execution
}

func newHTTPTestApplication(t *testing.T, withSessions bool) *httpTestApplication {
	return newHTTPTestApplicationWithInputSchema(t, withSessions, nil)
}

func newHTTPTestApplicationWithInputSchema(t *testing.T, withSessions bool, inputSchema json.RawMessage) *httpTestApplication {
	t.Helper()
	executionScope := scope.Must("tenant", "http-test")
	actor := identity.Actor{ID: "user-1", Type: "user", DisplayName: "Test User"}
	sessions := newTestAuthenticator(actor)
	handler := &testActionHandler{}
	auditHook := &mcpAuditRecorder{}
	provides := []module.Capability{
		module.CapabilityDatabase,
		module.CapabilityAuthorization,
		module.CapabilityAudit,
		"example",
	}
	if withSessions {
		provides = append(provides, module.CapabilityIdentity, module.CapabilityPasswords, module.CapabilitySessions)
	}
	manifest := module.Manifest{
		SchemaVersion: module.SchemaVersion, ID: "unrelated-module-name", Version: "1.0.0", Type: module.ModuleTypeFeature, Provides: provides,
	}
	start := func(_ context.Context, install module.Scope) error {
		if err := moduleassembly.ProvideActionPersistence(install, testsupport.NewMemoryPlanStore(), testsupport.NewMemoryIdempotencyStore(), testsupport.DirectTransactions{}); err != nil {
			return err
		}
		if err := module.Provide(install, module.Authorizer(), authz.Authorizer(testAuthorizer{})); err != nil {
			return err
		}
		if err := module.Provide(install, module.AuditHook(), audit.Hook(auditHook)); err != nil {
			return err
		}
		if withSessions {
			if err := module.Provide(install, module.IdentityResolver(), identity.Resolver(sessions)); err != nil {
				return err
			}
			if err := module.Provide(install, module.PasswordAuthenticator(), identity.PasswordAuthenticator(sessions)); err != nil {
				return err
			}
			if err := module.Provide(install, module.SessionManager(), identity.SessionManager(sessions)); err != nil {
				return err
			}
		}
		return nil
	}
	echoDescriptor := testActionDescriptor("example.echo", []action.Channel{action.ChannelHTTP}, action.PreviewOptional)
	if inputSchema != nil {
		echoDescriptor.InputSchema = append(json.RawMessage(nil), inputSchema...)
	}
	bindings := []module.ActionBinding{
		{Descriptor: echoDescriptor, NewHandler: func(context.Context, module.Resolver) (action.Handler, error) { return handler, nil }},
		{Descriptor: testActionDescriptor("example.agent", []action.Channel{action.ChannelMCP}, action.PreviewNone), NewHandler: func(context.Context, module.Resolver) (action.Handler, error) { return handler, nil }},
	}
	registration := module.Register(manifest, start, bindings...)
	application, err := appkit.Start(context.Background(), appkit.Definition{
		Metadata: appkit.Metadata{ID: "example", Name: "Example Application", Version: "1.0.0"},
		Modules:  []module.Registration{registration},
	}, appkit.Options{})
	if err != nil {
		t.Fatalf("appkit.Start() error = %v", err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	return &httpTestApplication{app: application, actor: actor, sessions: sessions, handler: handler, audit: auditHook, executionScope: executionScope}
}

func testActionDescriptor(id string, channels []action.Channel, preview action.PreviewPolicy) action.Descriptor {
	descriptor := action.Descriptor{
		ID: id, Version: "1.2.3", Title: "Echo", Permission: "example.echo", Preview: preview,
		AuditLevel: action.AuditMetadata, Channels: channels,
		InputSchema: action.Object(map[string]action.Field{
			"message": action.RequiredField(action.String(action.MinLength(1))),
		}).JSON(),
		OutputSchema: action.Object(map[string]action.Field{
			"echo": action.RequiredField(action.String()),
		}).JSON(),
	}
	if preview != action.PreviewNone {
		descriptor.PreviewSchema = action.Object(map[string]action.Field{
			"matched_rows": action.RequiredField(action.Integer()),
		}).JSON()
	}
	return descriptor
}

type testAuthorizer struct{}

func (testAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Allowed: true, Fingerprint: "http-test-policy"}, nil
}

type testActionHandler struct {
	mu         sync.Mutex
	planned    int
	executed   action.Plan
	executions int
}

func (handler *testActionHandler) Plan(_ context.Context, request action.Request) (action.PlanData, error) {
	handler.mu.Lock()
	handler.planned++
	handler.mu.Unlock()
	return action.PlanData{
		Payload: request.Input, Summary: json.RawMessage(`{"matched_rows":1}`),
		Impact: authz.Impact{Rows: 1, Resources: []string{"example/one"}},
	}, nil
}

func (handler *testActionHandler) Execute(_ context.Context, plan action.Plan) (action.Result, error) {
	handler.mu.Lock()
	handler.executed = plan
	handler.executions++
	handler.mu.Unlock()
	return action.Result{
		Data: json.RawMessage(`{"echo":"ok"}`), Summary: "echoed",
		References: []audit.Reference{{Kind: "example", ID: "one"}},
	}, nil
}

func (handler *testActionHandler) executedPlan() action.Plan {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	return handler.executed
}

func (handler *testActionHandler) planCount() int {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	return handler.planned
}

func (handler *testActionHandler) executionCount() int {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	return handler.executions
}

type testAuthenticator struct {
	mu           sync.Mutex
	actor        identity.Actor
	sessions     map[string]identity.Session
	sessionStart chan struct{}
	sessionOnce  sync.Once
	loginErr     error
	logoutErr    error
	sessionErr   error
	blockSession bool
	panicSession bool
}

func newTestAuthenticator(actor identity.Actor) *testAuthenticator {
	return &testAuthenticator{actor: actor, sessions: make(map[string]identity.Session), sessionStart: make(chan struct{})}
}

func (authenticator *testAuthenticator) ResolveByID(_ context.Context, id string) (identity.Actor, error) {
	if id != authenticator.actor.ID {
		return identity.Actor{}, identity.ErrActorNotFound
	}
	return authenticator.actor, nil
}

func (authenticator *testAuthenticator) AuthenticatePassword(_ context.Context, username, password string) (identity.Authentication, error) {
	authenticator.mu.Lock()
	loginErr := authenticator.loginErr
	authenticator.mu.Unlock()
	if loginErr != nil {
		return identity.Authentication{}, loginErr
	}
	if username != "admin" || password != "secret" {
		return identity.Authentication{}, identity.ErrAuthenticationFailed
	}
	return identity.Authentication{Actor: authenticator.actor, Method: identity.AuthenticationMethodPassword, CredentialVersion: "version"}, nil
}

func (authenticator *testAuthenticator) CreateSession(_ context.Context, authentication identity.Authentication) (identity.Session, error) {
	session := identity.Session{
		Token: "session-token-1", CSRFToken: "csrf-token-1", Actor: authentication.Actor, ExpiresAt: time.Now().Add(time.Hour),
	}
	authenticator.mu.Lock()
	authenticator.sessions[session.Token] = session
	authenticator.mu.Unlock()
	return session, nil
}

func (authenticator *testAuthenticator) RevokeSession(_ context.Context, token string) error {
	authenticator.mu.Lock()
	defer authenticator.mu.Unlock()
	if authenticator.logoutErr != nil {
		return authenticator.logoutErr
	}
	delete(authenticator.sessions, token)
	return nil
}

func (authenticator *testAuthenticator) ResolveSession(ctx context.Context, token string) (identity.Session, error) {
	authenticator.mu.Lock()
	panicSession := authenticator.panicSession
	block := authenticator.blockSession
	err := authenticator.sessionErr
	session, ok := authenticator.sessions[token]
	authenticator.mu.Unlock()
	if panicSession {
		panic("identity panic secret")
	}
	if block {
		authenticator.sessionOnce.Do(func() { close(authenticator.sessionStart) })
		<-ctx.Done()
		return identity.Session{}, ctx.Err()
	}
	if err != nil {
		return identity.Session{}, err
	}
	if !ok {
		return identity.Session{}, identity.ErrSessionInvalid
	}
	return session, nil
}

func (authenticator *testAuthenticator) setLogoutError(err error) {
	authenticator.mu.Lock()
	authenticator.logoutErr = err
	authenticator.mu.Unlock()
}

func (authenticator *testAuthenticator) setLoginError(err error) {
	authenticator.mu.Lock()
	authenticator.loginErr = err
	authenticator.mu.Unlock()
}

func (authenticator *testAuthenticator) setSessionError(err error) {
	authenticator.mu.Lock()
	authenticator.sessionErr = err
	authenticator.mu.Unlock()
}

func (authenticator *testAuthenticator) mutateSession(token string, mutate func(*identity.Session)) {
	authenticator.mu.Lock()
	defer authenticator.mu.Unlock()
	session := authenticator.sessions[token]
	mutate(&session)
	authenticator.sessions[token] = session
}

func (authenticator *testAuthenticator) deleteSession(token string) {
	authenticator.mu.Lock()
	delete(authenticator.sessions, token)
	authenticator.mu.Unlock()
}

func (authenticator *testAuthenticator) setBlockSession(value bool) {
	authenticator.mu.Lock()
	authenticator.blockSession = value
	authenticator.mu.Unlock()
}

func (authenticator *testAuthenticator) waitForBlockedSession(t *testing.T) {
	t.Helper()
	select {
	case <-authenticator.sessionStart:
	case <-time.After(time.Second):
		t.Fatal("session authentication did not start")
	}
}

func (authenticator *testAuthenticator) setPanicSession(value bool) {
	authenticator.mu.Lock()
	authenticator.panicSession = value
	authenticator.mu.Unlock()
}

func mustNewAPI(t *testing.T, application *appkit.Application, options APIOptions) http.Handler {
	t.Helper()
	if options.ResolveScope == nil {
		options.ResolveScope = fixedScope(scope.Must("tenant", "http-test"))
	}
	options.EnablePasswordLogin = true
	handler, err := NewAPI(application, options)
	if err != nil {
		t.Fatalf("NewAPI() error = %v", err)
	}
	return handler
}

func fixedScope(executionScope scope.Execution) ScopeResolver {
	return func(*http.Request, identity.Actor) (scope.Execution, error) { return executionScope, nil }
}

func loginForTest(t *testing.T, handler http.Handler) (*http.Cookie, string) {
	t.Helper()
	response := performRequest(handler, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"secret"}`, "application/json", "application/json", nil, nil, false)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) != 1 {
		t.Fatalf("login = %d %s", response.Code, response.Body.String())
	}
	var body loginResponse
	decodeResponse(t, response, &body)
	return response.Result().Cookies()[0], body.CSRFToken
}

func performRequest(handler http.Handler, method, path, body, contentType, accept string, cookies []*http.Cookie, headers map[string]string, tlsRequest bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body == "" {
		request.Body = http.NoBody
		request.ContentLength = 0
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	if tlsRequest {
		request.TLS = &tls.ConnectionState{}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
}

func assertSecurityHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	for name, wanted := range map[string]string{
		"Content-Type":           "application/json; charset=utf-8",
		"Cache-Control":          "no-store",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := response.Header().Get(name); got != wanted {
			t.Errorf("%s = %q, want %q", name, got, wanted)
		}
	}
	if !requestIDPattern.MatchString(response.Header().Get("X-Request-ID")) {
		t.Errorf("invalid response request ID %q", response.Header().Get("X-Request-ID"))
	}
}

func TestHTTPTestApplicationDoesNotDependOnConcreteModuleName(t *testing.T) {
	application := newHTTPTestApplication(t, true)
	catalog := application.app.Catalog()
	if len(catalog) != 2 || catalog[0].ModuleID != "unrelated-module-name" {
		t.Fatalf("test application catalog = %#v", catalog)
	}
	if reflect.TypeOf(application.app).Kind() != reflect.Pointer {
		t.Fatal("unexpected Application representation")
	}
}
