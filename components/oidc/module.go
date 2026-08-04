// Package oidc provides a production-oriented OpenID Connect authentication
// component. It authenticates an upstream subject into an explicitly
// provisioned Modary principal; product scope, roles, and permissions are never
// accepted from identity-provider claims.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package oidc

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/module"
	"golang.org/x/oauth2"
)

const (
	// ModuleID is the stable module manifest identifier.
	ModuleID = "oidc"
	// DefaultFlowTTL bounds one browser redirect ceremony.
	DefaultFlowTTL = 10 * time.Minute
	// DefaultMaxPendingFlows bounds in-process state retained by one instance.
	DefaultMaxPendingFlows = 4096
	// MaximumPendingFlows is the hard configuration limit.
	MaximumPendingFlows          = 65536
	maximumUpstreamResponseBytes = 1 << 20
)

// SubjectMapping binds one exact provider subject to one already-provisioned
// Modary principal. ActorType is checked again on every completed login so a
// principal type change cannot silently inherit the old mapping.
type SubjectMapping struct {
	Subject   string
	ActorID   string
	ActorType string
}

// Options is one explicit OIDC trust configuration. Issuer discovery,
// signature verification, audience verification, nonce, state, and PKCE S256
// are mandatory. ClientSecret may be empty for a public client.
type Options struct {
	IssuerURL       string
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	Scopes          []string
	SubjectMappings []SubjectMapping
	FlowTTL         time.Duration
	MaxPendingFlows int
	// AllowInsecureHTTP permits HTTP issuer and redirect URLs for an explicitly
	// isolated local/test environment. Production should always leave it false.
	AllowInsecureHTTP bool
	// HTTPClient optionally supplies trusted proxy/TLS behavior. The component
	// clones it and still enforces bounded upstream response bodies.
	HTTPClient *http.Client
}

type normalizedOptions struct {
	issuerURL       string
	clientID        string
	clientSecret    string
	redirectURL     string
	scopes          []string
	mappings        map[string]SubjectMapping
	flowTTL         time.Duration
	maxPendingFlows int
	httpClient      *http.Client
}

// Module validates and copies configuration without network or database side
// effects. Provider discovery and principal checks occur during lifecycle
// startup, before the browser-authentication capability is published.
func Module(options Options) (module.Registration, error) {
	normalized, err := normalizeOptions(options)
	if err != nil {
		return module.Registration{}, err
	}
	return module.Registration{
		Definition: module.Definition{Manifest: module.Manifest{
			SchemaVersion: module.SchemaVersion,
			ID:            ModuleID,
			Version:       "0.1.0",
			Type:          module.ModuleTypeAdapter,
			Requires:      []module.Capability{module.CapabilityIdentity},
			Provides:      []module.Capability{module.CapabilityBrowserAuthentication},
		}},
		Start: func(ctx context.Context, scope module.Scope) error {
			return start(ctx, scope, normalized)
		},
	}, nil
}

func start(ctx context.Context, installation module.Scope, options normalizedOptions) error {
	if ctx == nil {
		return fmt.Errorf("OIDC start context is required")
	}
	resolver, err := module.Resolve(installation, module.IdentityResolver())
	if err != nil {
		return fmt.Errorf("resolve OIDC identity store: %w", err)
	}
	for subject, mapping := range options.mappings {
		actor, resolveErr := resolver.ResolveByID(ctx, mapping.ActorID)
		if resolveErr != nil {
			return fmt.Errorf("verify OIDC subject mapping %q: %w", subject, resolveErr)
		}
		if actor.Type != mapping.ActorType {
			return fmt.Errorf("verify OIDC subject mapping %q: actor type does not match provisioned principal", subject)
		}
	}
	providerContext := coreoidc.ClientContext(ctx, options.httpClient)
	provider, err := coreoidc.NewProvider(providerContext, options.issuerURL)
	if err != nil {
		return fmt.Errorf("discover OIDC provider: %w", err)
	}
	service := &authenticator{
		resolver: resolver,
		verifier: provider.Verifier(&coreoidc.Config{ClientID: options.clientID}),
		oauth: oauth2.Config{
			ClientID: options.clientID, ClientSecret: options.clientSecret,
			Endpoint: provider.Endpoint(), RedirectURL: options.redirectURL,
			Scopes: append([]string(nil), options.scopes...),
		},
		mappings: options.mappings, flowTTL: options.flowTTL,
		maxPendingFlows: options.maxPendingFlows, httpClient: options.httpClient,
		clock: time.Now, pending: make(map[string]pendingFlow),
	}
	if err := module.OnStop(installation, service.stop); err != nil {
		return err
	}
	return module.Provide(installation, module.BrowserAuthenticator(), identity.BrowserAuthenticator(service))
}

