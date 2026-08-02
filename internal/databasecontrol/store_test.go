package databasecontrol

import (
	"context"
	"errors"
	"testing"

	"github.com/iiwish/modary/database"
)

func TestStoreOwnsOrdinaryTransactionAuthority(t *testing.T) {
	db := openPostgresTestDB(t)
	if _, err := db.Exec(`CREATE TABLE business_record (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	control, err := New(&testBackend{db: db})
	if err != nil {
		t.Fatal(err)
	}
	store := control.Store()
	if store == nil {
		t.Fatal("Store() is nil")
	}
	if _, err := store.ExecContext(context.Background(), `INSERT INTO business_record(id, name) VALUES (1, 'outside')`); !errors.Is(err, database.ErrTransactionRequired) {
		t.Fatalf("outside transaction ExecContext() error = %v", err)
	}
	if err := store.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		_, err := store.ExecContext(txCtx, `INSERT INTO business_record(id, name) VALUES ($1, $2)`, 1, "inside")
		return err
	}); err != nil {
		t.Fatalf("WithinTransaction() error = %v", err)
	}
	var name string
	if err := store.QueryRowContext(context.Background(), `SELECT name FROM business_record WHERE id = $1`, 1).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "inside" {
		t.Fatalf("name = %q", name)
	}

	rollback := errors.New("rollback")
	if err := store.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		if _, err := store.ExecContext(txCtx, `INSERT INTO business_record(id, name) VALUES ($1, $2)`, 2, "rolled-back"); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("rollback error = %v", err)
	}
	var count int
	if err := store.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM business_record WHERE id = 2`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back rows = %d", count)
	}
}

var _ database.Store = (*store)(nil)
