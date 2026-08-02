package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/task"
)

const defaultTestURL = "postgres://modary:modary-test-password@127.0.0.1:55432/modary_test?sslmode=disable"

var schemaSequence atomic.Uint64

type testServices struct {
	host         *module.Host
	db           *sql.DB
	control      databasecontrol.Control
	access       database.Access
	plans        actionpersistence.PlanStore
	idempotency  actionpersistence.IdempotencyStore
	transactions runtimecontrol.TransactionManager
	tasks        task.Service
	options      Options
}

func TestModuleIsExplicitAndSideEffectFree(t *testing.T) {
	registration, err := Module(Options{URL: defaultTestURL, ApplicationSchema: "side_effect_free", QueueSchema: "side_effect_free_queue"})
	if err != nil {
		t.Fatal(err)
	}
	manifest := registration.Definition.Manifest
	if manifest.ID != ModuleID || manifest.Type != module.ModuleTypeAdapter || !reflect.DeepEqual(manifest.Provides, []module.Capability{module.CapabilityDatabase, module.CapabilityTasks}) {
		t.Fatalf("manifest = %#v", manifest)
	}
	if len(registration.Definition.Migrations) != 1 || registration.Definition.Migrations[0].Driver != "postgres" {
		t.Fatalf("migrations = %#v", registration.Definition.Migrations)
	}
}

func TestModuleRejectsInvalidOptionsBeforeSideEffects(t *testing.T) {
	tests := []Options{
		{},
		{URL: " not-a-url"},
		{URL: defaultTestURL, ApplicationSchema: "bad-name"},
		{URL: defaultTestURL, ApplicationSchema: "public"},
		{URL: defaultTestURL, QueueSchema: "pg_queue"},
		{URL: defaultTestURL, ApplicationSchema: "same", QueueSchema: "same"},
		{URL: defaultTestURL, MaxOpenConnections: -1},
		{URL: defaultTestURL, MaxOpenConnections: 1, MaxIdleConnections: 2},
	}
	for _, options := range tests {
		if _, err := Module(options); err == nil {
			t.Fatalf("Module(%#v) accepted invalid options", options)
		}
	}
}

func TestModuleDoesNotExposeInvalidConnectionCredentials(t *testing.T) {
	secret := "postgres-secret-value"
	_, err := Module(Options{URL: "postgres://user:" + secret + "@%/database"})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("Module() error = %v", err)
	}
}

func TestStartupConnectionFailureDoesNotExposeCredentials(t *testing.T) {
	secret := "runtime-postgres-secret-value"
	registration, err := Module(Options{
		URL:               "postgres://modary_secret_error_user:" + secret + "@127.0.0.1:55432/modary_test?sslmode=disable",
		ApplicationSchema: "secret_error_test",
		QueueSchema:       "secret_error_test_queue",
	})
	if err != nil {
		t.Fatal(err)
	}
	host := module.NewHost()
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	err = host.Start(context.Background())
	if err == nil {
		_ = host.Shutdown(context.Background())
		t.Fatal("PostgreSQL startup unexpectedly accepted the invalid credential")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("PostgreSQL startup error exposed credential: %v", err)
	}
}

type typedNilHandler struct{}

func (*typedNilHandler) Handle(context.Context, task.Job) error { return nil }

func TestTaskRunnerRejectsTypedNilHandlerAndClosesBeforeStart(t *testing.T) {
	services := startTestServices(t)
	var handler *typedNilHandler
	if _, err := services.tasks.NewRunner(handler, task.RunnerOptions{}); err == nil {
		t.Fatal("typed-nil task handler was accepted")
	}
	runner, err := services.tasks.NewRunner(task.HandlerFunc(func(context.Context, task.Job) error { return nil }), task.RunnerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.Stopped():
	default:
		t.Fatal("runner stopped before start without closing Stopped")
	}
	if err := runner.Start(context.Background()); err == nil {
		t.Fatal("stopped runner was restarted")
	}
}

