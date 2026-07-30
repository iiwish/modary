package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"modary/core/action"
	"modary/core/identity"
	"modary/internal/app"
	"modary/internal/webui"
)

type Server struct {
	app *app.Application
}

func New(application *app.Application) http.Handler {
	server := &Server{app: application}
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(30 * time.Second))
	router.Get("/healthz", server.health)
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", server.login)
		api.Group(func(authenticated chi.Router) {
			authenticated.Use(server.requireSession)
			authenticated.Get("/auth/session", server.session)
			authenticated.Post("/auth/logout", server.requireCSRF(server.logout))
			authenticated.Get("/actions", server.listActions)
			authenticated.Get("/actions/{actionID}/schema", server.actionSchema)
			authenticated.Post("/actions/{actionID}/preview", server.requireCSRF(server.previewAction))
			authenticated.Post("/actions/{actionID}/execute", server.requireCSRF(server.executeAction))
			authenticated.Get("/audit", server.queryAudit)
		})
	})
	if application.Host.HasModule("agent-mcp") {
		router.Post("/mcp", server.mcp)
	}
	if application.Host.HasModule("console-react") {
		router.Handle("/*", spaHandler())
	}
	return router
}

type sessionContextKey struct{}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie("modary_session")
		if err != nil || cookie.Value == "" {
			writeAPIError(writer, http.StatusUnauthorized, action.NewError(action.CodeAuthzDenied, "authentication is required"))
			return
		}
		session, err := s.app.Identity.Session(request.Context(), cookie.Value)
		if err != nil {
			writeAPIError(writer, http.StatusUnauthorized, action.NewError(action.CodeAuthzDenied, "session is invalid or expired"))
			return
		}
		ctx := context.WithValue(request.Context(), sessionContextKey{}, session)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func (s *Server) requireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		session, ok := request.Context().Value(sessionContextKey{}).(identity.Session)
		if !ok || request.Header.Get("X-CSRF-Token") == "" || request.Header.Get("X-CSRF-Token") != session.CSRFToken {
			writeAPIError(writer, http.StatusForbidden, action.NewError(action.CodeAuthzDenied, "valid CSRF token is required"))
			return
		}
		next(writer, request)
	}
}

func (s *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"status": "ready", "modules": s.app.Host.StartedModules(), "startup_ms": s.app.StartupDuration.Milliseconds(),
	})
}

