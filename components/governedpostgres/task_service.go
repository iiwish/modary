package governedpostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/task"
)

const riverEnvelopeKind = "modary_task_v1"

const forcedRunnerStopTimeout = 5 * time.Second

type taskEnvelope struct {
	TaskKind  string          `json:"kind" river:"unique"`
	Payload   json.RawMessage `json:"payload"`
	UniqueKey string          `json:"unique_key,omitempty" river:"unique"`
}

func (taskEnvelope) Kind() string { return riverEnvelopeKind }

type taskService struct {
	db          *sql.DB
	backend     *backend
	queueSchema string
	insert      *river.Client[*sql.Tx]

	mu      sync.Mutex
	closed  bool
	runners map[*runner]struct{}
}

func newTaskService(db *sql.DB, backend *backend, queueSchema string) (*taskService, error) {
	insert, err := river.NewClient(riverdatabasesql.New(db), &river.Config{
		Schema:              queueSchema,
		SkipUnknownJobCheck: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create River insert client: %w", err)
	}
	return &taskService{db: db, backend: backend, queueSchema: queueSchema, insert: insert, runners: make(map[*runner]struct{})}, nil
}

func (service *taskService) Enqueue(ctx context.Context, request task.Request) (task.Receipt, error) {
	if ctx == nil {
		return task.Receipt{}, fmt.Errorf("task enqueue context is required")
	}
	if service == nil || service.backend == nil || service.insert == nil {
		return task.Receipt{}, task.ErrUnavailable
	}
	service.mu.Lock()
	closed := service.closed
	service.mu.Unlock()
	if closed {
		return task.Receipt{}, task.ErrUnavailable
	}
	normalized, err := task.NormalizeRequest(request)
	if err != nil {
		return task.Receipt{}, err
	}
	tx, err := service.backend.transaction(ctx)
	if errors.Is(err, database.ErrTransactionRequired) {
		return task.Receipt{}, task.ErrTransactionRequired
	}
	if err != nil {
		return task.Receipt{}, fmt.Errorf("resolve task transaction: %w", err)
	}
	opts := &river.InsertOpts{
		Queue: normalized.Queue, MaxAttempts: normalized.MaxAttempts,
		ScheduledAt: normalized.ScheduledAt,
	}
	if normalized.UniqueKey != "" {
		opts.UniqueOpts = river.UniqueOpts{ByArgs: true}
	}
	result, err := service.insert.InsertTx(ctx, tx, taskEnvelope{
		TaskKind: normalized.Kind, Payload: normalized.Payload, UniqueKey: normalized.UniqueKey,
	}, opts)
	if err != nil {
		return task.Receipt{}, fmt.Errorf("insert durable task: %w", err)
	}
	if result == nil || result.Job == nil {
		return task.Receipt{}, fmt.Errorf("insert durable task returned no job")
	}
	return task.Receipt{ID: result.Job.ID, DuplicateSuppressed: result.UniqueSkippedAsDuplicate}, nil
}

func (service *taskService) NewRunner(handler task.Handler, options task.RunnerOptions) (task.Runner, error) {
	if service == nil || service.db == nil {
		return nil, task.ErrUnavailable
	}
	if isNilHandler(handler) {
		return nil, fmt.Errorf("task handler is required")
	}
	normalized, err := task.NormalizeRunnerOptions(options)
	if err != nil {
		return nil, err
	}
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &envelopeWorker{handler: handler}); err != nil {
		return nil, fmt.Errorf("register task worker: %w", err)
	}
	queues := make(map[string]river.QueueConfig, len(normalized.Queues))
	for _, queue := range normalized.Queues {
		queues[queue.Name] = river.QueueConfig{MaxWorkers: queue.MaxWorkers}
	}
	config := &river.Config{
		Schema:          service.queueSchema,
		Workers:         workers,
		Queues:          queues,
		JobTimeout:      normalized.JobTimeout,
		SoftStopTimeout: normalized.SoftStopTimeout,
	}
	if len(normalized.RetryDelays) != 0 {
		config.RetryPolicy = &retryPolicy{delays: append([]time.Duration(nil), normalized.RetryDelays...)}
	}
	client, err := river.NewClient(riverdatabasesql.New(service.db), config)
	if err != nil {
		return nil, fmt.Errorf("create River task runner: %w", err)
	}
	run := &runner{client: client, owner: service, done: make(chan struct{})}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return nil, task.ErrUnavailable
	}
	service.runners[run] = struct{}{}
	return run, nil
}

