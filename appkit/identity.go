package appkit

import "github.com/iiwish/modary/identity"

// Identities returns the optional actor resolver installed by the consumer's
// Modules.
func (application *Application) Identities() (identity.Resolver, error) {
	if application == nil {
		return nil, ErrApplicationUnavailable
	}
	if !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.identities == nil {
		return nil, ErrIdentitiesUnavailable
	}
	return application.identities, nil
}

// Sessions returns the optional session authenticator installed by the
// consumer's Modules.
func (application *Application) Sessions() (identity.Authenticator, error) {
	if application == nil {
		return nil, ErrApplicationUnavailable
	}
	if !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.sessions == nil {
		return nil, ErrSessionsUnavailable
	}
	return application.sessions, nil
}

// Tokens returns the optional bearer-token authenticator installed by the
// consumer's Modules.
func (application *Application) Tokens() (identity.TokenAuthenticator, error) {
	if application == nil {
		return nil, ErrApplicationUnavailable
	}
	if !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.tokens == nil {
		return nil, ErrTokensUnavailable
	}
	return application.tokens, nil
}
