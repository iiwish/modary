package testcomponent

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/internal/testsupport"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/task"
)

// RuntimeModule installs non-durable database and Action persistence services
// for public-boundary tests without pulling a database driver into Core.
func RuntimeModule(withTasks bool) module.Registration {
	provides := []module.Capability{module.CapabilityDatabase}
	if withTasks {
		provides = append(provides, module.CapabilityTasks)
	}
	return module.Register(module.Manifest{
		SchemaVersion: module.SchemaVersion,
		ID:            "test-runtime",
		Version:       "1.0.0",
		Type:          module.ModuleTypeAdapter,
		Provides:      provides,
	}, func(_ context.Context, installation module.Scope) error {
		control, err := databasecontrol.New(directBackend{})
		if err != nil {
			return err
		}
		if err := moduleassembly.ProvideDatabase(installation, control); err != nil {
			return err
		}
		if err := moduleassembly.ProvideActionPersistence(
			installation,
			testsupport.NewMemoryPlanStore(),
			testsupport.NewMemoryIdempotencyStore(),
			testsupport.DirectTransactions{},
		); err != nil {
			return err
		}
		if withTasks {
			tasks := &memoryTasks{runners: make(map[*memoryRunner]struct{})}
			if err := module.OnStop(installation, tasks.close); err != nil {
				return err
			}
			return module.Provide[task.Service](installation, module.Tasks(), tasks)
		}
		return nil
	})
}

type directBackend struct{}

func (directBackend) Driver() string { return "test" }

func (directBackend) ValidateMigration(string) error { return nil }

func (directBackend) ReadExecutor(context.Context) (database.Executor, error) {
	return rejectingExecutor{}, nil
}

func (directBackend) WriteExecutor(context.Context) (database.Executor, error) {
	return rejectingExecutor{}, nil
}

func (directBackend) AdminExecutor(context.Context) (database.Executor, error) {
	return rejectingExecutor{}, nil
}

func (directBackend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

type rejectingExecutor struct{}

func (rejectingExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("test database does not execute SQL")
}

func (rejectingExecutor) QueryContext(context.Context, string, ...any) (database.Rows, error) {
	return nil, errors.New("test database does not execute SQL")
}

func (rejectingExecutor) QueryRowContext(context.Context, string, ...any) database.Row {
	return errorRow{err: errors.New("test database does not execute SQL")}
}

type errorRow struct{ err error }

func (row errorRow) Scan(...any) error { return row.err }

type memoryTasks struct {
	mu      sync.Mutex
	runners map[*memoryRunner]struct{}
}

func (*memoryTasks) Enqueue(_ context.Context, request task.Request) (task.Receipt, error) {
	if _, err := task.NormalizeRequest(request); err != nil {
		return task.Receipt{}, err
	}
	return task.Receipt{ID: 1}, nil
}

func (tasks *memoryTasks) NewRunner(handler task.Handler, options task.RunnerOptions) (task.Runner, error) {
	if handler == nil {
		return nil, errors.New("task handler is required")
	}
	if _, err := task.NormalizeRunnerOptions(options); err != nil {
		return nil, err
	}
	runner := &memoryRunner{stopped: make(chan struct{})}
	tasks.mu.Lock()
	tasks.runners[runner] = struct{}{}
	tasks.mu.Unlock()
	return runner, nil
}

func (tasks *memoryTasks) close(ctx context.Context) error {
	tasks.mu.Lock()
	runners := make([]*memoryRunner, 0, len(tasks.runners))
	for runner := range tasks.runners {
		runners = append(runners, runner)
	}
	tasks.mu.Unlock()
	for _, runner := range runners {
		if err := runner.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

type memoryRunner struct {
	mu      sync.Mutex
	stopped chan struct{}
	closed  bool
}

func (*memoryRunner) Start(context.Context) error { return nil }

func (runner *memoryRunner) Stop(context.Context) error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if !runner.closed {
		close(runner.stopped)
		runner.closed = true
	}
	return nil
}

func (runner *memoryRunner) Stopped() <-chan struct{} { return runner.stopped }
