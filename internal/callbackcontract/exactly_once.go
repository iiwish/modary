// Package callbackcontract provides concurrency-safe guards for extension
// callbacks whose invocation count is part of a framework correctness contract.
package callbackcontract

import (
	"errors"
	"sync"
)

// ErrRepeatedInvocation is returned to a caller that attempts to invoke a
// guarded callback more than once. The first invocation remains authoritative.
var ErrRepeatedInvocation = errors.New("callback was invoked more than once")

// ErrClosedInvocation is returned when a callback is invoked after its owner
// has returned from the extension boundary.
var ErrClosedInvocation = errors.New("callback was invoked after its boundary returned")

// ExactlyOnce admits at most one callback invocation and records whether an
// extension honored its synchronous exactly-once contract. Its zero value is
// ready for use. Call CloseAndWait and inspect Snapshot.Outcome before trusting
// the extension's result.
type ExactlyOnce struct {
	mu        sync.Mutex
	calls     int
	inFlight  bool
	completed bool
	returned  bool
	repeated  bool
	late      bool
	closed    bool
	escaped   bool
	result    error
	done      chan struct{}
}

// Snapshot is an immutable observation of one ExactlyOnce guard.
type Snapshot struct {
	Calls     int
	InFlight  bool
	Completed bool
	Returned  bool
	Repeated  bool
	Late      bool
	Closed    bool
	Escaped   bool
	Result    error
}

// Outcome classifies the callback state after its invocation window closes.
type Outcome uint8

const (
	// OutcomeContractViolation means the callback was repeated, escaped its
	// extension boundary, or otherwise ended in an invalid state.
	OutcomeContractViolation Outcome = iota
	// OutcomeNotInvoked means no callback began before the boundary closed.
	OutcomeNotInvoked
	// OutcomeReturned means one callback returned synchronously.
	OutcomeReturned
	// OutcomePanicked means one callback unwound synchronously without returning.
	OutcomePanicked
)

// Outcome returns the callback's centralized state classification. Callers
// separately decide whether OutcomeNotInvoked is valid for a non-nil setup
// error and whether an OutcomePanicked boundary faithfully propagated panic.
func (snapshot Snapshot) Outcome() Outcome {
	if snapshot.Closed && snapshot.Calls == 0 && !snapshot.InFlight && !snapshot.Completed && !snapshot.Returned && !snapshot.Repeated && !snapshot.Late && !snapshot.Escaped && snapshot.Result == nil {
		return OutcomeNotInvoked
	}
	if snapshot.Closed && snapshot.Calls == 1 && !snapshot.InFlight && snapshot.Completed && !snapshot.Repeated && !snapshot.Late && !snapshot.Escaped {
		if snapshot.Returned {
			return OutcomeReturned
		}
		if snapshot.Result == nil {
			return OutcomePanicked
		}
	}
	return OutcomeContractViolation
}

// NotInvoked reports that no callback began before the boundary closed. This
// can be valid when an extension returns a non-nil setup error first.
func (snapshot Snapshot) NotInvoked() bool {
	return snapshot.Outcome() == OutcomeNotInvoked
}

// SynchronouslyReturnedOnce reports the only valid state after an extension
// has invoked its callback.
func (snapshot Snapshot) SynchronouslyReturnedOnce() bool {
	return snapshot.Outcome() == OutcomeReturned
}

// SynchronouslyPanickedOnce reports that one invocation unwound without
// returning while the extension boundary was still active. The boundary must
// independently prove that it propagated the panic rather than swallowing it.
func (snapshot Snapshot) SynchronouslyPanickedOnce() bool {
	return snapshot.Outcome() == OutcomePanicked
}

// Invoke runs operation only for the first call. A panic is not recovered or
// retained, but the snapshot records that the callback completed without
// returning so an outer boundary can detect a swallowed panic.
func (guard *ExactlyOnce) Invoke(operation func() error) (result error) {
	guard.mu.Lock()
	if guard.closed {
		guard.late = true
		guard.mu.Unlock()
		return ErrClosedInvocation
	}
	guard.calls++
	if guard.calls != 1 {
		guard.repeated = true
		guard.mu.Unlock()
		return ErrRepeatedInvocation
	}
	guard.inFlight = true
	guard.done = make(chan struct{})
	guard.mu.Unlock()

	defer func() {
		guard.mu.Lock()
		guard.inFlight = false
		guard.completed = true
		close(guard.done)
		guard.mu.Unlock()
	}()
	result = operation()
	guard.mu.Lock()
	guard.returned = true
	guard.result = result
	guard.mu.Unlock()
	return result
}

// CloseAndWait closes the invocation window and returns its final state. An
// invocation that already escaped the extension boundary is allowed to finish
// before this method returns, so callback work can never outlive the framework
// call. Later invocations are rejected without running operation.
func (guard *ExactlyOnce) CloseAndWait() Snapshot {
	if guard == nil {
		return Snapshot{Closed: true}
	}
	guard.mu.Lock()
	guard.closed = true
	if guard.inFlight {
		guard.escaped = true
	}
	done := guard.done
	inFlight := guard.inFlight
	guard.mu.Unlock()
	if inFlight {
		<-done
	}
	return guard.Snapshot()
}

// Snapshot returns a race-safe copy of the current invocation state.
func (guard *ExactlyOnce) Snapshot() Snapshot {
	if guard == nil {
		return Snapshot{}
	}
	guard.mu.Lock()
	defer guard.mu.Unlock()
	return Snapshot{
		Calls:     guard.calls,
		InFlight:  guard.inFlight,
		Completed: guard.completed,
		Returned:  guard.returned,
		Repeated:  guard.repeated,
		Late:      guard.late,
		Closed:    guard.closed,
		Escaped:   guard.escaped,
		Result:    guard.result,
	}
}
