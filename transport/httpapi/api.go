package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/safeerr"
	"github.com/iiwish/modary/scope"
)

const (
	// DefaultCookieName is the host-only session cookie used by NewAPI.
	DefaultCookieName = "modary_session"
	// DefaultMaxBodyBytes bounds an API request body while leaving room for one
	// complete Action JSON document and its protocol envelope.
	DefaultMaxBodyBytes = int64(2 << 20)
	// MaximumBodyBytes is the largest body limit accepted by NewAPI.
	MaximumBodyBytes = int64(16 << 20)
	// DefaultTimeout bounds each API request when no timeout is supplied.
	DefaultTimeout = 30 * time.Second
)

var (
	cookieNamePattern = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]{1,128}$")
	requestIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	requestSequence   atomic.Uint64
)

// APIOptions controls the protocol boundary. Zero values select secure,
// bounded defaults.
type APIOptions struct {
	CookieName string
	// EnablePasswordLogin explicitly contributes /api/auth/login. Applications
	// using OIDC leave it false and mount the OIDC contribution instead.
	EnablePasswordLogin bool
	// ResolveScope derives the product execution boundary independently from
	// principal identity. It is required for governed Action requests.
	ResolveScope ScopeResolver
	// AllowInsecureCookie disables the Secure attribute for an explicitly
	// HTTP-only development environment. Production applications should retain
	// the secure default.
	AllowInsecureCookie bool
	MaxBodyBytes        int64
	Timeout             time.Duration
}

// ScopeResolver derives a validated execution scope for one authenticated
// request. Implementations should use bounded trusted routing context rather
// than identity claims that have not been mapped by consumer policy.
type ScopeResolver func(*http.Request, identity.Actor) (scope.Execution, error)

type apiServer struct {
	passwords    identity.PasswordAuthenticator
	sessions     identity.SessionManager
	runtime      appkit.Runtime
	catalog      []action.CatalogEntry
	byActionID   map[string]action.CatalogEntry
	cookieName   string
	secureCookie bool
	maxBodyBytes int64
	timeout      time.Duration
	resolveScope ScopeResolver
}

type sessionContextKey struct{}

type authenticatedSession struct {
	session identity.Session
	token   string
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Actor     identity.Actor `json:"actor"`
	CSRFToken string         `json:"csrf_token"`
	ExpiresAt time.Time      `json:"expires_at"`
	RequestID string         `json:"request_id"`
}

type previewRequest struct {
	Input json.RawMessage `json:"input"`
}

