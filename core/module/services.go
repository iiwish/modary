package module

const (
	ServiceConfig           = "core.config"
	ServiceActionRegistry   = "core.action.registry"
	ServiceDatabase         = "database"
	ServiceTransactions     = "database.transactions"
	ServicePlanStore        = "action.plan-store"
	ServiceIdempotencyStore = "action.idempotency-store"
	ServiceIdentityResolver = "identity.resolver"
	ServiceIdentityStore    = "identity.store"
	ServiceAuthorizer       = "authorization"
	ServiceAuditHook        = "audit.hook"
	ServiceAuditStore       = "audit.store"
)
