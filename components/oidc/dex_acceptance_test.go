package oidc

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/module"
)

func TestDisposableDexAuthorizationCodeFlow(t *testing.T) {
	issuer := os.Getenv("MODARY_TEST_DEX_ISSUER_URL")
	if issuer == "" {
		t.Skip("MODARY_TEST_DEX_ISSUER_URL is required for disposable Dex acceptance")
	}
	identityModule := module.Register(module.Manifest{
		SchemaVersion: module.SchemaVersion,
		ID:            "dex-acceptance-identities",
		Version:       "0.1.0",
		Type:          module.ModuleTypeAdapter,
		Provides:      []module.Capability{module.CapabilityIdentity},
	}, func(_ context.Context, installation module.Scope) error {
		return module.Provide(installation, module.IdentityResolver(), identity.Resolver(staticResolver{}))
	})
	oidcModule, err := Module(Options{
		IssuerURL: issuer, ClientID: "modary-acceptance", ClientSecret: "modary-acceptance-secret",
		RedirectURL: "http://127.0.0.1:5557/callback", AllowInsecureHTTP: true,
		SubjectMappings: []SubjectMapping{{Subject: "ChJtb2RhcnktZGV4LXN1YmplY3QSBWxvY2Fs", ActorID: "person-1", ActorType: "human"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := appkit.Start(context.Background(), appkit.Definition{
		Metadata: appkit.Metadata{ID: "dex-acceptance", Name: "Dex Acceptance", Version: "0.1.0"},
		Modules:  []module.Registration{identityModule, oidcModule},
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
	authenticator, err := application.BrowserAuthentication()
	if err != nil {
		t.Fatal(err)
	}
	flow, err := authenticator.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar:     jar,
		Timeout: 15 * time.Second,
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			if request.URL.Host == "127.0.0.1:5557" {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	loginPage, err := client.Get(flow.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	_ = loginPage.Body.Close()
	if loginPage.StatusCode != http.StatusOK || !strings.HasSuffix(loginPage.Request.URL.Path, "/auth/local/login") {
		t.Fatalf("Dex login page = %d %s", loginPage.StatusCode, loginPage.Request.URL)
	}
	loginResponse, err := client.PostForm(loginPage.Request.URL.String(), url.Values{
		"login":    {"admin@example.com"},
		"password": {"modary-acceptance-password"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusSeeOther && loginResponse.StatusCode != http.StatusFound {
		t.Fatalf("Dex login response = %d", loginResponse.StatusCode)
	}
	callbackURL, err := url.Parse(loginResponse.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := identity.BrowserCallback{State: callbackURL.Query().Get("state"), Code: callbackURL.Query().Get("code")}
	if callback.State != flow.State || callback.Code == "" {
		t.Fatalf("Dex callback is incomplete: %s", callbackURL.Redacted())
	}
	authentication, err := authenticator.Complete(context.Background(), callback)
	if err != nil {
		t.Fatal(err)
	}
	if authentication.Method != identity.AuthenticationMethodOIDC || authentication.Actor.ID != "person-1" || authentication.Actor.Type != "human" {
		t.Fatalf("authentication = %#v", authentication)
	}
}
