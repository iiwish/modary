// Package task defines Modary's bounded durable background-work contract.
//
// The official PostgreSQL adapter implements this contract with River, but
// consumers do not depend on River or PostgreSQL types. Delivery is at least
// once: handlers must use stable job identity and idempotent external effects.
package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxPayloadBytes bounds one serialized task payload.
	MaxPayloadBytes = 1 << 20
	// DefaultQueue is used when Request.Queue is empty.
	DefaultQueue = "default"
	// DefaultMaxAttempts is used when Request.MaxAttempts is zero.
	DefaultMaxAttempts = 3
	// DefaultMaxWorkers is used when Queue.MaxWorkers is zero.
	DefaultMaxWorkers = 10
	// MinimumRetryDelay keeps a custom retry scheduled strictly in the future.
	MinimumRetryDelay = time.Millisecond
	// MaximumRetryDelay bounds declarative per-runner retry configuration.
	MaximumRetryDelay = 30 * 24 * time.Hour
	// DefaultListLimit bounds one operational inspection page by default.
	DefaultListLimit = 50
	// MaxListLimit is the largest operational inspection page.
	MaxListLimit = 100
)

var (
	// ErrUnavailable reports use before installation or after shutdown.
	ErrUnavailable = errors.New("task service is unavailable")
	// ErrTransactionRequired reports enqueue outside a governed Action transaction.
	ErrTransactionRequired = errors.New("task enqueue requires a governed transaction")
	kindPattern            = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,126}$`)
	queuePattern           = regexp.MustCompile(`^[a-z0-9]+(?:[_-]?[a-z0-9]+)*$`)
)

// Request describes one durable task insertion. UniqueKey is optional. When
// present, an equivalent logical kind and key are inserted at most once while
// River's active uniqueness states apply.
type Request struct {
	Kind        string
	Payload     json.RawMessage
	Queue       string
	MaxAttempts int
	ScheduledAt time.Time
	UniqueKey   string
}

// Receipt identifies the durable job selected by an enqueue operation.
type Receipt struct {
	ID                  int64
	DuplicateSuppressed bool
}

// State is the provider-neutral lifecycle state exposed by task inspection.
// Queue implementations map their internal states into this closed contract.
type State string

const (
	// StateQueued is ready for a worker to claim.
	StateQueued State = "queued"
	// StatePending is accepted but not yet eligible to run.
	StatePending State = "pending"
	// StateScheduled is waiting for its scheduled time.
	StateScheduled State = "scheduled"
	// StateRunning is currently executing.
	StateRunning State = "running"
	// StateRetrying is waiting for another attempt after failure.
	StateRetrying State = "retrying"
	// StateSucceeded completed successfully.
	StateSucceeded State = "succeeded"
	// StateFailed reached a terminal failure.
	StateFailed State = "failed"
	// StateCancelled was cancelled before successful completion.
	StateCancelled State = "cancelled"
)

// Valid reports whether state belongs to the public inspection contract.
func (state State) Valid() bool {
	switch state {
	case StateQueued, StatePending, StateScheduled, StateRunning, StateRetrying, StateSucceeded, StateFailed, StateCancelled:
		return true
	default:
		return false
	}
}

// Job is the framework-neutral task value supplied to a Handler.
type Job struct {
	ID          int64
	Kind        string
	Payload     json.RawMessage
	Queue       string
	Attempt     int
	MaxAttempts int
}

// Summary is provider-neutral operational task metadata. Payloads and backend
// error details are deliberately excluded from the Admin inspection surface.
type Summary struct {
	ID          int64      `json:"id,string"`
	Kind        string     `json:"kind"`
	Queue       string     `json:"queue"`
	State       State      `json:"state"`
	Attempt     int        `json:"attempt"`
	MaxAttempts int        `json:"max_attempts"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	CreatedAt   time.Time  `json:"created_at"`
	FinalizedAt *time.Time `json:"finalized_at,omitempty"`
}

// ListOptions selects one descending task page. BeforeID is the exclusive
// cursor returned by the previous Page.
type ListOptions struct {
	Limit    int
	BeforeID int64
	Queue    string
	State    State
}

// Page is one bounded operational task result.
type Page struct {
	Tasks        []Summary `json:"tasks"`
	NextBeforeID int64     `json:"next_before_id,omitempty,string"`
}

// Inspector reads bounded task metadata without exposing a queue backend or
// mutation authority.
type Inspector interface {
	List(context.Context, ListOptions) (Page, error)
}

// TerminalAttempt reports whether the current attempt is the last configured
// attempt. It is useful when a product must persist its own terminal status.
func (job Job) TerminalAttempt() bool {
	return job.MaxAttempts > 0 && job.Attempt >= job.MaxAttempts
}