func TestPostgresModuleCreatesIsolatedSchemas(t *testing.T) {
	services := startTestServices(t)
	for _, table := range []string{"modary_action_plan", "modary_action_idempotency", "modary_module_migration"} {
		var exists bool
		if err := services.db.QueryRowContext(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, services.options.ApplicationSchema+"."+table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("application table %s is absent", table)
		}
	}
	var queueExists bool
	if err := services.db.QueryRowContext(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, services.options.QueueSchema+".river_job").Scan(&queueExists); err != nil {
		t.Fatal(err)
	}
	if !queueExists {
		t.Fatal("River queue schema was not migrated")
	}
}

func TestSingleConnectionPoolCanBootstrapTheDurableProfile(t *testing.T) {
	options := newTestOptions(t)
	options.MaxOpenConnections = 1
	options.MaxIdleConnections = 1
	services := startTestServicesWithOptions(t, options)
	var boundRole, boundPeer string
	if err := services.db.QueryRowContext(context.Background(), `SELECT schema_role, peer_schema FROM modary_schema_profile WHERE profile_id = 1`).Scan(&boundRole, &boundPeer); err != nil {
		t.Fatal(err)
	}
	if boundRole != schemaRoleApplication || boundPeer != options.QueueSchema {
		t.Fatalf("application schema profile binding = %q, %q; want application, %q", boundRole, boundPeer, options.QueueSchema)
	}
}

func TestConcurrentHostsSerializeSchemaAndModuleMigrations(t *testing.T) {
	options := newTestOptions(t)
	hosts := []*module.Host{module.NewHost(), module.NewHost()}
	t.Cleanup(func() {
		for _, host := range hosts {
			if host.State() == module.StateRunning {
				_ = host.Shutdown(context.Background())
			}
		}
		admin, err := sql.Open("pgx", options.URL)
		if err == nil {
			_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoteIdentifier(options.QueueSchema) + ` CASCADE`)
			_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoteIdentifier(options.ApplicationSchema) + ` CASCADE`)
			_ = admin.Close()
		}
	})
	for _, host := range hosts {
		registration, err := Module(options)
		if err != nil {
			t.Fatal(err)
		}
		if err := host.Register(registration); err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	errorsByHost := make(chan error, len(hosts))
	var wait sync.WaitGroup
	for _, host := range hosts {
		wait.Add(1)
		go func(host *module.Host) {
			defer wait.Done()
			<-start
			errorsByHost <- host.Start(context.Background())
		}(host)
	}
	close(start)
	wait.Wait()
	close(errorsByHost)
	for err := range errorsByHost {
		if err != nil {
			t.Fatalf("concurrent PostgreSQL startup: %v", err)
		}
	}
	for _, host := range hosts {
		if err := host.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown concurrent PostgreSQL host: %v", err)
		}
	}

	admin, err := sql.Open("pgx", options.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var migrations int
	query := `SELECT COUNT(*) FROM ` + quoteIdentifier(options.ApplicationSchema) + `.modary_module_migration WHERE module_id = $1`
	if err := admin.QueryRowContext(context.Background(), query, ModuleID).Scan(&migrations); err != nil {
		t.Fatal(err)
	}
	if migrations != 1 {
		t.Fatalf("PostgreSQL module migrations = %d, want 1", migrations)
	}
}

func TestQueueSchemaCannotBeSharedAcrossApplicationProfiles(t *testing.T) {
	first := newTestOptions(t)
	second := first
	second.ApplicationSchema += "_other"
	hosts := []*module.Host{module.NewHost(), module.NewHost()}
	t.Cleanup(func() {
		for _, host := range hosts {
			if host.State() == module.StateRunning {
				_ = host.Shutdown(context.Background())
			}
		}
		admin, err := sql.Open("pgx", first.URL)
		if err == nil {
			_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoteIdentifier(first.QueueSchema) + ` CASCADE`)
			_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoteIdentifier(first.ApplicationSchema) + ` CASCADE`)
			_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoteIdentifier(second.ApplicationSchema) + ` CASCADE`)
			_ = admin.Close()
		}
	})
	for index, options := range []Options{first, second} {
		registration, err := Module(options)
		if err != nil {
			t.Fatal(err)
		}
		if err := hosts[index].Register(registration); err != nil {
			t.Fatal(err)
		}
	}
	if err := hosts[0].Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := hosts[1].Start(context.Background()); err == nil {
		t.Fatal("shared queue schema was accepted by a different application profile")
	}

	admin, err := sql.Open("pgx", first.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var boundRole, boundPeer string
	query := `SELECT schema_role, peer_schema FROM ` + quoteIdentifier(first.QueueSchema) + `.modary_schema_profile WHERE profile_id = 1`
	if err := admin.QueryRowContext(context.Background(), query).Scan(&boundRole, &boundPeer); err != nil {
		t.Fatal(err)
	}
	if boundRole != schemaRoleQueue || boundPeer != first.ApplicationSchema {
		t.Fatalf("queue schema profile binding = %q, %q; want queue, %q", boundRole, boundPeer, first.ApplicationSchema)
	}
}