type executeRequest struct {
	Input          json.RawMessage `json:"input"`
	PlanHash       string          `json:"plan_hash,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

// NewAPI builds the session-authenticated governed Action API. The returned
// handler owns only /api routes and must be mounted explicitly by the consumer.
func NewAPI(application *appkit.Application, options APIOptions) (http.Handler, error) {
	if application == nil {
		return nil, fmt.Errorf("http API application is required")
	}
	if !application.Ready() {
		return nil, fmt.Errorf("http API application: %w", appkit.ErrApplicationUnavailable)
	}
	options, err := normalizeAPIOptions(options)
	if err != nil {
		return nil, err
	}
	sessions, err := application.Sessions()
	if err != nil {
		return nil, fmt.Errorf("http API sessions: %w", err)
	}
	if isTypedNil(sessions) {
		return nil, fmt.Errorf("http API sessions are required")
	}
	var passwords identity.PasswordAuthenticator
	if options.EnablePasswordLogin {
		passwords, err = application.Passwords()
		if err != nil {
			return nil, fmt.Errorf("http API passwords: %w", err)
		}
		if isTypedNil(passwords) {
			return nil, fmt.Errorf("http API passwords are required")
		}
	}
	runtime := application.Runtime()
	if isTypedNil(runtime) {
		return nil, fmt.Errorf("http API runtime is required")
	}
	catalog := make([]action.CatalogEntry, 0)
	byActionID := make(map[string]action.CatalogEntry)
	for _, entry := range application.Catalog() {
		if !slices.Contains(entry.Descriptor.Channels, action.ChannelHTTP) {
			continue
		}
		catalog = append(catalog, entry)
		byActionID[entry.Descriptor.ID] = entry
	}
	server := &apiServer{
		passwords: passwords, sessions: sessions, runtime: runtime, catalog: catalog, byActionID: byActionID,
		cookieName: options.CookieName, secureCookie: !options.AllowInsecureCookie,
		maxBodyBytes: options.MaxBodyBytes, timeout: options.Timeout, resolveScope: options.ResolveScope,
	}
	if !application.Ready() {
		return nil, fmt.Errorf("http API application stopped during construction: %w", appkit.ErrApplicationUnavailable)
	}
	return server, nil
}

func normalizeAPIOptions(options APIOptions) (APIOptions, error) {
	if options.ResolveScope == nil {
		return APIOptions{}, fmt.Errorf("http API scope resolver is required")
	}
	if options.CookieName == "" {
		options.CookieName = DefaultCookieName
	}
	if !cookieNamePattern.MatchString(options.CookieName) {
		return APIOptions{}, fmt.Errorf("http API cookie name %q is invalid", options.CookieName)
	}
	if options.MaxBodyBytes < 0 {
		return APIOptions{}, fmt.Errorf("http API maximum body bytes cannot be negative")
	}
	if options.MaxBodyBytes == 0 {
		options.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if options.MaxBodyBytes > MaximumBodyBytes {
		return APIOptions{}, fmt.Errorf("http API maximum body bytes cannot exceed %d", MaximumBodyBytes)
	}
	if options.Timeout < 0 {
		return APIOptions{}, fmt.Errorf("http API timeout cannot be negative")
	}
	if options.Timeout == 0 {
		options.Timeout = DefaultTimeout
	}
	return options, nil
}

// ServeHTTP applies the API timeout, response security headers, and panic
// containment before dispatching a request.
func (server *apiServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	tracked := &trackedResponseWriter{ResponseWriter: writer}
	writer = tracked
	requestID := ""
	returned := false
	defer func() {
		if returned {
			return
		}
		_ = recover()
		containResponsePanic(tracked, func() {
			writePublicError(writer, http.StatusInternalServerError, requestID, action.CodeInternal, "internal server error")
		})
	}()
	requestID = prepareResponse(writer, request)
	ctx, cancel := context.WithTimeout(request.Context(), server.timeout)
	defer cancel()
	request = request.WithContext(ctx)
	releaseBody := bindRequestBody(ctx, request)
	defer releaseBody()
	server.serve(writer, request, requestID)
	returned = true
}

func (server *apiServer) serve(writer http.ResponseWriter, request *http.Request, requestID string) {
	path := request.URL.Path
	switch path {
	case "/api/auth/login":
		if isTypedNil(server.passwords) {
			writePublicError(writer, http.StatusNotFound, requestID, action.CodeActionNotFound, "API route was not found")
			return
		}
		server.requireMethod(writer, request, requestID, http.MethodPost, server.login)
	case "/api/auth/session":
		server.requireMethod(writer, request, requestID, http.MethodGet, server.authenticated(server.session))
	case "/api/auth/logout":
		server.requireMethod(writer, request, requestID, http.MethodPost, server.authenticated(server.csrf(server.logout)))
	case "/api/actions":
		server.requireMethod(writer, request, requestID, http.MethodGet, server.authenticated(server.listActions))
	default:
		actionID, operation, ok := parseActionPath(path)
		if !ok {
			writePublicError(writer, http.StatusNotFound, requestID, action.CodeActionNotFound, "API route was not found")
			return
		}
		switch operation {
		case "schema":
			server.requireMethod(writer, request, requestID, http.MethodGet, server.authenticated(func(w http.ResponseWriter, r *http.Request, id string) {
				server.actionSchema(w, r, id, actionID)
			}))
		case "preview":
			server.requireMethod(writer, request, requestID, http.MethodPost, server.authenticated(server.csrf(func(w http.ResponseWriter, r *http.Request, id string) {
				server.preview(w, r, id, actionID)
			})))
		case "execute":
			server.requireMethod(writer, request, requestID, http.MethodPost, server.authenticated(server.csrf(func(w http.ResponseWriter, r *http.Request, id string) {
				server.execute(w, r, id, actionID)
			})))
		}
	}
}

type apiHandler func(http.ResponseWriter, *http.Request, string)

func (server *apiServer) requireMethod(writer http.ResponseWriter, request *http.Request, requestID, method string, next apiHandler) {
	if request.Method != method {
		writer.Header().Set("Allow", method)
		writePublicError(writer, http.StatusMethodNotAllowed, requestID, action.CodeValidationFailed, "method is not allowed")
		return
	}
	if !acceptsJSON(request) {
		writePublicError(writer, http.StatusNotAcceptable, requestID, action.CodeValidationFailed, "response must be accepted as application/json")
		return
	}
	if request.URL.RawQuery != "" {
		writePublicError(writer, http.StatusBadRequest, requestID, action.CodeValidationFailed, "query parameters are not allowed")
		return
	}
	if method == http.MethodGet {
		if !requestHasNoBody(request) {
			writePublicError(writer, http.StatusBadRequest, requestID, action.CodeValidationFailed, "request body is not allowed")
			return
		}
	} else if !hasJSONContentType(request) {
		writePublicError(writer, http.StatusUnsupportedMediaType, requestID, action.CodeValidationFailed, "Content-Type must be application/json")
		return
	}
	next(writer, request, requestID)
}

func (server *apiServer) authenticated(next apiHandler) apiHandler {
	return func(writer http.ResponseWriter, request *http.Request, requestID string) {
		token, ok := exactlyOneCookie(request, server.cookieName)
		if !ok {
			writePublicError(writer, http.StatusUnauthorized, requestID, action.CodeAuthzDenied, "authentication is required")
			return
		}
		session, err := server.sessions.ResolveSession(request.Context(), token)
		if err != nil {
			if writeContextError(writer, requestID, request.Context(), err) {
				return
			}
			if !isTypedNil(err) && safeerr.Is(err, identity.ErrSessionInvalid) {
				server.clearCookie(writer, request)
				writePublicError(writer, http.StatusUnauthorized, requestID, action.CodeAuthzDenied, "session is invalid or expired")
				return
			}
			writePublicError(writer, http.StatusInternalServerError, requestID, action.CodeInternal, "internal server error")
			return
		}
		if validateSessionContract(session, false) != nil {
			writePublicError(writer, http.StatusInternalServerError, requestID, action.CodeInternal, "internal server error")
			return
		}
		if sessionExpired(session) {
			server.clearCookie(writer, request)
			writePublicError(writer, http.StatusUnauthorized, requestID, action.CodeAuthzDenied, "session is invalid or expired")
			return
		}
		authenticated := authenticatedSession{session: session, token: token}
		ctx := context.WithValue(request.Context(), sessionContextKey{}, authenticated)
		next(writer, request.WithContext(ctx), requestID)
	}
}

func (server *apiServer) csrf(next apiHandler) apiHandler {
	return func(writer http.ResponseWriter, request *http.Request, requestID string) {
		authenticated, ok := request.Context().Value(sessionContextKey{}).(authenticatedSession)
		values := request.Header.Values("X-CSRF-Token")
		if !ok || len(values) != 1 || !constantTimeTokenEqual(values[0], authenticated.session.CSRFToken) {
			writePublicError(writer, http.StatusForbidden, requestID, action.CodeAuthzDenied, "valid CSRF token is required")
			return
		}
		next(writer, request, requestID)
	}
}

func (server *apiServer) login(writer http.ResponseWriter, request *http.Request, requestID string) {
	var input loginRequest
	if err := decodeRequestJSON(writer, request, server.maxBodyBytes, &input); err != nil {
		server.writeBodyError(writer, requestID, request.Context(), err, "invalid login request")
		return
	}
	if err := validateCredential("username", input.Username, 256); err != nil || input.Password == "" || len(input.Password) > 4096 {
		writePublicError(writer, http.StatusBadRequest, requestID, action.CodeValidationFailed, "invalid login request")
		return
	}
	authentication, err := server.passwords.AuthenticatePassword(request.Context(), input.Username, input.Password)
	if err != nil {
		if writeContextError(writer, requestID, request.Context(), err) {
			return
		}
		if !isTypedNil(err) && safeerr.Is(err, identity.ErrAuthenticationFailed) {
			writePublicError(writer, http.StatusUnauthorized, requestID, action.CodeAuthzDenied, "invalid username or password")
			return
		}
		writePublicError(writer, http.StatusInternalServerError, requestID, action.CodeInternal, "internal server error")
		return
	}
	session, err := server.sessions.CreateSession(request.Context(), authentication)
	if err != nil {
		if writeContextError(writer, requestID, request.Context(), err) {
			return
		}
		writePublicError(writer, http.StatusInternalServerError, requestID, action.CodeInternal, "internal server error")
		return
	}
	if err := validateSessionContract(session, true); err != nil || sessionExpired(session) {
		writePublicError(writer, http.StatusInternalServerError, requestID, action.CodeInternal, "internal server error")
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: server.cookieName, Value: session.Token, Path: "/", Expires: session.ExpiresAt,
		HttpOnly: true, Secure: server.secureCookie, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(writer, http.StatusOK, loginResponse{
		Actor: session.Actor, CSRFToken: session.CSRFToken, ExpiresAt: session.ExpiresAt, RequestID: requestID,
	})
}

func (server *apiServer) session(writer http.ResponseWriter, request *http.Request, requestID string) {
	authenticated := request.Context().Value(sessionContextKey{}).(authenticatedSession)
	writeJSON(writer, http.StatusOK, loginResponse{
		Actor: authenticated.session.Actor, CSRFToken: authenticated.session.CSRFToken,
		ExpiresAt: authenticated.session.ExpiresAt, RequestID: requestID,
	})
}

func (server *apiServer) logout(writer http.ResponseWriter, request *http.Request, requestID string) {
	var input struct{}
	if err := decodeRequestJSON(writer, request, server.maxBodyBytes, &input); err != nil {
		server.writeBodyError(writer, requestID, request.Context(), err, "invalid logout request")
		return
	}
	authenticated := request.Context().Value(sessionContextKey{}).(authenticatedSession)
	if err := server.sessions.RevokeSession(request.Context(), authenticated.token); err != nil {
		if writeContextError(writer, requestID, request.Context(), err) {
			return
		}
		writePublicError(writer, http.StatusInternalServerError, requestID, action.CodeInternal, "internal server error")
		return
	}
	server.clearCookie(writer, request)
	writer.WriteHeader(http.StatusNoContent)
}

func (server *apiServer) listActions(writer http.ResponseWriter, _ *http.Request, requestID string) {
	writeJSON(writer, http.StatusOK, struct {
		Actions   []action.CatalogEntry `json:"actions"`
		RequestID string                `json:"request_id"`
	}{Actions: server.catalog, RequestID: requestID})
}

func (server *apiServer) actionSchema(writer http.ResponseWriter, _ *http.Request, requestID, actionID string) {
	entry, ok := server.byActionID[actionID]
	if !ok {
		writePublicError(writer, http.StatusNotFound, requestID, action.CodeActionNotFound, "action is not registered for HTTP")
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		Action    action.CatalogEntry `json:"action"`
		RequestID string              `json:"request_id"`
	}{Action: entry, RequestID: requestID})
}

func (server *apiServer) preview(writer http.ResponseWriter, request *http.Request, requestID, actionID string) {
	if !server.isHTTPAction(actionID) {
		writePublicError(writer, http.StatusNotFound, requestID, action.CodeActionNotFound, "action is not registered for HTTP")
		return
	}
	var call previewRequest
	if err := decodeRequestJSON(writer, request, server.maxBodyBytes, &call); err != nil {
		server.writeBodyError(writer, requestID, request.Context(), err, "invalid Action preview request")
		return
	}
	if len(call.Input) == 0 {
		writePublicError(writer, http.StatusBadRequest, requestID, action.CodeValidationFailed, "Action preview input is required")
		return
	}
	authenticated := request.Context().Value(sessionContextKey{}).(authenticatedSession)
	executionScope, err := server.resolveExecutionScope(request, authenticated.session.Actor)
	if err != nil {
		writePublicError(writer, http.StatusBadRequest, requestID, action.CodeValidationFailed, "invalid execution scope")
		return
	}
	preview, err := server.runtime.Preview(request.Context(), action.Request{
		RequestID: requestID, Actor: authenticated.session.Actor, Channel: action.ChannelHTTP, ActionID: actionID,
		Scope: executionScope, Input: call.Input,
	})
	if err != nil {
		writeActionError(writer, requestID, request.Context(), err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		Preview   action.Preview `json:"preview"`
		RequestID string         `json:"request_id"`
	}{Preview: preview, RequestID: requestID})
}

func (server *apiServer) execute(writer http.ResponseWriter, request *http.Request, requestID, actionID string) {
	if !server.isHTTPAction(actionID) {
		writePublicError(writer, http.StatusNotFound, requestID, action.CodeActionNotFound, "action is not registered for HTTP")
		return
	}
	var call executeRequest
	if err := decodeRequestJSON(writer, request, server.maxBodyBytes, &call); err != nil {
		server.writeBodyError(writer, requestID, request.Context(), err, "invalid Action execution request")
		return
	}
	if len(call.Input) == 0 {
		writePublicError(writer, http.StatusBadRequest, requestID, action.CodeValidationFailed, "Action execution input is required")
		return
	}
	authenticated := request.Context().Value(sessionContextKey{}).(authenticatedSession)
	executionScope, err := server.resolveExecutionScope(request, authenticated.session.Actor)
	if err != nil {
		writePublicError(writer, http.StatusBadRequest, requestID, action.CodeValidationFailed, "invalid execution scope")
		return
	}
	result, err := server.runtime.Execute(request.Context(), action.Request{
		RequestID: requestID, Actor: authenticated.session.Actor, Channel: action.ChannelHTTP, ActionID: actionID,
		Scope: executionScope, Input: call.Input,
		PlanHash: call.PlanHash, IdempotencyKey: call.IdempotencyKey,
	})
	if err != nil {
		writeActionError(writer, requestID, request.Context(), err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		Result     json.RawMessage   `json:"result"`
		Summary    string            `json:"summary,omitempty"`
		References []audit.Reference `json:"references,omitempty"`
		RequestID  string            `json:"request_id"`
	}{Result: result.Data, Summary: result.Summary, References: result.References, RequestID: requestID})
}

func (server *apiServer) resolveExecutionScope(request *http.Request, actor identity.Actor) (scope.Execution, error) {
	if server == nil || server.resolveScope == nil {
		return scope.Execution{}, fmt.Errorf("scope resolver is unavailable")
	}
	executionScope, err := server.resolveScope(request, actor)
	if err != nil {
		return scope.Execution{}, err
	}
	if err := executionScope.Validate(); err != nil {
		return scope.Execution{}, err
	}
	return executionScope, nil
}

func (server *apiServer) writeBodyError(writer http.ResponseWriter, requestID string, ctx context.Context, err error, message string) {
	if writeContextError(writer, requestID, ctx, err) {
		return
	}
	if safeerr.Is(err, errBodyTooLarge) {
		writePublicError(writer, http.StatusRequestEntityTooLarge, requestID, action.CodeLimitExceeded, "request body exceeds the configured limit")
		return
	}
	writePublicError(writer, http.StatusBadRequest, requestID, action.CodeValidationFailed, message)
}

func (server *apiServer) clearCookie(writer http.ResponseWriter, _ *http.Request) {
	http.SetCookie(writer, &http.Cookie{
		Name: server.cookieName, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0).UTC(),
		HttpOnly: true, Secure: server.secureCookie, SameSite: http.SameSiteStrictMode,
	})
}

func (server *apiServer) isHTTPAction(actionID string) bool {
	_, ok := server.byActionID[actionID]
	return ok
}

func writeContextError(writer http.ResponseWriter, requestID string, ctx context.Context, err error) bool {
	status, message, ok := classifyContextFailure(ctx, err)
	if !ok {
		return false
	}
	writePublicError(writer, status, requestID, action.CodeUnavailable, message)
	return true
}

func parseActionPath(path string) (string, string, bool) {
	const prefix = "/api/actions/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	remainder := strings.TrimPrefix(path, prefix)
	for _, operation := range []string{"schema", "preview", "execute"} {
		suffix := "/" + operation
		if actionID := strings.TrimSuffix(remainder, suffix); actionID != remainder && actionID != "" {
			return actionID, operation, true
		}
	}
	return "", "", false
}

func exactlyOneCookie(request *http.Request, name string) (string, bool) {
	value := ""
	found := 0
	for _, cookie := range request.Cookies() {
		if cookie.Name == name {
			found++
			value = cookie.Value
		}
	}
	if found != 1 || !validSessionToken(value) {
		return "", false
	}
	return value, true
}

func validateSessionContract(session identity.Session, requireToken bool) error {
	if requireToken && !validSessionToken(session.Token) {
		return errors.New("invalid session token")
	}
	if session.CSRFToken == "" || len(session.CSRFToken) > 4096 || !utf8.ValidString(session.CSRFToken) || strings.ContainsFunc(session.CSRFToken, unicode.IsControl) {
		return errors.New("invalid CSRF token")
	}
	if err := identity.ValidateActor(session.Actor); err != nil {
		return err
	}
	return nil
}

func sessionExpired(session identity.Session) bool {
	return !session.ExpiresAt.After(time.Now())
}

func validSessionToken(value string) bool {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
		return false
	}
	return (&http.Cookie{Name: "session", Value: value}).Valid() == nil
}

func constantTimeTokenEqual(candidate, expected string) bool {
	if candidate == "" || len(candidate) > 4096 {
		return false
	}
	candidateHash := sha256.Sum256([]byte(candidate))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(candidateHash[:], expectedHash[:]) == 1
}

func validateCredential(name, value string, maxRunes int) error {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) > maxRunes || strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s is invalid", name)
	}
	return nil
}

func isTypedNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func prepareResponse(writer http.ResponseWriter, request *http.Request) string {
	values := request.Header.Values("X-Request-ID")
	requestID := ""
	if len(values) == 1 && requestIDPattern.MatchString(values[0]) {
		requestID = values[0]
	} else {
		requestID = newRequestID()
	}
	writer.Header().Set("X-Request-ID", requestID)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Vary", "Accept")
	return requestID
}

func newRequestID() string {
	var data [12]byte
	if _, err := rand.Read(data[:]); err == nil {
		return "req_" + hex.EncodeToString(data[:])
	}
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), requestSequence.Add(1))
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writeJSONMethod(writer, http.MethodGet, status, value)
}

func writeJSONMethod(writer http.ResponseWriter, method string, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		requestID := writer.Header().Get("X-Request-ID")
		data, _ = json.Marshal(errorEnvelope{
			Error: publicError{
				Code:      action.CodeInternal,
				Kind:      action.ErrorKindInternal,
				Message:   "internal server error",
				RequestID: requestID,
			},
			RequestID: requestID,
		})
	}
	data = append(data, '\n')
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Content-Length", fmt.Sprint(len(data)))
	writer.WriteHeader(status)
	if method != http.MethodHead && status != http.StatusNoContent {
		_, _ = writer.Write(data)
	}
}