type retryPolicy struct {
	delays []time.Duration
}

func (policy *retryPolicy) NextRetry(job *rivertype.JobRow) time.Time {
	base := time.Now().UTC()
	if policy == nil || len(policy.delays) == 0 {
		return base
	}
	index := 0
	if job != nil && job.Attempt > 1 {
		index = job.Attempt - 1
	}
	if index >= len(policy.delays) {
		index = len(policy.delays) - 1
	}
	return base.Add(policy.delays[index])
}

func (service *taskService) close(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("task service close context is required")
	}
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return nil
	}
	service.closed = true
	runners := make([]*runner, 0, len(service.runners))
	for run := range service.runners {
		runners = append(runners, run)
	}
	service.mu.Unlock()
	var joined error
	for _, run := range runners {
		if err := run.Stop(ctx); err != nil {
			joined = errors.Join(joined, err)
			forceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forcedRunnerStopTimeout)
			joined = errors.Join(joined, run.stop(forceCtx, true))
			cancel()
		}
	}
	return joined
}

func (service *taskService) release(run *runner) {
	if service == nil {
		return
	}
	service.mu.Lock()
	delete(service.runners, run)
	service.mu.Unlock()
}

type envelopeWorker struct {
	river.WorkerDefaults[taskEnvelope]
	handler task.Handler
}

func (worker *envelopeWorker) Work(ctx context.Context, job *river.Job[taskEnvelope]) error {
	if worker == nil || worker.handler == nil || job == nil || job.JobRow == nil {
		return fmt.Errorf("task worker received an invalid job")
	}
	payload := append(json.RawMessage(nil), job.Args.Payload...)
	return worker.handler.Handle(ctx, task.Job{
		ID: job.ID, Kind: job.Args.TaskKind, Payload: payload, Queue: job.Queue,
		Attempt: job.Attempt, MaxAttempts: job.MaxAttempts,
	})
}

type runner struct {
	client *river.Client[*sql.Tx]
	owner  *taskService

	mu          sync.Mutex
	started     bool
	stopping    bool
	stopped     bool
	stopAttempt chan struct{}
	done        chan struct{}
}

func (run *runner) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("task runner context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if run == nil || run.client == nil {
		return task.ErrUnavailable
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	if run.started {
		return fmt.Errorf("task runner was already started")
	}
	if run.stopped {
		return fmt.Errorf("task runner was already stopped")
	}
	if err := run.client.Start(ctx); err != nil {
		return fmt.Errorf("start task runner: %w", err)
	}
	run.started = true
	go run.observeStopped()
	return nil
}

func (run *runner) Stop(ctx context.Context) error {
	return run.stop(ctx, false)
}

func (run *runner) stop(ctx context.Context, hard bool) error {
	if ctx == nil {
		return fmt.Errorf("task runner stop context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if run == nil || run.client == nil {
		return nil
	}
	for {
		run.mu.Lock()
		if run.stopped {
			run.mu.Unlock()
			return nil
		}
		if run.stopping {
			attempt := run.stopAttempt
			run.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-attempt:
				continue
			}
		}
		if !run.started {
			released := run.markStoppedLocked()
			run.mu.Unlock()
			if released {
				run.owner.release(run)
			}
			return nil
		}
		run.stopping = true
		run.stopAttempt = make(chan struct{})
		attempt := run.stopAttempt
		run.mu.Unlock()

		var err error
		if hard {
			err = run.client.StopAndCancel(ctx)
		} else {
			err = run.client.Stop(ctx)
		}
		run.mu.Lock()
		run.stopping = false
		close(attempt)
		released := false
		if err == nil {
			released = run.markStoppedLocked()
		}
		run.mu.Unlock()
		if released {
			run.owner.release(run)
		}
		if err != nil {
			return fmt.Errorf("stop task runner: %w", err)
		}
		return nil
	}
}

func (run *runner) Stopped() <-chan struct{} {
	if run == nil || run.done == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return run.done
}

func (run *runner) observeStopped() {
	<-run.client.Stopped()
	run.mu.Lock()
	released := run.markStoppedLocked()
	run.mu.Unlock()
	if released {
		run.owner.release(run)
	}
}

func (run *runner) markStoppedLocked() bool {
	if run.stopped {
		return false
	}
	run.stopped = true
	close(run.done)
	return true
}

func isNilHandler(handler task.Handler) bool {
	if handler == nil {
		return true
	}
	value := reflect.ValueOf(handler)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