func TestSchemaRoleCannotBeReusedAcrossProfiles(t *testing.T) {
	tests := []struct {
		name   string
		second func(Options) Options
	}{
		{
			name: "queue becomes application",
			second: func(first Options) Options {
				return Options{
					URL:               first.URL,
					ApplicationSchema: first.QueueSchema,
					QueueSchema:       first.QueueSchema + "_other",
				}
			},
		},
		{
			name: "application becomes queue",
			second: func(first Options) Options {
				return Options{
					URL:               first.URL,
					ApplicationSchema: first.ApplicationSchema + "_other",
					QueueSchema:       first.ApplicationSchema,
				}
			},
		},
		{
			name: "roles are swapped",
			second: func(first Options) Options {
				return Options{
					URL:               first.URL,
					ApplicationSchema: first.QueueSchema,
					QueueSchema:       first.ApplicationSchema,
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := newTestOptions(t)
			second := test.second(first)
			hosts := []*module.Host{module.NewHost(), module.NewHost()}
			cleanupProfileSchemas(t, first.URL, hosts, first.ApplicationSchema, first.QueueSchema, second.ApplicationSchema, second.QueueSchema)
			for index, options := range []Options{first, second} {
				registration, err := Module(options)
				if err != nil {
					t.Fatal(err)
				}
				if err := hosts[index].Register(registration); err != nil {
					t.Fatal(err)
				}
			}
			if err := hosts[0].Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := hosts[1].Start(context.Background()); err == nil {
				t.Fatal("schema role reuse was accepted by a different profile")
			}
		})
	}
}

func TestConcurrentSwappedSchemaProfilesHaveOneWinner(t *testing.T) {
	first := newTestOptions(t)
	second := Options{
		URL:               first.URL,
		ApplicationSchema: first.QueueSchema,
		QueueSchema:       first.ApplicationSchema,
	}
	hosts := []*module.Host{module.NewHost(), module.NewHost()}
	cleanupProfileSchemas(t, first.URL, hosts, first.ApplicationSchema, first.QueueSchema)
	for index, options := range []Options{first, second} {
		registration, err := Module(options)
		if err != nil {
			t.Fatal(err)
		}
		if err := hosts[index].Register(registration); err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	errorsByHost := make(chan error, len(hosts))
	var wait sync.WaitGroup
	for _, host := range hosts {
		wait.Add(1)
		go func(host *module.Host) {
			defer wait.Done()
			<-start
			errorsByHost <- host.Start(context.Background())
		}(host)
	}
	close(start)
	wait.Wait()
	close(errorsByHost)

	var succeeded, rejected int
	for err := range errorsByHost {
		if err == nil {
			succeeded++
		} else {
			rejected++
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent swapped profiles = %d succeeded, %d rejected; want 1 and 1", succeeded, rejected)
	}
}

func cleanupProfileSchemas(t *testing.T, url string, hosts []*module.Host, schemas ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, host := range hosts {
			if host.State() == module.StateRunning {
				_ = host.Shutdown(context.Background())
			}
		}
		admin, err := sql.Open("pgx", url)
		if err != nil {
			return
		}
		defer admin.Close()
		seen := make(map[string]struct{}, len(schemas))
		for _, schema := range schemas {
			if _, duplicate := seen[schema]; duplicate {
				continue
			}
			seen[schema] = struct{}{}
			_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoteIdentifier(schema) + ` CASCADE`)
		}
	})
}

