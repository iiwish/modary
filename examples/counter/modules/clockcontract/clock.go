// Package clockcontract owns the Counter application's custom clock capability.
// Provider and feature Modules share this package-level typed key.
package clockcontract

import (
	"time"

	"github.com/iiwish/modary/module"
)

const (
	// Capability is the consumer-owned capability required by clock users.
	Capability module.Capability = "counter.clock"
)

// Clock supplies timestamps. Implementations must be safe for concurrent use
// and return promptly.
type Clock interface {
	Now() time.Time
}

// Key is the single identity-bearing service key shared by provider and users.
var Key = module.MustKey[Clock]("counter.clock", Capability)
