package app

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"modary/core/action"
	"modary/core/audit"
	"modary/core/authz"
	"modary/core/config"
	"modary/core/identity"
	"modary/core/module"
	"modary/internal/generated"
)

type Application struct {
	Config          config.Runtime
	Host            *module.Host
	Registry        *action.Registry
	Runtime         *action.Runtime
	Identity        identity.Authenticator
	DB              *sql.DB
	StartupDuration time.Duration
}

func Bootstrap(ctx context.Context, cfg config.Runtime) (*Application, error) {
	return BootstrapWithRegistrar(ctx, cfg, generated.RegisterModules)
}

// BootstrapWithRegistrar runs the same application bootstrap with a generated
// or test-supplied static module composition.
func BootstrapWithRegistrar(ctx context.Context, cfg config.Runtime, register func(*module.Host) error) (*Application, error) {
	startedAt := time.Now()
	host := module.NewHost()
	registry := action.NewRegistry()
	if err := host.Provide(module.ServiceConfig, cfg); err != nil {
		return nil, err
	}
	if err := host.Provide(module.ServiceActionRegistry, registry); err != nil {
		return nil, err
	}
	if register == nil {
		return nil, fmt.Errorf("module registrar is required")
	}
	if err := register(host); err != nil {
		return nil, err
	}
	if err := host.Start(ctx); err != nil {
		return nil, err
	}
	if err := validateActionCatalog(host, registry); err != nil {
		return nil, err
	}
	db, err := module.ServiceAs[*sql.DB](host, module.ServiceDatabase)
	if err != nil {
		return nil, err
	}
	authorizer, err := module.ServiceAs[authz.Authorizer](host, module.ServiceAuthorizer)
	if err != nil {
		return nil, err
	}
	auditHook, err := module.ServiceAs[audit.Hook](host, module.ServiceAuditHook)
	if err != nil {
		return nil, err
	}
	plans, err := module.ServiceAs[action.PlanStore](host, module.ServicePlanStore)
	if err != nil {
		return nil, err
	}
	idempotency, err := module.ServiceAs[action.IdempotencyStore](host, module.ServiceIdempotencyStore)
	if err != nil {
		return nil, err
	}
	transactions, err := module.ServiceAs[action.TransactionManager](host, module.ServiceTransactions)
	if err != nil {
		return nil, err
	}
	authenticator, err := module.ServiceAs[identity.Authenticator](host, module.ServiceIdentityStore)
	if err != nil {
		return nil, err
	}
	runtime, err := action.NewRuntime(action.RuntimeOptions{
		Registry:     registry,
		Authorizer:   authorizer,
		Audit:        auditHook,
		Plans:        plans,
		Idempotency:  idempotency,
		Transactions: transactions,
		PlanTTL:      5 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	return &Application{
		Config: cfg, Host: host, Registry: registry, Runtime: runtime, Identity: authenticator, DB: db,
		StartupDuration: time.Since(startedAt),
	}, nil
}

func (a *Application) Close() error {
	if a.DB == nil {
		return nil
	}
	return a.DB.Close()
}

func validateActionCatalog(host *module.Host, registry *action.Registry) error {
	declared := make(map[string]string)
	for _, manifest := range host.Manifests() {
		for _, actionID := range manifest.Actions {
			declared[actionID] = manifest.ID
		}
	}
	registered := registry.List()
	for _, item := range registered {
		moduleID, ok := declared[item.Descriptor.ID]
		if !ok {
			return fmt.Errorf("registered action %s is missing from module manifest", item.Descriptor.ID)
		}
		if moduleID != item.ModuleID {
			return fmt.Errorf("action %s is declared by %s but registered by %s", item.Descriptor.ID, moduleID, item.ModuleID)
		}
		delete(declared, item.Descriptor.ID)
	}
	if len(declared) > 0 {
		missing := make([]string, 0, len(declared))
		for id := range declared {
			missing = append(missing, id)
		}
		sort.Strings(missing)
		return fmt.Errorf("manifest actions have no registered handler: %v", missing)
	}
	return nil
}
