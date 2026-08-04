package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/components/oidc/oidchttp"
	"github.com/iiwish/modary/httpkit"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/module"
)

func TestOIDCFlowVerifiesDiscoverySignatureAudienceNonceStateAndPKCE(t *testing.T) {
	provider := newTestProvider(t)
	application := startTestApplication(t, provider, []SubjectMapping{{Subject: "upstream-1", ActorID: "person-1", ActorType: "human"}})
	authenticator, err := application.BrowserAuthentication()
	if err != nil {
		t.Fatal(err)
	}
	flow, err := authenticator.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := url.Parse(flow.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	query := authorizationURL.Query()
	for name, expected := range map[string]string{
		"client_id": "client-1", "redirect_uri": "http://127.0.0.1/callback",
		"response_type": "code", "state": flow.State, "code_challenge_method": "S256",
	} {
		if query.Get(name) != expected {
			t.Fatalf("authorization %s = %q, want %q", name, query.Get(name), expected)
		}
	}
	if len(query["state"]) != 1 || len(query["nonce"]) != 1 || len(query["code_challenge"]) != 1 {
		t.Fatalf("authorization query is not singular: %v", query)
	}
	provider.allow("valid-code", tokenClaims{
		subject: "upstream-1", audience: "client-1", nonce: query.Get("nonce"),
	}, query.Get("code_challenge"))
	authentication, err := authenticator.Complete(context.Background(), identity.BrowserCallback{State: flow.State, Code: "valid-code"})
	if err != nil {
		t.Fatal(err)
	}
	if authentication.Method != identity.AuthenticationMethodOIDC || authentication.Actor.ID != "person-1" || authentication.Actor.Type != "human" {
		t.Fatalf("authentication = %#v", authentication)
	}
	if _, err := authenticator.Complete(context.Background(), identity.BrowserCallback{State: flow.State, Code: "valid-code"}); !errors.Is(err, identity.ErrBrowserFlowInvalid) {
		t.Fatalf("replayed state error = %v", err)
	}
	if provider.lastVerifier() == "" || pkceChallenge(provider.lastVerifier()) != query.Get("code_challenge") {
		t.Fatal("token exchange did not prove the authorization PKCE challenge")
	}
}

func TestOIDCFlowRejectsHostileTokenClaims(t *testing.T) {
	provider := newTestProvider(t)
	application := startTestApplication(t, provider, []SubjectMapping{{Subject: "upstream-1", ActorID: "person-1", ActorType: "human"}})
	authenticator, _ := application.BrowserAuthentication()
	tests := []struct {
		name   string
		claims func(string) tokenClaims
	}{
		{name: "wrong nonce", claims: func(string) tokenClaims {
			return tokenClaims{subject: "upstream-1", audience: "client-1", nonce: "wrong"}
		}},
		{name: "wrong audience", claims: func(nonce string) tokenClaims {
			return tokenClaims{subject: "upstream-1", audience: "another-client", nonce: nonce}
		}},
		{name: "expired", claims: func(nonce string) tokenClaims {
			return tokenClaims{subject: "upstream-1", audience: "client-1", nonce: nonce, expired: true}
		}},
		{name: "unmapped subject", claims: func(nonce string) tokenClaims {
			return tokenClaims{subject: "unknown", audience: "client-1", nonce: nonce}
		}},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flow, err := authenticator.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			parsed, _ := url.Parse(flow.AuthorizationURL)
			code := fmt.Sprintf("hostile-%d", index)
			provider.allow(code, test.claims(parsed.Query().Get("nonce")), parsed.Query().Get("code_challenge"))
			if _, err := authenticator.Complete(context.Background(), identity.BrowserCallback{State: flow.State, Code: code}); !errors.Is(err, identity.ErrBrowserFlowInvalid) {
				t.Fatalf("Complete error = %v", err)
			}
			if _, err := authenticator.Complete(context.Background(), identity.BrowserCallback{State: flow.State, Code: code}); !errors.Is(err, identity.ErrBrowserFlowInvalid) {
				t.Fatalf("failed flow state was reusable: %v", err)
			}
		})
	}
}

