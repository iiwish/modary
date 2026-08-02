package sessionhttp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/safeerr"
)

const (
	// DefaultCookieName is the host-only browser-session cookie name.
	DefaultCookieName = "modary_session"
	// DefaultMaxBodyBytes bounds login and logout JSON bodies.
	DefaultMaxBodyBytes = int64(64 << 10)
	// MaximumBodyBytes is the largest configurable session request body.
	MaximumBodyBytes = int64(1 << 20)
	// DefaultTimeout bounds each session endpoint or protected handler call.
	DefaultTimeout = 30 * time.Second
)

const (
	// CodeValidationFailed identifies malformed session HTTP input.
	CodeValidationFailed = "VALIDATION_FAILED"
	// CodeAuthenticationFailed identifies rejected login credentials.
	CodeAuthenticationFailed = "AUTHENTICATION_FAILED"
	// CodeAuthenticationNeeded identifies a missing or invalid session.
	CodeAuthenticationNeeded = "AUTHENTICATION_REQUIRED"
	// CodeCSRFFailed identifies a missing or invalid mutation token.
	CodeCSRFFailed = "CSRF_REQUIRED"
	// CodeUnavailable identifies cancellation or dependency unavailability.
	CodeUnavailable = "UNAVAILABLE"
	// CodeInternal identifies an unexpected server failure.
	CodeInternal = "INTERNAL"
	// CodeNotFound identifies an unknown session route.
	CodeNotFound = "NOT_FOUND"
)

var (
	cookieNamePattern = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]{1,128}$")
	requestIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	requestSequence   atomic.Uint64
)

// Options controls the bounded session protocol and cookie policy.
type Options struct {
	CookieName string
	// AllowInsecureCookie is intended only for an explicitly HTTP-only local
	// development environment. Production should retain secure cookies.
	AllowInsecureCookie bool
	MaxBodyBytes        int64
	Timeout             time.Duration
}

// API serves login, current-session, and logout routes and constructs
// lifecycle-safe session middleware for consumer-owned handlers.
type API struct {
	sessions     identity.Authenticator
	cookieName   string
	secureCookie bool
	maxBodyBytes int64
	timeout      time.Duration
}

type contextKey uint8

const (
	actorContextKey contextKey = iota
	sessionContextKey
	requestIDContextKey
)

