package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	sqliteadapter "github.com/iiwish/modary/adapters/sqlite"
	"github.com/iiwish/modary/module"
)

func TestPublicModuleComposition(t *testing.T) {
	registration, err := sqliteadapter.Module(sqliteadapter.Options{
		Path: filepath.Join(t.TempDir(), "modary.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	host := module.NewHost()
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, exposed := any(host).(module.Resolver); exposed {
		t.Fatal("public Host exposes service resolution")
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