func TestTaskEnqueueSharesGovernedTransaction(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	if _, err := services.db.ExecContext(ctx, `CREATE TABLE transaction_probe (value TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	rollbackCause := errors.New("roll back task and row")
	err := services.control.WithinTransaction(ctx, func(txCtx context.Context) error {
		executor, err := services.control.Executor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(txCtx, `INSERT INTO transaction_probe(value) VALUES ($1)`, "rolled-back"); err != nil {
			return err
		}
		if _, err := services.tasks.Enqueue(txCtx, task.Request{Kind: "probe.run", Payload: []byte(`{"value":"rolled-back"}`), UniqueKey: "rolled-back"}); err != nil {
			return err
		}
		return rollbackCause
	})
	if !errors.Is(err, rollbackCause) {
		t.Fatalf("rollback error = %v", err)
	}
	assertCounts(t, services, 0, 0)

	err = services.control.WithinTransaction(ctx, func(txCtx context.Context) error {
		executor, err := services.control.Executor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(txCtx, `INSERT INTO transaction_probe(value) VALUES ($1)`, "committed"); err != nil {
			return err
		}
		receipt, err := services.tasks.Enqueue(txCtx, task.Request{Kind: "probe.run", Payload: []byte(`{"value":"committed"}`), UniqueKey: "committed"})
		if err != nil {
			return err
		}
		if receipt.ID == 0 || receipt.DuplicateSuppressed {
			return fmt.Errorf("unexpected receipt %#v", receipt)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, services, 1, 1)
	if _, err := services.tasks.Enqueue(ctx, task.Request{Kind: "probe.run"}); !errors.Is(err, task.ErrTransactionRequired) {
		t.Fatalf("enqueue outside transaction error = %v", err)
	}
}

func TestTaskUniqueKeySuppressesOnlyTheSameLogicalActiveTask(t *testing.T) {
	services := startTestServices(t)
	var receipts []task.Receipt
	err := services.control.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		requests := []task.Request{
			{Kind: "probe.execute", UniqueKey: "run-one", Payload: json.RawMessage(`{"value":1}`)},
			{Kind: "probe.execute", UniqueKey: "run-one", Payload: json.RawMessage(`{"value":2}`)},
			{Kind: "probe.reconcile", UniqueKey: "run-one", Payload: json.RawMessage(`{"value":3}`)},
			{Kind: "probe.execute", UniqueKey: "run-two", Payload: json.RawMessage(`{"value":4}`)},
		}
		for _, request := range requests {
			receipt, err := services.tasks.Enqueue(txCtx, request)
			if err != nil {
				return err
			}
			receipts = append(receipts, receipt)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 4 || receipts[0].ID == 0 || receipts[1].ID != receipts[0].ID ||
		!receipts[1].DuplicateSuppressed || receipts[0].DuplicateSuppressed ||
		receipts[2].DuplicateSuppressed || receipts[3].DuplicateSuppressed ||
		receipts[2].ID == receipts[0].ID || receipts[3].ID == receipts[0].ID {
		t.Fatalf("task receipts = %#v", receipts)
	}
	var jobs int
	query := `SELECT COUNT(*) FROM ` + quoteIdentifier(services.options.QueueSchema) + `.river_job`
	if err := services.db.QueryRowContext(context.Background(), query).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 3 {
		t.Fatalf("durable jobs = %d, want 3", jobs)
	}
}

func TestTaskRunnerWorksCommittedJob(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	worked := make(chan task.Job, 1)
	runner, err := services.tasks.NewRunner(task.HandlerFunc(func(_ context.Context, job task.Job) error {
		worked <- job
		return nil
	}), task.RunnerOptions{Queues: []task.Queue{{Name: task.DefaultQueue, MaxWorkers: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runner.Stop(stopCtx)
	})
	if err := services.control.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := services.tasks.Enqueue(txCtx, task.Request{Kind: "probe.execute", Payload: []byte(`{"id":"one"}`), UniqueKey: "one"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case job := <-worked:
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if job.Kind != "probe.execute" || job.Attempt != 1 || job.MaxAttempts != task.DefaultMaxAttempts || payload.ID != "one" {
			t.Fatalf("job = %#v", job)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("task was not worked")
	}
}

func TestTaskRunnerUsesConfiguredRetryDelays(t *testing.T) {
	services := startTestServices(t)
	ctx := context.Background()
	attempts := make(chan int, 2)
	runner, err := services.tasks.NewRunner(task.HandlerFunc(func(_ context.Context, job task.Job) error {
		attempts <- job.Attempt
		if job.Attempt == 1 {
			return errors.New("retry")
		}
		return nil
	}), task.RunnerOptions{
		Queues:      []task.Queue{{Name: task.DefaultQueue, MaxWorkers: 1}},
		RetryDelays: []time.Duration{time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runner.Stop(stopCtx)
	})
	if err := services.control.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := services.tasks.Enqueue(txCtx, task.Request{Kind: "probe.retry", MaxAttempts: 2})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{1, 2} {
		select {
		case got := <-attempts:
			if got != want {
				t.Fatalf("attempt = %d, want %d", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("attempt %d was not worked", want)
		}
	}
}

func assertCounts(t *testing.T, services testServices, domainRows, jobs int) {
	t.Helper()
	var gotRows, gotJobs int
	if err := services.db.QueryRow(`SELECT COUNT(*) FROM transaction_probe`).Scan(&gotRows); err != nil {
		t.Fatal(err)
	}
	query := `SELECT COUNT(*) FROM ` + quoteIdentifier(services.options.QueueSchema) + `.river_job`
	if err := services.db.QueryRow(query).Scan(&gotJobs); err != nil {
		t.Fatal(err)
	}
	if gotRows != domainRows || gotJobs != jobs {
		t.Fatalf("counts = rows %d jobs %d, want %d %d", gotRows, gotJobs, domainRows, jobs)
	}
}

func startTestServices(t *testing.T) testServices {
	t.Helper()
	return startTestServicesWithOptions(t, newTestOptions(t))
}

func newTestOptions(t *testing.T) Options {
	t.Helper()
	url := os.Getenv("MODARY_TEST_DATABASE_URL")
	if url == "" {
		url = defaultTestURL
	}
	suffix := fmt.Sprintf("%d_%d", time.Now().UnixNano(), schemaSequence.Add(1))
	return Options{URL: url, ApplicationSchema: "modary_test_" + suffix, QueueSchema: "modary_queue_" + suffix}
}

func startTestServicesWithOptions(t *testing.T, options Options) testServices {
	t.Helper()
	registration, err := Module(options)
	if err != nil {
		t.Fatal(err)
	}
	services := testServices{options: options}
	originalStart := registration.Start
	registration.Start = func(ctx context.Context, scope module.Scope) error {
		if err := originalStart(ctx, scope); err != nil {
			return err
		}
		control, err := moduleassembly.ResolveDatabaseControl(scope)
		if err != nil {
			return err
		}
		tasks, err := module.Resolve(scope, module.Tasks())
		if err != nil {
			return err
		}
		executor, err := control.Executor(ctx)
		if err != nil {
			return err
		}
		postgresExecutor, ok := executor.(sqlExecutor)
		if !ok {
			return fmt.Errorf("database executor is not PostgreSQL")
		}
		db, ok := postgresExecutor.runner.(*sql.DB)
		if !ok {
			return fmt.Errorf("PostgreSQL executor does not use sql.DB")
		}
		services.control, services.tasks, services.db = control, tasks, db
		services.access = control.Access()
		services.plans = &planStore{control: control}
		services.idempotency = &idempotencyStore{control: control}
		services.transactions = &transactionManager{control: control}
		return nil
	}
	host := module.NewHost()
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			t.Skipf("PostgreSQL integration service unavailable: %v", err)
		}
		t.Fatal(err)
	}
	services.host = host
	url := options.URL
	t.Cleanup(func() {
		if host.State() == module.StateRunning {
			_ = host.Shutdown(context.Background())
		}
		admin, err := sql.Open("pgx", url)
		if err == nil {
			_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoteIdentifier(options.QueueSchema) + ` CASCADE`)
			_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoteIdentifier(options.ApplicationSchema) + ` CASCADE`)
			_ = admin.Close()
		}
	})
	if services.db == nil || services.control == nil || services.tasks == nil {
		t.Fatal("PostgreSQL test services were not captured")
	}
	return services
}
