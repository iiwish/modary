package action

import (
	"context"
	"time"

	"github.com/iiwish/modary/audit"
)

// Runtime is the governed execution surface for registered Actions. It never
// exposes the mutable binding table or raw Handlers owned by framework assembly.
// Runtime methods are safe for concurrent use. Callers control each supplied
// context independently, and trusted dependencies must return promptly after
// its cancellation.
type Runtime interface {
	Preview(context.Context, Request) (Preview, error)
	Execute(context.Context, Request) (Result, error)
	CleanupExpiredPlans(context.Context) (int64, error)
}

// RuntimePolicy controls timing and audit-reporting policy without accepting
// governance services. Framework assembly resolves those services from the
// canonical Module Host capabilities.
type RuntimePolicy struct {
	// Clock may be called concurrently by Runtime methods. It must be safe for
	// concurrent use and return promptly.
	Clock   func() time.Time
	PlanTTL time.Duration
	// AuditTimeout bounds detached Audit persistence and each AuditFailure
	// notification independently.
	AuditTimeout time.Duration
	// AuditFailure reports an Audit persistence failure without replacing the
	// primary Runtime result. It may be called concurrently. The callback must be
	// safe for concurrent use, honor the supplied context, and return promptly
	// after cancellation. The Event is a defensive copy.
	AuditFailure func(context.Context, error, audit.Event)
}
