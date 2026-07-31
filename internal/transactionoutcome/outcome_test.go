package transactionoutcome

import (
	"errors"
	"fmt"
	"testing"
)

func TestRootAcceptsEachExactValidOutcome(t *testing.T) {
	operation := errors.New("operation failed")
	completion := errors.New("transaction completion failed")
	tests := []struct {
		name           string
		outcome        error
		state          State
		wantOperation  bool
		wantCompletion bool
		message        string
	}{
		{
			name:          "rollback pending",
			outcome:       RollbackPending(operation),
			state:         StateRollbackPending,
			wantOperation: true,
			message:       "transaction is rollback-only",
		},
		{
			name:          "rolled back",
			outcome:       RolledBack(operation),
			state:         StateRolledBack,
			wantOperation: true,
			message:       "transaction operation was rolled back",
		},
		{
			name:           "rollback failed",
			outcome:        RollbackFailed(operation, completion),
			state:          StateRollbackFailed,
			wantOperation:  true,
			wantCompletion: true,
			message:        "transaction rollback failed",
		},
		{
			name:           "commit failed",
			outcome:        CommitFailed(completion),
			state:          StateCommitFailed,
			wantCompletion: true,
			message:        "transaction commit failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, ok := Root(test.outcome)
			if !ok {
				t.Fatal("Root rejected an exact valid outcome")
			}
			if snapshot.State != test.state || !IsState(test.outcome, test.state) {
				t.Fatalf("state = %v, want %v", snapshot.State, test.state)
			}
			if got := test.outcome.Error(); got != test.message {
				t.Fatalf("diagnostic = %q, want %q", got, test.message)
			}
			if got := errors.Is(snapshot.Operation, operation); got != test.wantOperation {
				t.Fatalf("operation present = %t, want %t", got, test.wantOperation)
			}
			if got := errors.Is(snapshot.Completion, completion); got != test.wantCompletion {
				t.Fatalf("completion present = %t, want %t", got, test.wantCompletion)
			}
			if got := IsOperation(test.outcome, operation); got != test.wantOperation {
				t.Fatalf("IsOperation = %t, want %t", got, test.wantOperation)
			}
		})
	}
}

func TestRootRejectsAnythingExceptAnExactValidRoot(t *testing.T) {
	operation := errors.New("operation failed")
	completion := errors.New("completion failed")
	rolledBack := RolledBack(operation)
	rollbackFailed := RollbackFailed(operation, completion)
	rollbackSnapshot, ok := Root(rollbackFailed)
	if !ok {
		t.Fatal("test setup produced an invalid rollback outcome")
	}
	var typedNil *outcome

	tests := []struct {
		name string
		err  error
	}{
		{name: "nil", err: nil},
		{name: "typed nil", err: typedNil},
		{name: "ordinary error", err: operation},
		{name: "wrapped", err: fmt.Errorf("wrapped outcome: %w", rolledBack)},
		{name: "joined", err: errors.Join(rolledBack, completion)},
		{name: "replayed operation fragment", err: rollbackSnapshot.Operation},
		{name: "replayed completion fragment", err: rollbackSnapshot.Completion},
		{name: "unknown state", err: &outcome{}},
		{name: "pending without operation", err: &outcome{state: StateRollbackPending}},
		{name: "rolled back with completion", err: &outcome{state: StateRolledBack, operation: operation, completion: completion}},
		{name: "rollback failed without operation", err: &outcome{state: StateRollbackFailed, completion: completion}},
		{name: "rollback failed without completion", err: &outcome{state: StateRollbackFailed, operation: operation}},
		{name: "commit failed with operation", err: &outcome{state: StateCommitFailed, operation: operation, completion: completion}},
		{name: "commit failed without completion", err: &outcome{state: StateCommitFailed}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if snapshot, ok := Root(test.err); ok || snapshot != (Snapshot{}) {
				t.Fatalf("Root(%T) = %#v, %t", test.err, snapshot, ok)
			}
			if IsState(test.err, StateRolledBack) || IsOperation(test.err, operation) {
				t.Fatal("invalid root was accepted by a convenience classifier")
			}
		})
	}
}

func TestOutcomeCorrelationIsExactAndHostileErrorsStayOpaque(t *testing.T) {
	operation := &hostileOutcomeError{}
	wrongOperation := errors.New("different operation")
	proof := RollbackFailed(operation, &hostileOutcomeError{})

	if !IsOperation(proof, operation) {
		t.Fatal("exact operation marker was not correlated")
	}
	if IsOperation(proof, wrongOperation) || IsOperation(proof, nil) {
		t.Fatal("wrong or nil operation marker was correlated")
	}
	if !errors.Is(proof, operation) {
		t.Fatal("opaque operation cause was not preserved")
	}
	var matched *hostileOutcomeError
	if !errors.As(proof, &matched) || matched != operation {
		t.Fatalf("opaque errors.As = %#v", matched)
	}
}

func TestOutcomeTreatsTypedNilCausesAsPresentWithoutDispatchingMethods(t *testing.T) {
	var typedNil *hostileOutcomeError
	var cause error = typedNil
	proof := RolledBack(cause)

	snapshot, ok := Root(proof)
	if !ok || snapshot.State != StateRolledBack || snapshot.Operation == nil {
		t.Fatalf("typed-nil outcome = %#v, %t", snapshot, ok)
	}
	if !IsOperation(proof, cause) || !errors.Is(proof, cause) {
		t.Fatal("typed-nil operation identity was lost")
	}
	matched := &hostileOutcomeError{}
	if !errors.As(proof, &matched) || matched != nil {
		t.Fatalf("typed-nil errors.As = %#v", matched)
	}
}

func TestConstructorsFailClosedWhenRequiredCausesAreMissing(t *testing.T) {
	operation := errors.New("operation failed")
	completion := errors.New("completion failed")
	invalid := []error{
		RollbackPending(nil),
		RolledBack(nil),
		RollbackFailed(nil, completion),
		RollbackFailed(operation, nil),
		CommitFailed(nil),
	}
	for index, err := range invalid {
		if _, ok := Root(err); ok {
			t.Fatalf("invalid constructor result %d was accepted", index)
		}
	}
}

type hostileOutcomeError struct{}

func (*hostileOutcomeError) Error() string { panic("hostile Error invoked") }
func (*hostileOutcomeError) Is(error) bool { panic("hostile Is invoked") }
func (*hostileOutcomeError) As(any) bool   { panic("hostile As invoked") }
func (*hostileOutcomeError) Unwrap() error { panic("hostile Unwrap invoked") }
