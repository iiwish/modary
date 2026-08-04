package oidc

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/iiwish/modary/identity"
	"golang.org/x/oauth2"
)

const maximumIDTokenBytes = 32 << 10

type pendingFlow struct {
	verifier string
	nonce    string
	expires  time.Time
}

type authenticator struct {
	mu              sync.Mutex
	stopped         bool
	pending         map[string]pendingFlow
	resolver        identity.Resolver
	verifier        *coreoidc.IDTokenVerifier
	oauth           oauth2.Config
	mappings        map[string]SubjectMapping
	flowTTL         time.Duration
	maxPendingFlows int
	httpClient      *http.Client
	clock           func() time.Time
}

func (service *authenticator) Begin(ctx context.Context) (identity.BrowserFlow, error) {
	if ctx == nil {
		return identity.BrowserFlow{}, identity.ErrBrowserFlowInvalid
	}
	if err := ctx.Err(); err != nil {
		return identity.BrowserFlow{}, err
	}
	state, err := randomToken(32)
	if err != nil {
		return identity.BrowserFlow{}, fmt.Errorf("create OIDC state: %w", err)
	}
	nonce, err := randomToken(32)
	if err != nil {
		return identity.BrowserFlow{}, fmt.Errorf("create OIDC nonce: %w", err)
	}
	verifier, err := randomToken(32)
	if err != nil {
		return identity.BrowserFlow{}, fmt.Errorf("create OIDC PKCE verifier: %w", err)
	}
	now := service.clock().UTC()
	expires := now.Add(service.flowTTL)
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.stopped {
		return identity.BrowserFlow{}, identity.ErrBrowserFlowInvalid
	}
	for key, flow := range service.pending {
		if !flow.expires.After(now) {
			delete(service.pending, key)
		}
	}
	if len(service.pending) >= service.maxPendingFlows {
		return identity.BrowserFlow{}, fmt.Errorf("OIDC pending flow capacity is exhausted")
	}
	service.pending[state] = pendingFlow{verifier: verifier, nonce: nonce, expires: expires}
	authorizationURL := service.oauth.AuthCodeURL(state, coreoidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
	return identity.BrowserFlow{AuthorizationURL: authorizationURL, State: state, ExpiresAt: expires}, nil
}

func (service *authenticator) Complete(ctx context.Context, callback identity.BrowserCallback) (identity.Authentication, error) {
	if ctx == nil || validateCallback(callback) != nil {
		return identity.Authentication{}, identity.ErrBrowserFlowInvalid
	}
	flow, ok := service.consume(callback.State)
	if !ok {
		return identity.Authentication{}, identity.ErrBrowserFlowInvalid
	}
	if err := ctx.Err(); err != nil {
		return identity.Authentication{}, err
	}
	requestContext := context.WithValue(ctx, oauth2.HTTPClient, service.httpClient)
	token, err := service.oauth.Exchange(requestContext, callback.Code, oauth2.VerifierOption(flow.verifier))
	if err != nil {
		return identity.Authentication{}, identity.ErrBrowserFlowInvalid
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || len(rawIDToken) == 0 || len(rawIDToken) > maximumIDTokenBytes {
		return identity.Authentication{}, identity.ErrBrowserFlowInvalid
	}
	idToken, err := service.verifier.Verify(coreoidc.ClientContext(ctx, service.httpClient), rawIDToken)
	if err != nil || subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(flow.nonce)) != 1 {
		return identity.Authentication{}, identity.ErrBrowserFlowInvalid
	}
	var claims struct {
		Subject string `json:"sub"`
	}
	if err := idToken.Claims(&claims); err != nil || validateBoundedText("subject", claims.Subject, 512, true) != nil {
		return identity.Authentication{}, identity.ErrBrowserFlowInvalid
	}
	mapping, found := service.mappings[claims.Subject]
	if !found {
		return identity.Authentication{}, identity.ErrBrowserFlowInvalid
	}
	actor, err := service.resolver.ResolveByID(ctx, mapping.ActorID)
	if err != nil || actor.Type != mapping.ActorType || identity.ValidateActor(actor) != nil {
		return identity.Authentication{}, identity.ErrBrowserFlowInvalid
	}
	authentication := identity.Authentication{Actor: actor, Method: identity.AuthenticationMethodOIDC}
	if identity.ValidateAuthentication(authentication) != nil {
		return identity.Authentication{}, identity.ErrBrowserFlowInvalid
	}
	return authentication, nil
}

func (service *authenticator) consume(state string) (pendingFlow, bool) {
	now := service.clock().UTC()
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.stopped {
		return pendingFlow{}, false
	}
	flow, ok := service.pending[state]
	delete(service.pending, state)
	if !ok || !flow.expires.After(now) {
		return pendingFlow{}, false
	}
	return flow, true
}

func (service *authenticator) stop(context.Context) error {
	service.mu.Lock()
	service.stopped = true
	clear(service.pending)
	service.mu.Unlock()
	return nil
}

func validateCallback(callback identity.BrowserCallback) error {
	if validateBoundedText("state", callback.State, 256, true) != nil || validateBoundedText("code", callback.Code, 8192, true) != nil {
		return identity.ErrBrowserFlowInvalid
	}
	return nil
}

func randomToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func cloneBoundedHTTPClient(source *http.Client) *http.Client {
	var cloned http.Client
	if source == nil {
		cloned = http.Client{Timeout: 30 * time.Second}
	} else {
		cloned = *source
		if cloned.Timeout <= 0 || cloned.Timeout > time.Minute {
			cloned.Timeout = 30 * time.Second
		}
	}
	transport := cloned.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	cloned.Transport = boundedTransport{next: transport, maximum: maximumUpstreamResponseBytes}
	return &cloned
}

type boundedTransport struct {
	next    http.RoundTripper
	maximum int64
}

func (transport boundedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.next.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.ContentLength > transport.maximum {
		_ = response.Body.Close()
		return nil, errors.New("OIDC upstream response exceeds limit")
	}
	response.Body = &boundedBody{next: response.Body, remaining: transport.maximum}
	return response, nil
}

type boundedBody struct {
	next      io.ReadCloser
	remaining int64
	exhausted bool
}

func (body *boundedBody) Read(buffer []byte) (int, error) {
	if body.exhausted {
		return 0, errors.New("OIDC upstream response exceeds limit")
	}
	if int64(len(buffer)) > body.remaining {
		buffer = buffer[:body.remaining]
	}
	if len(buffer) > 0 {
		count, err := body.next.Read(buffer)
		body.remaining -= int64(count)
		return count, err
	}
	probe := []byte{0}
	count, err := body.next.Read(probe)
	if count > 0 {
		body.exhausted = true
		return 0, errors.New("OIDC upstream response exceeds limit")
	}
	return 0, err
}

func (body *boundedBody) Close() error { return body.next.Close() }
