# Run Durable Background Tasks

This guide adds one transactionally enqueued task and a worker runner. The
Governed PostgreSQL component uses River internally; consumer code imports only
`task`.

This path requires the Governed PostgreSQL component. The API and Admin
Profiles do not expose `task.Service` by default.

## 1. Resolve The Task Service

An assembled application exposes the public service:

```go
tasks := application.Tasks()
if tasks == nil {
    return errors.New("durable task service is unavailable")
}
```

A Module handler factory can resolve the same canonical service with
`module.Resolve(services, module.Tasks())` after declaring
`module.CapabilityTasks`.

## 2. Enqueue Inside A Governed Action

Call `Enqueue` from `Handler.Execute`. Runtime has already opened the governed
transaction, so the job and the handler's control-store writes share one commit.

```go
receipt, err := tasks.Enqueue(ctx, task.Request{
    Kind:        "report.generate",
    Queue:       "reports",
    Payload:     json.RawMessage(`{"report_id":"report-42"}`),
    UniqueKey:   "report-42",
    MaxAttempts: 5,
})
if err != nil {
    return action.Result{}, fmt.Errorf("enqueue report: %w", err)
}
```

Task kinds are stable lowercase identifiers. Queue names are at most 64 bytes
and use lowercase letters, digits, underscores, or hyphens. Payloads contain one
valid JSON value and are limited to 1 MiB. `UniqueKey` is optional and suppresses
an equivalent active kind/key insertion; it is at most 256 bytes and contains
no control or surrounding whitespace.

## 3. Implement An Idempotent Handler

```go
handler := task.HandlerFunc(func(ctx context.Context, job task.Job) error {
    var input struct {
        ReportID string `json:"report_id"`
    }
    if err := json.Unmarshal(job.Payload, &input); err != nil {
        return fmt.Errorf("decode report task: %w", err)
    }

    // Load product state by ReportID. Return nil when it is already complete.
    // Send ReportID as the downstream idempotency key when supported.
    return generateReport(ctx, input.ReportID)
})
```

Returning an error asks River to retry according to its policy until the job is
discarded. `job.TerminalAttempt()` reports whether the current configured
attempt is last, which is useful when product state records terminal failure.
River persists returned error text, so handlers return a stable secret-safe
error rather than a wrapped Connector or infrastructure error.

## 4. Start An Immutable Runner

```go
runner, err := tasks.NewRunner(handler, task.RunnerOptions{
    Queues: []task.Queue{{Name: "reports", MaxWorkers: 8}},
    JobTimeout:      10 * time.Minute,
    SoftStopTimeout: 30 * time.Second,
    RetryDelays:     []time.Duration{time.Second, 10 * time.Second, time.Minute},
})
if err != nil {
    return err
}
if err := runner.Start(ctx); err != nil {
    return err
}
```

Construct a new runner to change queues or concurrency. Do not mutate runner
configuration after startup. An empty retry-delay list uses River's default;
attempts beyond a configured list reuse its final delay. On shutdown, call
`Stop` with a bounded context and
wait on `Stopped` when coordinating additional process resources.

## 5. Inspect Tasks Without Queue Coupling

Operational surfaces use `task.Inspector`, not River tables or River state
names. A page is bounded to 100 items and accepts an exclusive `BeforeID`
cursor plus optional queue and state filters. The public constants are
`StateQueued`, `StatePending`, `StateScheduled`, `StateRunning`,
`StateRetrying`, `StateSucceeded`, `StateFailed`, and `StateCancelled`. Their
wire values are the corresponding lowercase names. Treat this list as the
application contract; the selected queue component owns the mapping from its
internal lifecycle.

Task summaries deliberately omit payloads and persisted backend error text.
Inspection is read-only and does not grant enqueue, retry, cancel, or queue
administration authority.

## 6. Test The Atomic Boundary

Integration tests use a real PostgreSQL instance. Cover both outcomes:

1. insert a domain row and enqueue, then return an error; assert neither exists;
2. repeat and return nil; assert both exist, start a runner, and observe work.

Also test duplicate delivery, handler cancellation, retry-to-terminal behavior,
restart recovery, and multiple workers contending for the same queue.

Read [Persistence And Durable Tasks](../concepts/persistence-and-tasks.md) for the
database boundary and [Deployment](../operations/deployment.md) for production
topology.
