package module

import (
	"context"

	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/internal/testsupport"
)

var testActionPersistenceKey = MustKey[runtimecontrol.Persistence](runtimecontrol.ServiceName, "database")

type testRuntimeServices struct {
	authorizer   authz.Authorizer
	audit        audit.Hook
	plans        actionpersistence.PlanStore
	idempotency  actionpersistence.IdempotencyStore
	transactions runtimecontrol.TransactionManager
}

func testRuntimeServicesRegistration(services testRuntimeServices) Registration {
	if services.authorizer == nil {
		services.authorizer = allowAllAuthorizer{}
	}
	if services.audit == nil {
		services.audit = testsupport.DiscardAudit{}
	}
	if services.plans == nil {
		services.plans = testsupport.NewMemoryPlanStore()
	}
	if services.idempotency == nil {
		services.idempotency = testsupport.NewMemoryIdempotencyStore()
	}
	if services.transactions == nil {
		services.transactions = testsupport.DirectTransactions{}
	}
	return Register(Manifest{
		SchemaVersion: SchemaVersion, ID: "runtime-services", Version: "0.1.0", Type: ModuleTypeAdapter,
		Provides: []Capability{CapabilityAudit, CapabilityAuthorization, CapabilityDatabase},
	}, func(_ context.Context, scope Scope) error {
		if err := Provide(scope, Authorizer(), services.authorizer); err != nil {
			return err
		}
		if err := Provide(scope, AuditHook(), services.audit); err != nil {
			return err
		}
		persistence, err := runtimecontrol.New(services.plans, services.idempotency, services.transactions)
		if err != nil {
			return err
		}
		return Provide(scope, testActionPersistenceKey, persistence)
	})
}
