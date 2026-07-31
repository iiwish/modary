// Package transactionoutcome carries framework-owned proof of how a
// transaction callback completed. The package is internal so consumer code
// cannot make an operation error look as though rollback was confirmed.
package transactionoutcome

import (
	"errors"

	"github.com/iiwish/modary/internal/safeerr"
)

// State is the atomic completion state proven by an owning transaction layer.
type State uint8

const (
	StateUnknown State = iota
	StateRollbackPending
	StateRolledBack
	StateRollbackFailed
	StateCommitFailed
)

// Snapshot is an immutable view of a verified root outcome.
type Snapshot struct {
	State      State
	Operation  error
	Completion error
}

type outcome struct {
	state      State
	operation  error
	completion error
}

// RollbackPending reports that a nested operation failed and made its outer
// transaction rollback-only, but the owning layer has not rolled back yet.
func RollbackPending(operation error) error {
	return newOutcome(StateRollbackPending, operation, nil)
}

// RolledBack proves that the operation failed and rollback completed.
func RolledBack(operation error) error {
	return newOutcome(StateRolledBack, operation, nil)
}

// RollbackFailed reports an operation failure whose rollback did not complete.
func RollbackFailed(operation, completion error) error {
	return newOutcome(StateRollbackFailed, operation, completion)
}

// CommitFailed reports that a successful callback could not be committed.
func CommitFailed(completion error) error {
	return newOutcome(StateCommitFailed, nil, completion)
}

func newOutcome(state State, operation, completion error) error {
	return &outcome{
		state:      state,
		operation:  safeerr.Opaque(operation),
		completion: safeerr.Opaque(completion),
	}
}

// Root returns a snapshot only when err itself is one framework outcome. A
// wrapper, join, typed-nil value, or replayed fragment is not accepted as proof.
func Root(err error) (Snapshot, bool) {
	value, ok := err.(*outcome)
	if !ok || value == nil || !value.valid() {
		return Snapshot{}, false
	}
	return Snapshot{
		State:      value.state,
		Operation:  value.operation,
		Completion: value.completion,
	}, true
}

func (value *outcome) valid() bool {
	switch value.state {
	case StateRollbackPending, StateRolledBack:
		return value.operation != nil && value.completion == nil
	case StateRollbackFailed:
		return value.operation != nil && value.completion != nil
	case StateCommitFailed:
		return value.operation == nil && value.completion != nil
	default:
		return false
	}
}

func (value *outcome) Error() string {
	if value == nil {
		return "transaction outcome is invalid"
	}
	switch value.state {
	case StateRollbackPending:
		return "transaction is rollback-only"
	case StateRolledBack:
		return "transaction operation was rolled back"
	case StateRollbackFailed:
		return "transaction rollback failed"
	case StateCommitFailed:
		return "transaction commit failed"
	default:
		return "transaction outcome is invalid"
	}
}

func (value *outcome) Unwrap() []error {
	if value == nil {
		return nil
	}
	causes := make([]error, 0, 2)
	if value.operation != nil {
		causes = append(causes, value.operation)
	}
	if value.completion != nil {
		causes = append(causes, value.completion)
	}
	return causes
}

// IsState reports an exact root outcome state.
func IsState(err error, state State) bool {
	snapshot, ok := Root(err)
	return ok && snapshot.State == state
}

// IsOperation reports whether the exact root outcome is correlated with one
// operation marker without dispatching caller-defined error methods.
func IsOperation(err, operation error) bool {
	snapshot, ok := Root(err)
	return ok && operation != nil && safeerr.Is(snapshot.Operation, operation)
}

// ErrInvalid is available to internal contract tests as a stable diagnostic.
var ErrInvalid = errors.New("transaction outcome proof is invalid")