func TestOptionsRejectAmbiguousOrUnsafeTrustConfiguration(t *testing.T) {
	valid := Options{
		IssuerURL: "https://issuer.example.test", ClientID: "client", RedirectURL: "https://app.example.test/callback",
		SubjectMappings: []SubjectMapping{{Subject: "subject", ActorID: "person", ActorType: "human"}},
	}
	tests := []struct {
		name   string
		mutate func(*Options)
	}{
		{name: "insecure issuer", mutate: func(value *Options) { value.IssuerURL = "http://issuer.example.test" }},
		{name: "redirect query", mutate: func(value *Options) { value.RedirectURL += "?next=evil" }},
		{name: "userinfo", mutate: func(value *Options) { value.IssuerURL = "https://user@issuer.example.test" }},
		{name: "missing openid", mutate: func(value *Options) { value.Scopes = []string{"profile"} }},
		{name: "duplicate subject", mutate: func(value *Options) { value.SubjectMappings = append(value.SubjectMappings, value.SubjectMappings[0]) }},
		{name: "no mapping", mutate: func(value *Options) { value.SubjectMappings = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			candidate.SubjectMappings = append([]SubjectMapping(nil), valid.SubjectMappings...)
			test.mutate(&candidate)
			if _, err := Module(candidate); err == nil {
				t.Fatal("Module accepted unsafe configuration")
			}
		})
	}
	registration, err := Module(valid)
	if err != nil {
		t.Fatal(err)
	}
	if got := registration.Definition.Manifest; len(got.Requires) != 1 || got.Requires[0] != module.CapabilityIdentity || len(got.Provides) != 1 || got.Provides[0] != module.CapabilityBrowserAuthentication {
		t.Fatalf("manifest = %#v", got)
	}
}

