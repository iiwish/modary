// Package testsupport contains explicitly non-production dependencies used by
// framework tests. It is internal so external applications cannot mistake
// these implementations for durable governance adapters.
package testsupport

import (
	"context"

	"github.com/iiwish/modary/audit"
)

// DiscardAudit is a non-durable test Hook.
type DiscardAudit struct{}

func (DiscardAudit) Record(context.Context, audit.Event) error { return nil }

// DirectTransactions is a non-atomic test transaction manager.
type DirectTransactions struct{}

func (DirectTransactions) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if ctx == nil {
		return context.Canceled
	}
	if operation == nil {
		return nil
	}
	return operation(ctx)
}
