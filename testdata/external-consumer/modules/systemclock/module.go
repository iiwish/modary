// Package systemclock provides the Counter application's custom clock
// capability independently of the feature that consumes it.
package systemclock

import (
	"context"
	"time"

	"example.com/modary-counter-consumer/modules/clockcontract"
	"github.com/iiwish/modary/module"
)

const ModuleID = "system-clock"

type clock struct{}

func (clock) Now() time.Time { return time.Now() }

// Module returns the custom capability provider Registration.
func Module() module.Registration {
	return module.Register(
		module.Manifest{
			SchemaVersion: module.SchemaVersion,
			ID:            ModuleID,
			Version:       "0.1.0",
			Type:          module.ModuleTypeAdapter,
			Provides:      []module.Capability{clockcontract.Capability},
		},
		func(_ context.Context, scope module.Scope) error {
			return module.Provide[clockcontract.Clock](scope, clockcontract.Key, clock{})
		},
	)
}