type authenticatedSession struct {
	session identity.Session
	token   string
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// SessionResponse is the public login and current-session representation.
type SessionResponse struct {
	Actor     identity.Actor `json:"actor"`
	CSRFToken string         `json:"csrf_token"`
	ExpiresAt time.Time      `json:"expires_at"`
	RequestID string         `json:"request_id"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

// New assembles a session HTTP API from an application's optional session
// authenticator facade.
func New(application *appkit.Application, options Options) (*API, error) {
	if application == nil || !application.Ready() {
		return nil, fmt.Errorf("session HTTP application: %w", appkit.ErrApplicationUnavailable)
	}
	normalized, err := normalizeOptions(options)
	if err != nil {
		return nil, err
	}
	sessions, err := application.Sessions()
	if err != nil {
		return nil, fmt.Errorf("session HTTP authenticator: %w", err)
	}
	if typedNil(sessions) {
		return nil, fmt.Errorf("session HTTP authenticator is required")
	}
	api := &API{sessions: sessions, cookieName: normalized.CookieName,
		secureCookie: !normalized.AllowInsecureCookie, maxBodyBytes: normalized.MaxBodyBytes,
		timeout: normalized.Timeout}
	if !application.Ready() {
		return nil, fmt.Errorf("session HTTP application stopped during construction: %w", appkit.ErrApplicationUnavailable)
	}
	return api, nil
}

func normalizeOptions(options Options) (Options, error) {
	if options.CookieName == "" {
		options.CookieName = DefaultCookieName
	}
	if !cookieNamePattern.MatchString(options.CookieName) {
		return Options{}, fmt.Errorf("session HTTP cookie name %q is invalid", options.CookieName)
	}
	if options.MaxBodyBytes < 0 || options.MaxBodyBytes > MaximumBodyBytes {
		return Options{}, fmt.Errorf("session HTTP maximum body bytes must be between zero and %d", MaximumBodyBytes)
	}
	if options.MaxBodyBytes == 0 {
		options.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if options.Timeout < 0 {
		return Options{}, fmt.Errorf("session HTTP timeout cannot be negative")
	}
	if options.Timeout == 0 {
		options.Timeout = DefaultTimeout
	}
	return options, nil
}

// Actor returns the authenticated actor installed by API middleware.
func Actor(ctx context.Context) (identity.Actor, bool) {
	if ctx == nil {
		return identity.Actor{}, false
	}
	actor, ok := ctx.Value(actorContextKey).(identity.Actor)
	return actor, ok && identity.ValidateActor(actor) == nil
}

// RequestID returns the bounded request identifier installed by API handlers
// and middleware.
func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDContextKey).(string)
	return requestID
}

// Authenticate wraps a consumer handler with session authentication.
func (api *API) Authenticate(next http.Handler) (http.Handler, error) {
	if api == nil || typedNil(api.sessions) {
		return nil, fmt.Errorf("session HTTP API is unavailable")
	}
	if typedNil(next) {
		return nil, fmt.Errorf("session HTTP protected handler is required")
	}
	return api.protected(next, false), nil
}

// Mutate wraps a consumer mutation handler with session authentication and
// constant-time CSRF verification.
func (api *API) Mutate(next http.Handler) (http.Handler, error) {
	if api == nil || typedNil(api.sessions) {
		return nil, fmt.Errorf("session HTTP API is unavailable")
	}
	if typedNil(next) {
		return nil, fmt.Errorf("session HTTP mutation handler is required")
	}
	return api.protected(next, true), nil
}

func (api *API) protected(next http.Handler, mutation bool) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		api.boundary(writer, request, func(writer http.ResponseWriter, request *http.Request, requestID string) {
			authenticated, ok := api.authenticate(writer, request, requestID)
			if !ok {
				return
			}
			if mutation && !api.validCSRF(request, authenticated.session.CSRFToken) {
				writeError(writer, http.StatusForbidden, requestID, CodeCSRFFailed, "valid CSRF token is required")
				return
			}
			ctx := context.WithValue(request.Context(), actorContextKey, authenticated.session.Actor)
			ctx = context.WithValue(ctx, sessionContextKey, authenticated)
			next.ServeHTTP(writer, request.WithContext(ctx))
		})
	})
}

// ServeHTTP handles only /api/auth/login, /api/auth/session, and
// /api/auth/logout.
func (api *API) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	api.boundary(writer, request, api.serve)
}

func (api *API) boundary(writer http.ResponseWriter, request *http.Request, next func(http.ResponseWriter, *http.Request, string)) {
	if api == nil || request == nil || typedNil(api.sessions) {
		http.Error(writer, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	requestID := prepareResponse(writer, request)
	tracked := &responseTracker{ResponseWriter: writer}
	returned := false
	defer func() {
		if returned {
			return
		}
		_ = recover()
		if !tracked.wroteHeader {
			writeError(tracked, http.StatusInternalServerError, requestID, CodeInternal, "internal server error")
		}
	}()
	ctx, cancel := context.WithTimeout(request.Context(), api.timeout)
	defer cancel()
	ctx = context.WithValue(ctx, requestIDContextKey, requestID)
	next(tracked, request.WithContext(ctx), requestID)
	returned = true
}

func (api *API) serve(writer http.ResponseWriter, request *http.Request, requestID string) {
	switch request.URL.Path {
	case "/api/auth/login":
		if !validateMethod(writer, request, requestID, http.MethodPost) {
			return
		}
		api.login(writer, request, requestID)
	case "/api/auth/session":
		if !validateMethod(writer, request, requestID, http.MethodGet) {
			return
		}
		authenticated, ok := api.authenticate(writer, request, requestID)
		if ok {
			api.writeSession(writer, authenticated.session, requestID)
		}
	case "/api/auth/logout":
		if !validateMethod(writer, request, requestID, http.MethodPost) {
			return
		}
		authenticated, ok := api.authenticate(writer, request, requestID)
		if !ok {
			return
		}
		if !api.validCSRF(request, authenticated.session.CSRFToken) {
			writeError(writer, http.StatusForbidden, requestID, CodeCSRFFailed, "valid CSRF token is required")
			return
		}
		api.logout(writer, request, requestID, authenticated)
	default:
		writeError(writer, http.StatusNotFound, requestID, CodeNotFound, "session route was not found")
	}
}

func validateMethod(writer http.ResponseWriter, request *http.Request, requestID, method string) bool {
	if request.Method != method {
		writer.Header().Set("Allow", method)
		writeError(writer, http.StatusMethodNotAllowed, requestID, CodeValidationFailed, "method is not allowed")
		return false
	}
	if request.URL.RawQuery != "" {
		writeError(writer, http.StatusBadRequest, requestID, CodeValidationFailed, "query parameters are not allowed")
		return false
	}
	if !acceptsJSON(request) {
		writeError(writer, http.StatusNotAcceptable, requestID, CodeValidationFailed, "response must be accepted as application/json")
		return false
	}
	if method == http.MethodGet {
		if request.ContentLength != 0 || len(request.TransferEncoding) != 0 {
			writeError(writer, http.StatusBadRequest, requestID, CodeValidationFailed, "request body is not allowed")
			return false
		}
	} else if !hasJSONContentType(request) {
		writeError(writer, http.StatusUnsupportedMediaType, requestID, CodeValidationFailed, "Content-Type must be application/json")
		return false
	}
	return true
}

func (api *API) login(writer http.ResponseWriter, request *http.Request, requestID string) {
	var input loginRequest
	if err := decodeJSON(writer, request, api.maxBodyBytes, &input); err != nil ||
		validateCredential(input.Username, 256) != nil || input.Password == "" || len(input.Password) > 4096 {
		writeError(writer, http.StatusBadRequest, requestID, CodeValidationFailed, "invalid login request")
		return
	}
	session, err := api.sessions.Login(request.Context(), input.Username, input.Password)
	if err != nil {
		if errors.Is(request.Context().Err(), context.DeadlineExceeded) || safeerr.Is(err, context.DeadlineExceeded) {
			writeError(writer, http.StatusServiceUnavailable, requestID, CodeUnavailable, "request timed out")
			return
		}
		if safeerr.Is(err, identity.ErrAuthenticationFailed) {
			writeError(writer, http.StatusUnauthorized, requestID, CodeAuthenticationFailed, "invalid username or password")
			return
		}
		writeError(writer, http.StatusInternalServerError, requestID, CodeInternal, "internal server error")
		return
	}
	if validateSession(session, true) != nil || !session.ExpiresAt.After(time.Now()) {
		writeError(writer, http.StatusInternalServerError, requestID, CodeInternal, "internal server error")
		return
	}
	http.SetCookie(writer, &http.Cookie{Name: api.cookieName, Value: session.Token, Path: "/",
		Expires: session.ExpiresAt, HttpOnly: true, Secure: api.secureCookie, SameSite: http.SameSiteStrictMode})
	api.writeSession(writer, session, requestID)
}

func (api *API) authenticate(writer http.ResponseWriter, request *http.Request, requestID string) (authenticatedSession, bool) {
	token, ok := exactlyOneCookie(request, api.cookieName)
	if !ok {
		writeError(writer, http.StatusUnauthorized, requestID, CodeAuthenticationNeeded, "authentication is required")
		return authenticatedSession{}, false
	}
	session, err := api.sessions.Session(request.Context(), token)
	if err != nil {
		if safeerr.Is(err, identity.ErrSessionInvalid) {
			api.clearCookie(writer)
			writeError(writer, http.StatusUnauthorized, requestID, CodeAuthenticationNeeded, "session is invalid or expired")
			return authenticatedSession{}, false
		}
		writeError(writer, http.StatusInternalServerError, requestID, CodeInternal, "internal server error")
		return authenticatedSession{}, false
	}
	if validateSession(session, false) != nil || !session.ExpiresAt.After(time.Now()) {
		api.clearCookie(writer)
		writeError(writer, http.StatusUnauthorized, requestID, CodeAuthenticationNeeded, "session is invalid or expired")
		return authenticatedSession{}, false
	}
	return authenticatedSession{session: session, token: token}, true
}

func (api *API) logout(writer http.ResponseWriter, request *http.Request, requestID string, authenticated authenticatedSession) {
	var input struct{}
	if err := decodeJSON(writer, request, api.maxBodyBytes, &input); err != nil {
		writeError(writer, http.StatusBadRequest, requestID, CodeValidationFailed, "invalid logout request")
		return
	}
	if err := api.sessions.Logout(request.Context(), authenticated.token); err != nil {
		writeError(writer, http.StatusInternalServerError, requestID, CodeInternal, "internal server error")
		return
	}
	api.clearCookie(writer)
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) writeSession(writer http.ResponseWriter, session identity.Session, requestID string) {
	writeJSON(writer, http.StatusOK, SessionResponse{Actor: session.Actor, CSRFToken: session.CSRFToken,
		ExpiresAt: session.ExpiresAt, RequestID: requestID})
}

func (api *API) clearCookie(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{Name: api.cookieName, Value: "", Path: "/", MaxAge: -1,
		Expires: time.Unix(1, 0).UTC(), HttpOnly: true, Secure: api.secureCookie, SameSite: http.SameSiteStrictMode})
}

func (api *API) validCSRF(request *http.Request, expected string) bool {
	values := request.Header.Values("X-CSRF-Token")
	if len(values) != 1 || values[0] == "" || len(values[0]) > 4096 {
		return false
	}
	candidateHash := sha256.Sum256([]byte(values[0]))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(candidateHash[:], expectedHash[:]) == 1
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, limit int64, destination any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, limit)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON value")
	}
	return request.Body.Close()
}

func acceptsJSON(request *http.Request) bool {
	values := request.Header.Values("Accept")
	if len(values) == 0 {
		return true
	}
	items, ok := splitHTTPList(values)
	if !ok {
		return false
	}
	bestSpecificity, bestParameters, bestQuality := -1, -1, 0.0
	for _, item := range items {
		mediaType, parameters, err := mime.ParseMediaType(item)
		if err != nil {
			return false
		}
		quality := 1.0
		if raw, exists := parameters["q"]; exists {
			quality, err = strconv.ParseFloat(raw, 64)
			if err != nil || math.IsNaN(quality) || math.IsInf(quality, 0) || quality < 0 || quality > 1 {
				return false
			}
			delete(parameters, "q")
		}
		specificity := -1
		switch mediaType {
		case "application/json":
			specificity = 2
		case "application/*":
			specificity = 1
		case "*/*":
			specificity = 0
		}
		if specificity < 0 || !jsonParameters(parameters) {
			continue
		}
		parameterCount := len(parameters)
		if specificity > bestSpecificity || (specificity == bestSpecificity && parameterCount > bestParameters) ||
			(specificity == bestSpecificity && parameterCount == bestParameters && quality > bestQuality) {
			bestSpecificity, bestParameters, bestQuality = specificity, parameterCount, quality
		}
	}
	return bestSpecificity >= 0 && bestQuality > 0
}

func hasJSONContentType(request *http.Request) bool {
	values := request.Header.Values("Content-Type")
	if len(values) != 1 {
		return false
	}
	mediaType, parameters, err := mime.ParseMediaType(values[0])
	return err == nil && mediaType == "application/json" && jsonParameters(parameters)
}

func jsonParameters(parameters map[string]string) bool {
	for name, value := range parameters {
		if !strings.EqualFold(name, "charset") || !strings.EqualFold(value, "utf-8") {
			return false
		}
	}
	return true
}

func splitHTTPList(values []string) ([]string, bool) {
	joined := strings.Join(values, ",")
	items, start, quoted, escaped := make([]string, 0, strings.Count(joined, ",")+1), 0, false, false
	for index := range len(joined) {
		character := joined[index]
		if escaped {
			escaped = false
			continue
		}
		if quoted && character == '\\' {
			escaped = true
			continue
		}
		if character == '"' {
			quoted = !quoted
			continue
		}
		if character == ',' && !quoted {
			item := strings.TrimSpace(joined[start:index])
			if item == "" {
				return nil, false
			}
			items, start = append(items, item), index+1
		}
	}
	last := strings.TrimSpace(joined[start:])
	if quoted || escaped || last == "" {
		return nil, false
	}
	return append(items, last), true
}

func exactlyOneCookie(request *http.Request, name string) (string, bool) {
	value, count := "", 0
	for _, cookie := range request.Cookies() {
		if cookie.Name == name {
			value, count = cookie.Value, count+1
		}
	}
	return value, count == 1 && validToken(value)
}

func validToken(value string) bool {
	return value != "" && len(value) <= 4096 && utf8.ValidString(value) &&
		!strings.ContainsFunc(value, unicode.IsControl) && (&http.Cookie{Name: "session", Value: value}).Valid() == nil
}

func validateSession(session identity.Session, requireToken bool) error {
	if requireToken && !validToken(session.Token) {
		return errors.New("invalid session token")
	}
	if session.CSRFToken == "" || len(session.CSRFToken) > 4096 || !utf8.ValidString(session.CSRFToken) || strings.ContainsFunc(session.CSRFToken, unicode.IsControl) {
		return errors.New("invalid CSRF token")
	}
	return identity.ValidateActor(session.Actor)
}

func validateCredential(value string, maxRunes int) error {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
		utf8.RuneCountInString(value) > maxRunes || strings.ContainsFunc(value, unicode.IsControl) {
		return errors.New("credential is invalid")
	}
	return nil
}

func prepareResponse(writer http.ResponseWriter, request *http.Request) string {
	requestID := ""
	values := request.Header.Values("X-Request-ID")
	if len(values) == 1 && requestIDPattern.MatchString(values[0]) {
		requestID = values[0]
	} else {
		var data [12]byte
		if _, err := rand.Read(data[:]); err == nil {
			requestID = "req_" + hex.EncodeToString(data[:])
		} else {
			requestID = fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), requestSequence.Add(1))
		}
	}
	writer.Header().Set("X-Request-ID", requestID)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Vary", "Accept")
	return requestID
}

func writeError(writer http.ResponseWriter, status int, requestID, code, message string) {
	response := errorResponse{RequestID: requestID}
	response.Error.Code, response.Error.Message = code, message
	writeJSON(writer, status, response)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte(`{"error":{"code":"INTERNAL","message":"internal server error"}}`)
		status = http.StatusInternalServerError
	}
	data = append(data, '\n')
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Content-Length", fmt.Sprint(len(data)))
	writer.WriteHeader(status)
	if status != http.StatusNoContent {
		_, _ = writer.Write(data)
	}
}

func typedNil(value any) bool {
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

type responseTracker struct {
	http.ResponseWriter
	wroteHeader bool
}

func (writer *responseTracker) WriteHeader(status int) {
	writer.wroteHeader = true
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *responseTracker) Write(data []byte) (int, error) {
	writer.wroteHeader = true
	return writer.ResponseWriter.Write(data)
}
