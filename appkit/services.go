package appkit

import (
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/database"
)

// Database returns the optional lifecycle-gated business-data Store installed
// by the consumer's Modules.
func (application *Application) Database() (database.Store, error) {
	if application == nil || !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.database == nil {
		return nil, ErrDatabaseUnavailable
	}
	return application.database, nil
}

// Authorizer returns the optional lifecycle-gated policy evaluator installed
// by the consumer's Modules.
func (application *Application) Authorizer() (authz.Authorizer, error) {
	if application == nil || !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.authorizer == nil {
		return nil, ErrAuthorizerUnavailable
	}
	return application.authorizer, nil
}
