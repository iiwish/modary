package actionruntime

import (
	"context"

	"github.com/iiwish/modary/internal/transactionoutcome"
)

// confirmedTransactionManager is a test double for a transaction owner that
// confirms rollback whenever the callback fails. Contract-violation tests use
// explicit local non-atomic managers instead of a public production surface.
type confirmedTransactionManager struct{}

func (confirmedTransactionManager) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return confirmTestRollback(operation(ctx))
}

func confirmTestRollback(operationErr error) error {
	if operationErr == nil {
		return nil
	}
	return transactionoutcome.RolledBack(operationErr)
}
