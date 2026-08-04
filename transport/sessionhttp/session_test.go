package sessionhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/module"
)

func TestSessionAPIAndMiddleware(t *testing.T) {
	service := newTestAuthenticator()
	application := startTestApplication(t, service)
	api, err := New(application, Options{AllowInsecureCookie: true, EnablePasswordLogin: true})
	if err != nil {
		t.Fatal(err)
	}
	actorHandler, err := api.Authenticate(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := Actor(request.Context())
		if !ok {
			t.Error("actor missing from authenticated context")
		}
		writeJSON(writer, http.StatusOK, actor)
	}))
	if err != nil {
		t.Fatal(err)
	}
	mutationHandler, err := api.Mutate(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatal(err)
	}

	login := request(t, api, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"correct-password"}`, nil, "")
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	var session SessionResponse
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Actor.ID != "admin" || session.CSRFToken == "" || session.RequestID == "" {
		t.Fatalf("login response=%#v", session)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode || !cookies[0].HttpOnly {
		t.Fatalf("login cookie=%#v", cookies)
	}

	current := request(t, api, http.MethodGet, "/api/auth/session", "", cookies[0], "")
	if current.Code != http.StatusOK {
		t.Fatalf("session status=%d body=%s", current.Code, current.Body.String())
	}
	protected := request(t, actorHandler, http.MethodGet, "/api/records", "", cookies[0], "")
	if protected.Code != http.StatusOK || !strings.Contains(protected.Body.String(), `"id":"admin"`) {
		t.Fatalf("protected status=%d body=%s", protected.Code, protected.Body.String())
	}
	denied := request(t, mutationHandler, http.MethodPost, "/api/records", `{}`, cookies[0], "wrong")
	if denied.Code != http.StatusForbidden {
		t.Fatalf("CSRF denied status=%d body=%s", denied.Code, denied.Body.String())
	}
	allowed := request(t, mutationHandler, http.MethodPost, "/api/records", `{}`, cookies[0], session.CSRFToken)
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("mutation status=%d body=%s", allowed.Code, allowed.Body.String())
	}
	logout := request(t, api, http.MethodPost, "/api/auth/logout", `{}`, cookies[0], session.CSRFToken)
	if logout.Code != http.StatusNoContent || service.active() != 0 {
		t.Fatalf("logout status=%d sessions=%d body=%s", logout.Code, service.active(), logout.Body.String())
	}
	invalid := request(t, actorHandler, http.MethodGet, "/api/records", "", cookies[0], "")
	if invalid.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestSessionAPIRejectsInvalidConstructionAndProtocol(t *testing.T) {
	if _, err := New(nil, Options{}); !errors.Is(err, appkit.ErrApplicationUnavailable) {
		t.Fatalf("New(nil) error=%v", err)
	}
	service := newTestAuthenticator()
	application := startTestApplication(t, service)
	api, err := New(application, Options{EnablePasswordLogin: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Authenticate(nil); err == nil {
		t.Fatal("nil protected handler accepted")
	}
	for _, test := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodGet, "/api/auth/login", "", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/auth/login", `{"username":"admin","password":"bad","extra":true}`, http.StatusBadRequest},
		{http.MethodGet, "/api/auth/unknown", "", http.StatusNotFound},
	} {
		response := request(t, api, test.method, test.path, test.body, nil, "")
		if response.Code != test.want || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func request(t *testing.T, handler http.Handler, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Accept", "application/json")
	if method != http.MethodGet {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func startTestApplication(t *testing.T, service *testAuthenticator) *appkit.Application {
	t.Helper()
	registration := module.Register(module.Manifest{SchemaVersion: module.SchemaVersion, ID: "test-identity", Version: "0.1.0",
		Type: module.ModuleTypeAdapter, Provides: []module.Capability{module.CapabilityIdentity, module.CapabilityPasswords, module.CapabilitySessions}}, func(_ context.Context, installation module.Scope) error {
		if err := module.Provide(installation, module.IdentityResolver(), identity.Resolver(service)); err != nil {
			return err
		}
		if err := module.Provide(installation, module.PasswordAuthenticator(), identity.PasswordAuthenticator(service)); err != nil {
			return err
		}
		return module.Provide(installation, module.SessionManager(), identity.SessionManager(service))
	})
	application, err := appkit.Start(context.Background(), appkit.Definition{Metadata: appkit.Metadata{ID: "session-test", Name: "Session Test", Version: "0.1.0"},
		Modules: []module.Registration{registration}}, appkit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	return application
}

type testAuthenticator struct {
	mu       sync.Mutex
	sessions map[string]identity.Session
}

func newTestAuthenticator() *testAuthenticator {
	return &testAuthenticator{sessions: make(map[string]identity.Session)}
}

func (service *testAuthenticator) ResolveByID(context.Context, string) (identity.Actor, error) {
	return identity.Actor{}, identity.ErrActorNotFound
}

func (service *testAuthenticator) AuthenticatePassword(_ context.Context, username, password string) (identity.Authentication, error) {
	if username != "admin" || password != "correct-password" {
		return identity.Authentication{}, identity.ErrAuthenticationFailed
	}
	return identity.Authentication{Actor: identity.Actor{ID: "admin", Type: "human", DisplayName: "Admin"}, Method: identity.AuthenticationMethodPassword, CredentialVersion: "version"}, nil
}

func (service *testAuthenticator) CreateSession(_ context.Context, authentication identity.Authentication) (identity.Session, error) {
	session := identity.Session{Token: "session-token", CSRFToken: "csrf-token", Actor: authentication.Actor, ExpiresAt: time.Now().Add(time.Hour)}
	service.mu.Lock()
	service.sessions[session.Token] = session
	service.mu.Unlock()
	return session, nil
}

func (service *testAuthenticator) RevokeSession(_ context.Context, token string) error {
	service.mu.Lock()
	delete(service.sessions, token)
	service.mu.Unlock()
	return nil
}

func (service *testAuthenticator) ResolveSession(_ context.Context, token string) (identity.Session, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	session, ok := service.sessions[token]
	if !ok {
		return identity.Session{}, identity.ErrSessionInvalid
	}
	return session, nil
}

func (service *testAuthenticator) active() int {
	service.mu.Lock()
	defer service.mu.Unlock()
	return len(service.sessions)
}
