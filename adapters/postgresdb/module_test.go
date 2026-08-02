package postgresdb

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/testpostgres"
	"github.com/iiwish/modary/module"
)

func TestModuleCreatesOnlyApplicationSchema(t *testing.T) {
	config := testpostgres.New(t)
	registration, err := Module(Options{URL: config.URL, Schema: config.ApplicationSchema})
	if err != nil {
		t.Fatal(err)
	}
	application, err := appkit.Start(context.Background(), appkit.Definition{
		Metadata: appkit.Metadata{ID: "database-only", Name: "Database Only", Version: "0.1.0"},
		Modules:  []module.Registration{registration},
	}, appkit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(context.Background())
	store, err := application.Database()
	if err != nil {
		t.Fatal(err)
	}
	if application.Runtime() != nil || application.Tasks() != nil {
		t.Fatal("general database unexpectedly assembled governed runtime or tasks")
	}
	if _, err := store.ExecContext(context.Background(), `INSERT INTO missing_table(id) VALUES (1)`); !errors.Is(err, database.ErrTransactionRequired) {
		t.Fatalf("outside transaction error = %v", err)
	}
	connection, err := pgx.Connect(context.Background(), config.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	for _, item := range []struct {
		schema string
		want   bool
	}{{config.ApplicationSchema, true}, {config.QueueSchema, false}} {
		var exists bool
		if err := connection.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, item.schema).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists != item.want {
			t.Fatalf("schema %s exists=%t want=%t", item.schema, exists, item.want)
		}
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Database(); !errors.Is(err, appkit.ErrApplicationUnavailable) {
		t.Fatalf("Database() after shutdown error=%v", err)
	}
	if err := store.WithinTransaction(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, module.ErrApplicationUnavailable) {
		t.Fatalf("retained Store after shutdown error=%v", err)
	}
}

func TestOptionsRejectUnsafeConfiguration(t *testing.T) {
	for _, options := range []Options{{}, {URL: "secret invalid value"}, {URL: "postgres://localhost/test", Schema: "public"},
		{URL: "postgres://localhost/test", MaxOpenConnections: 1, MaxIdleConnections: 2}} {
		if _, err := Module(options); err == nil {
			t.Fatalf("Module(%#v) accepted", options)
		}
	}
}
