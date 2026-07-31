package database_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/iiwish/modary/database"
)

var errFakeAccess = errors.New("fake database access")

type fakeAccess struct{}

func (fakeAccess) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errFakeAccess
}

func (fakeAccess) QueryContext(context.Context, string, ...any) (database.Rows, error) {
	return nil, errFakeAccess
}

func (fakeAccess) QueryRowContext(context.Context, string, ...any) database.Row {
	return fakeRow{}
}

type fakeRow struct{}

func (fakeRow) Scan(...any) error { return errFakeAccess }

func TestConsumerCanImplementAccessForIsolatedTests(t *testing.T) {
	var access database.Access = fakeAccess{}
	if _, err := access.QueryContext(context.Background(), "consumer test query"); !errors.Is(err, errFakeAccess) {
		t.Fatalf("fake Access error = %v", err)
	}
	if err := access.QueryRowContext(context.Background(), "consumer test row").Scan(); !errors.Is(err, errFakeAccess) {
		t.Fatalf("fake Row error = %v", err)
	}
}
