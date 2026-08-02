package databasecontrol

import (
	"context"

	"github.com/iiwish/modary/database"
)

type store struct {
	*access
	control *control
}

func (store *store) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if store == nil || store.control == nil {
		return database.ErrAccessUnavailable
	}
	return store.control.withinTransaction(ctx, operation, false)
}
