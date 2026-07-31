package sqlite

import (
	"context"
	"fmt"

	"github.com/iiwish/modary/internal/databasecontrol"
)

type transactionManager struct {
	control databasecontrol.Control
}

// WithinTransaction runs one Action operation under SQLite control.
func (manager *transactionManager) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if ctx == nil {
		return fmt.Errorf("transaction context is required")
	}
	if manager == nil || manager.control == nil {
		return fmt.Errorf("SQLite transaction database is required")
	}
	if operation == nil {
		return fmt.Errorf("transaction operation is required")
	}
	return manager.control.WithinTransaction(ctx, operation)
}