func normalizeOptions(options Options) (normalizedOptions, error) {
	issuer, err := validateAbsoluteURL("issuer URL", options.IssuerURL, options.AllowInsecureHTTP)
	if err != nil {
		return normalizedOptions{}, err
	}
	redirect, err := validateAbsoluteURL("redirect URL", options.RedirectURL, options.AllowInsecureHTTP)
	if err != nil {
		return normalizedOptions{}, err
	}
	if redirect.RawQuery != "" || redirect.Fragment != "" {
		return normalizedOptions{}, fmt.Errorf("OIDC redirect URL cannot contain query or fragment")
	}
	if err := validateBoundedText("client id", options.ClientID, 512, true); err != nil {
		return normalizedOptions{}, fmt.Errorf("OIDC %w", err)
	}
	if len(options.ClientSecret) > 4096 || !utf8.ValidString(options.ClientSecret) {
		return normalizedOptions{}, fmt.Errorf("OIDC client secret is invalid")
	}
	if options.FlowTTL < 0 || options.FlowTTL > time.Hour {
		return normalizedOptions{}, fmt.Errorf("OIDC flow TTL must be between zero and one hour")
	}
	if options.FlowTTL == 0 {
		options.FlowTTL = DefaultFlowTTL
	}
	if options.MaxPendingFlows < 0 || options.MaxPendingFlows > MaximumPendingFlows {
		return normalizedOptions{}, fmt.Errorf("OIDC maximum pending flows must be between zero and %d", MaximumPendingFlows)
	}
	if options.MaxPendingFlows == 0 {
		options.MaxPendingFlows = DefaultMaxPendingFlows
	}
	scopes := append([]string(nil), options.Scopes...)
	if len(scopes) == 0 {
		scopes = []string{coreoidc.ScopeOpenID, "profile"}
	}
	if len(scopes) > 32 {
		return normalizedOptions{}, fmt.Errorf("OIDC scope count exceeds 32")
	}
	seenScopes := make(map[string]struct{}, len(scopes))
	for index, value := range scopes {
		if err := validateBoundedText("scope", value, 128, true); err != nil || strings.ContainsAny(value, " \t\r\n") {
			return normalizedOptions{}, fmt.Errorf("OIDC scope %d is invalid", index)
		}
		if _, duplicate := seenScopes[value]; duplicate {
			return normalizedOptions{}, fmt.Errorf("OIDC scope %q is declared more than once", value)
		}
		seenScopes[value] = struct{}{}
	}
	if _, ok := seenScopes[coreoidc.ScopeOpenID]; !ok {
		return normalizedOptions{}, fmt.Errorf("OIDC scopes must include openid")
	}
	if len(options.SubjectMappings) == 0 || len(options.SubjectMappings) > 4096 {
		return normalizedOptions{}, fmt.Errorf("OIDC requires between 1 and 4096 subject mappings")
	}
	mappings := make(map[string]SubjectMapping, len(options.SubjectMappings))
	for index, mapping := range options.SubjectMappings {
		if err := validateBoundedText("subject", mapping.Subject, 512, true); err != nil {
			return normalizedOptions{}, fmt.Errorf("OIDC subject mapping %d: %w", index, err)
		}
		if err := identity.ValidateActorID(mapping.ActorID); err != nil {
			return normalizedOptions{}, fmt.Errorf("OIDC subject mapping %d: %w", index, err)
		}
		if err := identity.ValidateActorType(mapping.ActorType); err != nil {
			return normalizedOptions{}, fmt.Errorf("OIDC subject mapping %d: %w", index, err)
		}
		if _, duplicate := mappings[mapping.Subject]; duplicate {
			return normalizedOptions{}, fmt.Errorf("OIDC subject %q is declared more than once", mapping.Subject)
		}
		mappings[mapping.Subject] = mapping
	}
	client := cloneBoundedHTTPClient(options.HTTPClient)
	return normalizedOptions{
		issuerURL: issuer.String(), clientID: options.ClientID, clientSecret: options.ClientSecret,
		redirectURL: redirect.String(), scopes: slices.Clone(scopes), mappings: mappings,
		flowTTL: options.FlowTTL, maxPendingFlows: options.MaxPendingFlows, httpClient: client,
	}, nil
}

func validateAbsoluteURL(name, value string, allowInsecure bool) (*url.URL, error) {
	if len(value) == 0 || len(value) > 2048 || strings.TrimSpace(value) != value {
		return nil, fmt.Errorf("OIDC %s is invalid", name)
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("OIDC %s is invalid", name)
	}
	if parsed.Scheme != "https" && !(allowInsecure && parsed.Scheme == "http") {
		return nil, fmt.Errorf("OIDC %s must use HTTPS", name)
	}
	if name == "issuer URL" && (parsed.RawQuery != "" || parsed.Fragment != "") {
		return nil, fmt.Errorf("OIDC issuer URL cannot contain query or fragment")
	}
	return parsed, nil
}

func validateBoundedText(name, value string, maximum int, required bool) error {
	if !utf8.ValidString(value) || len(value) > maximum || strings.TrimSpace(value) != value || strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s is invalid", name)
	}
	if required && value == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