func (s *Server) login(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(request, &input); err != nil {
		writeAPIError(writer, http.StatusBadRequest, action.NewError(action.CodeValidationFailed, "invalid login request"))
		return
	}
	session, err := s.app.Identity.Login(request.Context(), input.Username, input.Password)
	if err != nil {
		writeAPIError(writer, http.StatusUnauthorized, action.NewError(action.CodeAuthzDenied, "invalid username or password"))
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: "modary_session", Value: session.Token, Path: "/", Expires: session.ExpiresAt,
		HttpOnly: true, Secure: request.TLS != nil || strings.EqualFold(request.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(writer, http.StatusOK, map[string]any{"actor": session.Actor, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
}

func (s *Server) logout(writer http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie("modary_session"); err == nil {
		_ = s.app.Identity.Logout(request.Context(), cookie.Value)
	}
	http.SetCookie(writer, &http.Cookie{Name: "modary_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) session(writer http.ResponseWriter, request *http.Request) {
	session := request.Context().Value(sessionContextKey{}).(identity.Session)
	writeJSON(writer, http.StatusOK, map[string]any{"actor": session.Actor, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
}

func (s *Server) listActions(writer http.ResponseWriter, _ *http.Request) {
	items := s.app.Registry.List()
	descriptors := make([]action.Descriptor, 0, len(items))
	for _, item := range items {
		descriptors = append(descriptors, item.Descriptor)
	}
	writeJSON(writer, http.StatusOK, map[string]any{"actions": descriptors})
}

func (s *Server) actionSchema(writer http.ResponseWriter, request *http.Request) {
	id := chi.URLParam(request, "actionID")
	registered, ok := s.app.Registry.Resolve(id)
	if !ok {
		writeAPIError(writer, http.StatusNotFound, action.NewError(action.CodeActionNotFound, "action is not registered"))
		return
	}
	writeJSON(writer, http.StatusOK, registered.Descriptor)
}

type actionCall struct {
	Input          json.RawMessage `json:"input"`
	PlanHash       string          `json:"plan_hash"`
	IdempotencyKey string          `json:"idempotency_key"`
}

func (s *Server) previewAction(writer http.ResponseWriter, request *http.Request) {
	call, actionRequest, ok := s.buildActionRequest(writer, request)
	if !ok {
		return
	}
	_ = call
	preview, err := s.app.Runtime.Preview(request.Context(), actionRequest)
	if err != nil {
		writeActionError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"preview": preview, "request_id": actionRequest.RequestID})
}

func (s *Server) executeAction(writer http.ResponseWriter, request *http.Request) {
	_, actionRequest, ok := s.buildActionRequest(writer, request)
	if !ok {
		return
	}
	result, err := s.app.Runtime.Execute(request.Context(), actionRequest)
	if err != nil {
		writeActionError(writer, err)
		return
	}
	var data any
	_ = json.Unmarshal(result.Data, &data)
	writeJSON(writer, http.StatusOK, map[string]any{"result": data, "summary": result.Summary, "request_id": actionRequest.RequestID})
}

func (s *Server) buildActionRequest(writer http.ResponseWriter, request *http.Request) (actionCall, action.Request, bool) {
	var call actionCall
	if err := decodeBody(request, &call); err != nil {
		writeAPIError(writer, http.StatusBadRequest, action.NewError(action.CodeValidationFailed, "invalid action request"))
		return actionCall{}, action.Request{}, false
	}
	session := request.Context().Value(sessionContextKey{}).(identity.Session)
	actionRequest := action.Request{
		RequestID: middleware.GetReqID(request.Context()), Actor: session.Actor, Channel: "http",
		ActionID: chi.URLParam(request, "actionID"), WorkspaceID: session.Actor.WorkspaceID,
		Input: call.Input, PlanHash: call.PlanHash, IdempotencyKey: call.IdempotencyKey,
	}
	return call, actionRequest, true
}

func (s *Server) queryAudit(writer http.ResponseWriter, request *http.Request) {
	session := request.Context().Value(sessionContextKey{}).(identity.Session)
	input, _ := json.Marshal(map[string]any{
		"action_id": request.URL.Query().Get("action_id"),
		"actor_id":  request.URL.Query().Get("actor_id"),
		"decision":  request.URL.Query().Get("decision"),
		"limit":     100,
	})
	result, err := s.app.Runtime.Execute(request.Context(), action.Request{
		RequestID: middleware.GetReqID(request.Context()), Actor: session.Actor, Channel: "http",
		ActionID: "audit.query", WorkspaceID: session.Actor.WorkspaceID, Input: input,
	})
	if err != nil {
		writeActionError(writer, err)
		return
	}
	var data any
	_ = json.Unmarshal(result.Data, &data)
	writeJSON(writer, http.StatusOK, data)
}

func decodeBody(request *http.Request, target any) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(nil, request.Body, 2<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeActionError(writer http.ResponseWriter, err error) {
	slog.Error("action request failed", "error", err)
	status := http.StatusInternalServerError
	var actionErr *action.Error
	if errors.As(err, &actionErr) {
		switch actionErr.Code {
		case action.CodeActionNotFound, action.CodePlanNotFound:
			status = http.StatusNotFound
		case action.CodeAuthzDenied:
			status = http.StatusForbidden
		case action.CodeValidationFailed:
			status = http.StatusBadRequest
		case action.CodePlanRequired, action.CodePlanStale, action.CodeLimitExceeded,
			action.CodePreconditionFailed, action.CodeIdempotencyConflict, action.CodeIdempotencyProgress:
			status = http.StatusConflict
		case action.CodeIdempotencyRequired:
			status = http.StatusBadRequest
		}
	}
	writeAPIError(writer, status, err)
}

func writeAPIError(writer http.ResponseWriter, status int, err error) {
	var actionErr *action.Error
	if !errors.As(err, &actionErr) {
		actionErr = action.NewError(action.CodeInternal, "internal server error")
	}
	writeJSON(writer, status, map[string]any{"error": actionErr})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func spaHandler() http.Handler {
	content, _ := fs.Sub(webui.Files, "dist")
	files := http.FileServer(http.FS(content))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			http.NotFound(writer, request)
			return
		}
		path := strings.TrimPrefix(request.URL.Path, "/")
		if path == "" {
			files.ServeHTTP(writer, request)
			return
		}
		if _, err := fs.Stat(content, path); err == nil {
			files.ServeHTTP(writer, request)
			return
		}
		index, err := fs.ReadFile(content, "index.html")
		if err != nil {
			http.Error(writer, "console is not built", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write(index)
	})
}
