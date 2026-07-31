// Package moduleassembly owns private service keys used by official durable
// adapters. Consumer Modules cannot import this package, recreate key identity,
// or name the sealed database and Action-persistence control types.
package moduleassembly

import (
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/module"
)

var databaseControlKey = module.MustKey[databasecontrol.Control](databasecontrol.ServiceName, "database")
var actionPersistenceKey = module.MustKey[runtimecontrol.Persistence](runtimecontrol.ServiceName, "database")

// ProvideDatabase installs the distinct public Access and internal Control
// capabilities through an active official adapter Scope.
func ProvideDatabase(scope module.Scope, control databasecontrol.Control) error {
	return module.Provide(scope, databaseControlKey, control)
}

// ResolveDatabaseControl resolves internal database control for an official
// durable adapter during its declared startup lifecycle.
func ResolveDatabaseControl(resolver module.Resolver) (databasecontrol.Control, error) {
	return module.Resolve(resolver, databaseControlKey)
}

// ProvideActionPersistence atomically installs the official plan,
// idempotency, and transaction services as one private bundle.
func ProvideActionPersistence(
	scope module.Scope,
	plans actionpersistence.PlanStore,
	idempotency actionpersistence.IdempotencyStore,
	transactions runtimecontrol.TransactionManager,
) error {
	persistence, err := runtimecontrol.New(plans, idempotency, transactions)
	if err != nil {
		return err
	}
	return module.Provide(scope, actionPersistenceKey, persistence)
}
