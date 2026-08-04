// Package observe defines the dependency-neutral optional observability
// contract used by HTTP composition and process readiness. Implementations live
// in separately selected components.
package observe

import (
	"context"
	"net/http"
)

// Operation is a closed, low-cardinality framework operation name.
type Operation string

const (
	// OperationDatabaseExec identifies a business-data statement execution.
	OperationDatabaseExec Operation = "database.exec"
	// OperationDatabaseQuery identifies a bounded business-data query.
	OperationDatabaseQuery Operation = "database.query"
	// OperationDatabaseTransaction identifies a transaction callback.
	OperationDatabaseTransaction Operation = "database.transaction"
	// OperationTaskEnqueue identifies durable task insertion.
	OperationTaskEnqueue Operation = "task.enqueue"
	// OperationTaskHandle identifies one durable task attempt.
	OperationTaskHandle Operation = "task.handle"
	// OperationTaskInspect identifies a bounded operational task listing.
	OperationTaskInspect Operation = "task.inspect"
)

// Outcome is the bounded terminal result of an Operation.
type Outcome string

const (
	// OutcomeSuccess identifies a completed operation without an error.
	OutcomeSuccess Outcome = "success"
	// OutcomeError identifies a completed operation that returned an error.
	OutcomeError Outcome = "error"
)

// Service records requests and framework operations without accepting raw
// SQL, payloads, credentials, actor identifiers, or product scope values.
// Finish returned by StartOperation must be concurrency-safe and idempotent.
//
// WrapHTTP records one request using only the preflighted method and route
// template. Implementations must not derive metric labels from raw paths,
// queries, request bodies, actors, scopes, or credentials.
type Service interface {
	WrapHTTP(method, routeTemplate string, next http.Handler) http.Handler
	StartOperation(context.Context, Operation) (context.Context, func(Outcome))
	Ready(context.Context) error
}