func startTestApplication(t *testing.T, provider *testProvider, mappings []SubjectMapping) *appkit.Application {
	t.Helper()
	identityModule := module.Register(module.Manifest{
		SchemaVersion: module.SchemaVersion, ID: "test-identities", Version: "0.1.0", Type: module.ModuleTypeAdapter,
		Provides: []module.Capability{module.CapabilityIdentity},
	}, func(_ context.Context, scope module.Scope) error {
		return module.Provide(scope, module.IdentityResolver(), identity.Resolver(staticResolver{}))
	})
	oidcModule, err := Module(Options{
		IssuerURL: provider.server.URL, ClientID: "client-1", ClientSecret: "secret-1",
		RedirectURL: "http://127.0.0.1/callback", AllowInsecureHTTP: true, SubjectMappings: mappings,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := appkit.Start(context.Background(), appkit.Definition{
		Metadata: appkit.Metadata{ID: "oidc-test", Name: "OIDC Test", Version: "0.1.0"},
		Modules:  []module.Registration{oidcModule, identityModule},
	}, appkit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := application.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return application
}

type staticResolver struct{}

func (staticResolver) ResolveByID(_ context.Context, id string) (identity.Actor, error) {
	if id != "person-1" {
		return identity.Actor{}, identity.ErrActorNotFound
	}
	return identity.Actor{ID: id, Type: "human", DisplayName: "Person One"}, nil
}

type tokenClaims struct {
	subject  string
	audience string
	nonce    string
	expired  bool
}

type allowedCode struct {
	claims    tokenClaims
	challenge string
}

type testProvider struct {
	t                 *testing.T
	server            *httptest.Server
	key               *rsa.PrivateKey
	mu                sync.Mutex
	allowed           map[string]allowedCode
	lastVerifierValue string
}

func newTestProvider(t *testing.T) *testProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := &testProvider{t: t, key: key, allowed: make(map[string]allowedCode)}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(provider.server.Close)
	return provider
}

func (provider *testProvider) allow(code string, claims tokenClaims, challenge string) {
	provider.mu.Lock()
	provider.allowed[code] = allowedCode{claims: claims, challenge: challenge}
	provider.mu.Unlock()
}

func (provider *testProvider) lastVerifier() string {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.lastVerifierValue
}

func (provider *testProvider) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/.well-known/openid-configuration":
		writeTestJSON(writer, map[string]any{
			"issuer": provider.server.URL, "authorization_endpoint": provider.server.URL + "/authorize",
			"token_endpoint": provider.server.URL + "/token", "jwks_uri": provider.server.URL + "/keys",
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	case "/keys":
		writeTestJSON(writer, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &provider.key.PublicKey, KeyID: "test-key", Algorithm: "RS256", Use: "sig"}}})
	case "/token":
		provider.serveToken(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

func (provider *testProvider) serveToken(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "bad form", http.StatusBadRequest)
		return
	}
	provider.mu.Lock()
	allowed, ok := provider.allowed[request.Form.Get("code")]
	delete(provider.allowed, request.Form.Get("code"))
	provider.lastVerifierValue = request.Form.Get("code_verifier")
	provider.mu.Unlock()
	if !ok || request.Form.Get("grant_type") != "authorization_code" || request.Form.Get("redirect_uri") != "http://127.0.0.1/callback" || pkceChallenge(request.Form.Get("code_verifier")) != allowed.challenge {
		writeTestJSONStatus(writer, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	now := time.Now().UTC()
	expires := now.Add(5 * time.Minute)
	if allowed.claims.expired {
		expires = now.Add(-5 * time.Minute)
	}
	payload, _ := json.Marshal(map[string]any{
		"iss": provider.server.URL, "sub": allowed.claims.subject, "aud": allowed.claims.audience,
		"exp": expires.Unix(), "iat": now.Add(-time.Minute).Unix(), "nonce": allowed.claims.nonce,
	})
	signingKey := jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: provider.key, KeyID: "test-key", Algorithm: "RS256", Use: "sig"}}
	signer, err := jose.NewSigner(signingKey, nil)
	if err != nil {
		provider.t.Fatal(err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		provider.t.Fatal(err)
	}
	compact, err := signed.CompactSerialize()
	if err != nil {
		provider.t.Fatal(err)
	}
	writeTestJSON(writer, map[string]any{"access_token": "opaque", "token_type": "Bearer", "expires_in": 300, "id_token": compact})
}

func pkceChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func writeTestJSON(writer http.ResponseWriter, value any) {
	writeTestJSONStatus(writer, http.StatusOK, value)
}

func writeTestJSONStatus(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func TestBoundedTransportRejectsOversizedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("x", 33)))
	}))
	defer server.Close()
	client := &http.Client{Transport: boundedTransport{next: http.DefaultTransport, maximum: 32}}
	response, err := client.Get(server.URL)
	if err != nil {
		if !strings.Contains(err.Error(), "exceeds limit") {
			t.Fatal(err)
		}
		return
	}
	defer response.Body.Close()
	buffer := make([]byte, 64)
	if _, err := response.Body.Read(buffer); err != nil {
		t.Fatal(err)
	}
	if _, err := response.Body.Read(buffer); err == nil {
		t.Fatal("oversized response was not rejected")
	}
}

