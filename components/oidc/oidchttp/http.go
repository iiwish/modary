// Package oidchttp provides the official, explicitly selected OIDC browser
// redirect contribution. The provider adapter remains independent of appkit
// and HTTP composition.
package oidchttp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/httpkit"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/transport/sessionhttp"
)

const DefaultFlowCookieName = "modary_oidc_flow"

var cookieNamePattern = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]{1,128}$")

// HTTPOptions controls the official redirect transport. SuccessPath must be a
// local absolute path. AllowInsecureCookie is only for an explicitly HTTP-only
// local environment.
type HTTPOptions struct {
	FlowCookieName      string
	SuccessPath         string
	AllowInsecureCookie bool
	Timeout             time.Duration
}

// Contribution declares the exact OIDC redirect routes and their capabilities.
func Contribution(options HTTPOptions) (httpkit.Contribution, error) {
	normalized, err := normalizeHTTPOptions(options)
	if err != nil {
		return httpkit.Contribution{}, err
	}
	return httpkit.Contribution{
		ID:       "oidc-login",
		Requires: []module.Capability{module.CapabilityBrowserAuthentication, module.CapabilitySessions},
		Routes: []httpkit.RouteSpec{
			{Method: http.MethodGet, Path: "/api/auth/oidc/login"},
			{Method: http.MethodGet, Path: "/api/auth/oidc/callback"},
		},
		Build: func(_ context.Context, application *appkit.Application) ([]httpkit.Route, error) {
			return buildHTTP(application, normalized)
		},
	}, nil
}

func buildHTTP(application *appkit.Application, options HTTPOptions) ([]httpkit.Route, error) {
	authenticator, err := application.BrowserAuthentication()
	if err != nil {
		return nil, fmt.Errorf("OIDC HTTP authenticator: %w", err)
	}
	sessions, err := sessionhttp.New(application, sessionhttp.Options{AllowInsecureCookie: options.AllowInsecureCookie})
	if err != nil {
		return nil, err
	}
	handler := &redirectHandler{authenticator: authenticator, sessions: sessions, options: options}
	return []httpkit.Route{
		{Method: http.MethodGet, Path: "/api/auth/oidc/login", Handler: http.HandlerFunc(handler.login)},
		{Method: http.MethodGet, Path: "/api/auth/oidc/callback", Handler: http.HandlerFunc(handler.callback)},
	}, nil
}

type redirectHandler struct {
	authenticator identity.BrowserAuthenticator
	sessions      *sessionhttp.API
	options       HTTPOptions
}

func (handler *redirectHandler) login(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		writePublicError(writer, http.StatusBadRequest, "VALIDATION_FAILED", "登录请求无效")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.options.Timeout)
	defer cancel()
	flow, err := handler.authenticator.Begin(ctx)
	if err != nil || flow.State == "" || flow.AuthorizationURL == "" || !flow.ExpiresAt.After(time.Now()) {
		writePublicError(writer, http.StatusServiceUnavailable, "AUTHENTICATION_UNAVAILABLE", "暂时无法开始登录")
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: handler.options.FlowCookieName, Value: flow.State, Path: "/api/auth/oidc/callback",
		Expires: flow.ExpiresAt, MaxAge: int(time.Until(flow.ExpiresAt).Seconds()), HttpOnly: true,
		Secure: !handler.options.AllowInsecureCookie, SameSite: http.SameSiteLaxMode,
	})
	writer.Header().Set("Cache-Control", "no-store")
	http.Redirect(writer, request, flow.AuthorizationURL, http.StatusSeeOther)
}

func (handler *redirectHandler) callback(writer http.ResponseWriter, request *http.Request) {
	flowCookie, cookieErr := request.Cookie(handler.options.FlowCookieName)
	handler.clearFlowCookie(writer)
	if len(request.URL.RawQuery) > 16384 {
		writePublicError(writer, http.StatusBadRequest, "VALIDATION_FAILED", "登录回调无效")
		return
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil || len(values) != 2 || len(values["state"]) != 1 || len(values["code"]) != 1 {
		writePublicError(writer, http.StatusBadRequest, "VALIDATION_FAILED", "登录回调无效")
		return
	}
	state, code := values.Get("state"), values.Get("code")
	if cookieErr != nil || len(state) == 0 || len(state) > 256 || len(code) == 0 || len(code) > 8192 ||
		len(flowCookie.Value) != len(state) || subtle.ConstantTimeCompare([]byte(flowCookie.Value), []byte(state)) != 1 {
		writePublicError(writer, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "登录已失效，请重试")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.options.Timeout)
	defer cancel()
	authentication, err := handler.authenticator.Complete(ctx, identity.BrowserCallback{State: state, Code: code})
	if err != nil {
		writePublicError(writer, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "登录失败，请重试")
		return
	}
	if _, err := handler.sessions.Establish(ctx, writer, authentication); err != nil {
		writePublicError(writer, http.StatusServiceUnavailable, "AUTHENTICATION_UNAVAILABLE", "暂时无法建立会话")
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	http.Redirect(writer, request, handler.options.SuccessPath, http.StatusSeeOther)
}

func (handler *redirectHandler) clearFlowCookie(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{
		Name: handler.options.FlowCookieName, Value: "", Path: "/api/auth/oidc/callback",
		MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true,
		Secure: !handler.options.AllowInsecureCookie, SameSite: http.SameSiteLaxMode,
	})
}

func normalizeHTTPOptions(options HTTPOptions) (HTTPOptions, error) {
	if options.FlowCookieName == "" {
		options.FlowCookieName = DefaultFlowCookieName
	}
	if !cookieNamePattern.MatchString(options.FlowCookieName) {
		return HTTPOptions{}, fmt.Errorf("OIDC flow cookie name is invalid")
	}
	if options.SuccessPath == "" {
		options.SuccessPath = "/"
	}
	parsed, err := url.Parse(options.SuccessPath)
	if err != nil || !strings.HasPrefix(options.SuccessPath, "/") || strings.HasPrefix(options.SuccessPath, "//") || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return HTTPOptions{}, fmt.Errorf("OIDC success path must be a local absolute path without query or fragment")
	}
	if options.Timeout < 0 || options.Timeout > time.Minute {
		return HTTPOptions{}, fmt.Errorf("OIDC HTTP timeout must be between zero and one minute")
	}
	if options.Timeout == 0 {
		options.Timeout = 30 * time.Second
	}
	return options, nil
}

func writePublicError(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message}})
}
