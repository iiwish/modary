package integration_test

import (
	"context"
	"sync"
	"testing"

	governedpostgres "github.com/iiwish/modary/components/governedpostgres"
	ordinarypostgres "github.com/iiwish/modary/components/postgres"
	"github.com/iiwish/modary/integration/internal/testpostgres"
	"github.com/iiwish/modary/module"
)

func TestPostgresComponentsRejectCrossRoleSchemaReuse(t *testing.T) {
	t.Run("governed queue cannot become ordinary application", func(t *testing.T) {
		config := testpostgres.New(t)
		governed := governedPostgresHost(t, config)
		startHost(t, governed)

		ordinary := ordinaryPostgresHost(t, config.URL, config.QueueSchema)
		if err := ordinary.Start(context.Background()); err == nil {
			t.Fatal("governed queue schema was accepted as an ordinary application schema")
		}
	})

	t.Run("ordinary application cannot become governed queue", func(t *testing.T) {
		config := testpostgres.New(t)
		ordinary := ordinaryPostgresHost(t, config.URL, config.QueueSchema)
		startHost(t, ordinary)

		governed := governedPostgresHost(t, config)
		if err := governed.Start(context.Background()); err == nil {
			t.Fatal("ordinary application schema was accepted as a governed queue schema")
		}
	})
}

func TestOrdinaryApplicationSchemaCanUpgradeToGovernedProfile(t *testing.T) {
	config := testpostgres.New(t)
	ordinary := ordinaryPostgresHost(t, config.URL, config.ApplicationSchema)
	startHost(t, ordinary)

	governed := governedPostgresHost(t, config)
	startHost(t, governed)
}

func TestConcurrentCrossComponentSchemaClaimsHaveOneWinner(t *testing.T) {
	config := testpostgres.New(t)
	hosts := []*module.Host{
		ordinaryPostgresHost(t, config.URL, config.QueueSchema),
		governedPostgresHost(t, config),
	}
	start := make(chan struct{})
	results := make(chan error, len(hosts))
	var group sync.WaitGroup
	for _, host := range hosts {
		group.Add(1)
		go func(host *module.Host) {
			defer group.Done()
			<-start
			results <- host.Start(context.Background())
		}(host)
	}
	close(start)
	group.Wait()
	close(results)

	var succeeded, rejected int
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			rejected++
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent cross-component claims = %d succeeded, %d rejected; want 1 and 1", succeeded, rejected)
	}
	for _, host := range hosts {
		if host.State() == module.StateRunning {
			if err := host.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func ordinaryPostgresHost(t *testing.T, url, schema string) *module.Host {
	t.Helper()
	registration, err := ordinarypostgres.Module(ordinarypostgres.Options{URL: url, Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	return registeredHost(t, registration)
}

func governedPostgresHost(t *testing.T, config testpostgres.Config) *module.Host {
	t.Helper()
	registration, err := governedpostgres.Module(governedpostgres.Options{
		URL: config.URL, ApplicationSchema: config.ApplicationSchema, QueueSchema: config.QueueSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	return registeredHost(t, registration)
}

func registeredHost(t *testing.T, registration module.Registration) *module.Host {
	t.Helper()
	host := module.NewHost()
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	return host
}

func startHost(t *testing.T, host *module.Host) {
	t.Helper()
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if host.State() == module.StateRunning {
			if err := host.Shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		}
	})
}