func TestHTTPContributionBindsStateAndEstablishesStrictSession(t *testing.T) {
	browser := &stubBrowserAuthenticator{}
	sessions := &stubSessionManager{}
	registration := module.Register(module.Manifest{
		SchemaVersion: module.SchemaVersion, ID: "browser-session-test", Version: "0.1.0", Type: module.ModuleTypeAdapter,
		Provides: []module.Capability{module.CapabilityBrowserAuthentication, module.CapabilitySessions},
	}, func(_ context.Context, scope module.Scope) error {
		if err := module.Provide(scope, module.BrowserAuthenticator(), identity.BrowserAuthenticator(browser)); err != nil {
			return err
		}
		return module.Provide(scope, module.SessionManager(), identity.SessionManager(sessions))
	})
	definition := appkit.Definition{
		Metadata: appkit.Metadata{ID: "http-test", Name: "HTTP Test", Version: "0.1.0"},
		Modules:  []module.Registration{registration},
	}
	contribution, err := oidchttp.Contribution(oidchttp.HTTPOptions{AllowInsecureCookie: true})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := httpkit.NewPlan(definition, appkit.Options{}, contribution)
	if err != nil {
		t.Fatal(err)
	}
	application, err := appkit.Start(context.Background(), definition, appkit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	handler, err := plan.Handler(context.Background(), application)
	if err != nil {
		t.Fatal(err)
	}

	login := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/login", nil)
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusSeeOther || loginResponse.Header().Get("Location") != "https://issuer.example.test/authorize" {
		t.Fatalf("login response = %d %q", loginResponse.Code, loginResponse.Header().Get("Location"))
	}
	flowCookie := responseCookie(t, loginResponse.Result(), oidchttp.DefaultFlowCookieName)
	if flowCookie.Value != "flow-state" || !flowCookie.HttpOnly || flowCookie.SameSite != http.SameSiteLaxMode || flowCookie.Secure || flowCookie.Path != "/api/auth/oidc/callback" {
		t.Fatalf("flow cookie = %#v", flowCookie)
	}

	callback := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?state=flow-state&code=valid-code", nil)
	callback.AddCookie(flowCookie)
	callbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(callbackResponse, callback)
	if callbackResponse.Code != http.StatusSeeOther || callbackResponse.Header().Get("Location") != "/" {
		t.Fatalf("callback response = %d %q body=%s", callbackResponse.Code, callbackResponse.Header().Get("Location"), callbackResponse.Body.String())
	}
	sessionCookie := responseCookie(t, callbackResponse.Result(), "modary_session")
	if sessionCookie.Value != "session-token" || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteStrictMode || sessionCookie.Secure {
		t.Fatalf("session cookie = %#v", sessionCookie)
	}
	if browser.completed != 1 || sessions.created != 1 {
		t.Fatalf("complete calls=%d session calls=%d", browser.completed, sessions.created)
	}

	duplicate := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?state=flow-state&state=other&code=valid-code", nil)
	duplicate.AddCookie(flowCookie)
	duplicateResponse := httptest.NewRecorder()
	handler.ServeHTTP(duplicateResponse, duplicate)
	if duplicateResponse.Code != http.StatusBadRequest {
		t.Fatalf("duplicate callback status = %d", duplicateResponse.Code)
	}
	cleared := responseCookie(t, duplicateResponse.Result(), oidchttp.DefaultFlowCookieName)
	if cleared.MaxAge >= 0 {
		t.Fatalf("malformed callback did not clear flow cookie: %#v", cleared)
	}
}

type stubBrowserAuthenticator struct{ completed int }

func (*stubBrowserAuthenticator) Begin(context.Context) (identity.BrowserFlow, error) {
	return identity.BrowserFlow{
		AuthorizationURL: "https://issuer.example.test/authorize", State: "flow-state", ExpiresAt: time.Now().Add(time.Minute),
	}, nil
}

func (authenticator *stubBrowserAuthenticator) Complete(_ context.Context, callback identity.BrowserCallback) (identity.Authentication, error) {
	if callback.State != "flow-state" || callback.Code != "valid-code" {
		return identity.Authentication{}, identity.ErrBrowserFlowInvalid
	}
	authenticator.completed++
	return identity.Authentication{Actor: identity.Actor{ID: "person-1", Type: "human", DisplayName: "Person One"}, Method: identity.AuthenticationMethodOIDC}, nil
}

type stubSessionManager struct{ created int }

func (manager *stubSessionManager) CreateSession(_ context.Context, authentication identity.Authentication) (identity.Session, error) {
	manager.created++
	return identity.Session{
		Token: "session-token", CSRFToken: "0123456789abcdef0123456789abcdef",
		Actor: authentication.Actor, ExpiresAt: time.Now().Add(time.Hour),
	}, nil
}
func (*stubSessionManager) RevokeSession(context.Context, string) error { return nil }
func (*stubSessionManager) ResolveSession(context.Context, string) (identity.Session, error) {
	return identity.Session{}, identity.ErrSessionInvalid
}

func responseCookie(t *testing.T, response *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response has no %s cookie: %v", name, response.Header.Values("Set-Cookie"))
	return nil
}
