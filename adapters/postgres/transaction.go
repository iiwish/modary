package postgres

import (
	"context"
	"fmt"

	"github.com/iiwish/modary/internal/databasecontrol"
)

type transactionManager struct{ control databasecontrol.Control }

func (manager *transactionManager) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if ctx == nil {
		return fmt.Errorf("transaction context is required")
	}
	if manager == nil || manager.control == nil {
		return fmt.Errorf("PostgreSQL transaction database is required")
	}
	if operation == nil {
		return fmt.Errorf("transaction operation is required")
	}
	return manager.control.WithinTransaction(ctx, operation)
}