// Handler performs one at-least-once task attempt.
type Handler interface {
	Handle(context.Context, Job) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(context.Context, Job) error

// Handle invokes function.
func (function HandlerFunc) Handle(ctx context.Context, job Job) error {
	return function(ctx, job)
}

// Queue declares one runner queue and its per-process concurrency.
type Queue struct {
	Name       string
	MaxWorkers int
}

// RunnerOptions freezes runner behavior before it starts.
type RunnerOptions struct {
	Queues          []Queue
	JobTimeout      time.Duration
	SoftStopTimeout time.Duration
	// RetryDelays optionally replaces River's default retry schedule. Entry zero
	// follows the first failed attempt; attempts beyond the list reuse its last
	// value. An empty list selects the adapter default.
	RetryDelays []time.Duration
}

// Runner owns one immutable worker process lifecycle.
type Runner interface {
	Start(context.Context) error
	Stop(context.Context) error
	Stopped() <-chan struct{}
}

// Service inserts tasks and constructs immutable runners.
type Service interface {
	Enqueue(context.Context, Request) (Receipt, error)
	NewRunner(Handler, RunnerOptions) (Runner, error)
}

// NormalizeListOptions validates one provider-neutral inspection query.
func NormalizeListOptions(options ListOptions) (ListOptions, error) {
	if options.Limit < 0 || options.Limit > MaxListLimit {
		return ListOptions{}, fmt.Errorf("task list limit must be between 1 and %d", MaxListLimit)
	}
	if options.Limit == 0 {
		options.Limit = DefaultListLimit
	}
	if options.BeforeID < 0 {
		return ListOptions{}, fmt.Errorf("task list cursor cannot be negative")
	}
	if options.Queue != "" && !validQueue(options.Queue) {
		return ListOptions{}, fmt.Errorf("task list queue %q is invalid", options.Queue)
	}
	if options.State != "" && !options.State.Valid() {
		return ListOptions{}, fmt.Errorf("task list state %q is invalid", options.State)
	}
	return options, nil
}

// NormalizeRequest validates and defensively copies one enqueue request.
// It is public so adapter and consumer contract tests use the same rules.
func NormalizeRequest(request Request) (Request, error) {
	if !validKind(request.Kind) {
		return Request{}, fmt.Errorf("task kind %q is invalid", request.Kind)
	}
	if request.Queue == "" {
		request.Queue = DefaultQueue
	}
	if !validQueue(request.Queue) {
		return Request{}, fmt.Errorf("task queue %q is invalid", request.Queue)
	}
	if request.MaxAttempts < 0 {
		return Request{}, fmt.Errorf("task maximum attempts cannot be negative")
	}
	if request.MaxAttempts == 0 {
		request.MaxAttempts = DefaultMaxAttempts
	}
	if request.MaxAttempts > 1000 {
		return Request{}, fmt.Errorf("task maximum attempts cannot exceed 1000")
	}
	if request.ScheduledAt.Year() < 0 || request.ScheduledAt.Year() > 9999 {
		return Request{}, fmt.Errorf("task schedule is outside the supported time range")
	}
	if !utf8.ValidString(request.UniqueKey) || len(request.UniqueKey) > 256 ||
		strings.TrimSpace(request.UniqueKey) != request.UniqueKey || strings.ContainsFunc(request.UniqueKey, unicode.IsControl) {
		return Request{}, fmt.Errorf("task unique key must be valid UTF-8, at most 256 bytes, and contain no control or surrounding whitespace")
	}
	if len(request.Payload) == 0 {
		request.Payload = json.RawMessage(`{}`)
	}
	if len(request.Payload) > MaxPayloadBytes {
		return Request{}, fmt.Errorf("task payload exceeds %d bytes", MaxPayloadBytes)
	}
	if !json.Valid(request.Payload) {
		return Request{}, fmt.Errorf("task payload must be valid JSON")
	}
	request.Payload = append(json.RawMessage(nil), request.Payload...)
	request.ScheduledAt = request.ScheduledAt.UTC()
	return request, nil
}

// NormalizeRunnerOptions validates and defensively copies runner options.
func NormalizeRunnerOptions(options RunnerOptions) (RunnerOptions, error) {
	if options.JobTimeout < 0 {
		return RunnerOptions{}, fmt.Errorf("task runner timeout cannot be negative")
	}
	if options.SoftStopTimeout < 0 {
		return RunnerOptions{}, fmt.Errorf("task runner soft-stop timeout cannot be negative")
	}
	if len(options.RetryDelays) > 1000 {
		return RunnerOptions{}, fmt.Errorf("task runner cannot declare more than 1000 retry delays")
	}
	retryDelays := make([]time.Duration, len(options.RetryDelays))
	for index, delay := range options.RetryDelays {
		if delay < MinimumRetryDelay || delay > MaximumRetryDelay {
			return RunnerOptions{}, fmt.Errorf("task retry delay %d must be between %s and %s", index+1, MinimumRetryDelay, MaximumRetryDelay)
		}
		retryDelays[index] = delay
	}
	options.RetryDelays = retryDelays
	if len(options.Queues) == 0 {
		options.Queues = []Queue{{Name: DefaultQueue, MaxWorkers: DefaultMaxWorkers}}
	}
	if len(options.Queues) > 64 {
		return RunnerOptions{}, fmt.Errorf("task runner cannot declare more than 64 queues")
	}
	queues := make([]Queue, len(options.Queues))
	seen := make(map[string]struct{}, len(options.Queues))
	for index, queue := range options.Queues {
		if queue.Name == "" {
			queue.Name = DefaultQueue
		}
		if !validQueue(queue.Name) {
			return RunnerOptions{}, fmt.Errorf("task queue %q is invalid", queue.Name)
		}
		if _, exists := seen[queue.Name]; exists {
			return RunnerOptions{}, fmt.Errorf("task queue %q is declared more than once", queue.Name)
		}
		seen[queue.Name] = struct{}{}
		if queue.MaxWorkers < 0 {
			return RunnerOptions{}, fmt.Errorf("task queue %q maximum workers cannot be negative", queue.Name)
		}
		if queue.MaxWorkers == 0 {
			queue.MaxWorkers = DefaultMaxWorkers
		}
		if queue.MaxWorkers > 10000 {
			return RunnerOptions{}, fmt.Errorf("task queue %q maximum workers cannot exceed 10000", queue.Name)
		}
		queues[index] = queue
	}
	options.Queues = queues
	return options, nil
}

func validKind(value string) bool {
	return utf8.ValidString(value) && kindPattern.MatchString(value)
}

func validQueue(value string) bool {
	return utf8.ValidString(value) && len(value) <= 64 && queuePattern.MatchString(value)
}
