package module

import (
	"context"
	"io/fs"
	"regexp"

	"github.com/iiwish/modary/action"
)

var migrationDriverPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,62}$`)

// Definition is the complete, side-effect-free declaration of a Module.
// Inspecting a Definition never starts resources, opens migration sources, or
// constructs Action handlers.
type Definition struct {
	Manifest   Manifest          `json:"manifest"`
	Actions    []ActionBinding   `json:"actions,omitempty"`
	Migrations []MigrationSource `json:"migrations,omitempty"`
}

// StartFunc initializes one Module synchronously during Host startup. It is
// invoked at most once, must honor context cancellation and deadlines, and must
// not retain Scope after returning. Scope operations are concurrency-safe while
// the callback is active, but the callback must join every goroutine using Scope
// before it returns. Services and cleanup callbacks derived during startup must
// themselves satisfy their documented lifetime and concurrency contracts.
type StartFunc func(context.Context, Scope) error

// Cleanup releases one process resource registered during Module startup. It
// must honor context cancellation, return promptly after cancellation, and stop
// using dependent services before returning. A timed-out callback may continue
// concurrently with later cleanup callbacks, so trusted implementations must
// never ignore the supplied context and must synchronize any shared cleanup
// state.
type Cleanup func(context.Context) error

// HandlerFactory constructs one governed Action handler from a sealed,
// read-only service view. It is invoked synchronously at most once during
// startup and must honor context cancellation. Resolver is valid only for the
// duration of the call: resolve and retain the resulting service values, never
// retain Resolver or use it from another goroutine. It cannot publish services
// or register cleanup.
type HandlerFactory func(context.Context, Resolver) (action.Handler, error)

// ActionBinding pairs an inspectable Action descriptor with its runtime factory.
type ActionBinding struct {
	Descriptor action.Descriptor `json:"descriptor"`
	NewHandler HandlerFactory    `json:"-"`
}

// MigrationSource declares one driver-specific, forward-only migration set.
// Files is rooted at the directory containing the migration files. Open(".")
// must return a non-nil fs.ReadDirFile that follows the positive-size ReadDir
// batching and end-of-directory contract. Modary bounds directory entries,
// individual files, and the aggregate source before SQL validation or database
// effects. Definition validation only checks the declaration and never opens it.
type MigrationSource struct {
	Driver string `json:"driver"`
	Files  fs.FS  `json:"-"`
}

// Registration combines a pure Definition with its optional runtime startup.
type Registration struct {
	Definition Definition
	Start      StartFunc
}

// Register constructs a Registration from a manifest, startup callback, and
// Action bindings.
func Register(manifest Manifest, start StartFunc, actions ...ActionBinding) Registration {
	return Registration{
		Definition: Definition{
			Manifest: manifest,
			Actions:  append([]ActionBinding(nil), actions...),
		},
		Start: start,
	}
}

// OnStop registers process-resource cleanup for the current Module. Callback
// invocation starts LIFO within a Module and in reverse dependency order across
// Modules. When a callback times out, its goroutine may overlap later callbacks
// and provider cleanup; this is not a completion-order guarantee.
func OnStop(scope Scope, cleanup Cleanup) error {
	if cleanup == nil {
		return ErrNilCleanup
	}
	if isNilValue(scope) {
		return ErrInvalidScope
	}
	internal, ok := scope.(*installScope)
	if !ok {
		return ErrInvalidScope
	}
	return internal.onStop(cleanup)
}
