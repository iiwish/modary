package callbackcontract

import (
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestExactlyOnceRecordsOneReturnedInvocation(t *testing.T) {
	want := errors.New("operation failed")
	var guard ExactlyOnce
	if err := guard.Invoke(func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("Invoke() error = %v", err)
	}
	snapshot := guard.CloseAndWait()
	if snapshot.Calls != 1 || snapshot.InFlight || !snapshot.Completed || !snapshot.Returned || snapshot.Repeated || !snapshot.Closed || snapshot.Escaped || !errors.Is(snapshot.Result, want) {
		t.Fatalf("Snapshot() = %#v", snapshot)
	}
	if snapshot.Outcome() != OutcomeReturned || !snapshot.SynchronouslyReturnedOnce() || snapshot.NotInvoked() {
		t.Fatalf("valid state classification = %#v", snapshot)
	}
}

func TestExactlyOnceRejectsConcurrentRepeatWithoutRepeatingOperation(t *testing.T) {
	var guard ExactlyOnce
	entered := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- guard.Invoke(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	if err := guard.Invoke(func() error { t.Fatal("repeated operation ran"); return nil }); !errors.Is(err, ErrRepeatedInvocation) {
		t.Fatalf("repeated Invoke() error = %v", err)
	}
	snapshot := guard.Snapshot()
	if snapshot.Calls != 2 || !snapshot.InFlight || snapshot.Completed || snapshot.Returned || !snapshot.Repeated {
		t.Fatalf("in-flight Snapshot() = %#v", snapshot)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	snapshot = guard.CloseAndWait()
	if snapshot.InFlight || !snapshot.Completed || !snapshot.Returned || !snapshot.Closed || snapshot.Escaped || snapshot.Result != nil {
		t.Fatalf("completed Snapshot() = %#v", snapshot)
	}
	if snapshot.Outcome() != OutcomeContractViolation {
		t.Fatalf("repeated invocation outcome = %v", snapshot.Outcome())
	}
}

func TestExactlyOnceRejectsSequentialRepeatWithoutReplacingFirstResult(t *testing.T) {
	want := errors.New("first operation failed")
	var guard ExactlyOnce
	if err := guard.Invoke(func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("first Invoke() error = %v", err)
	}
	ran := false
	if err := guard.Invoke(func() error { ran = true; return nil }); !errors.Is(err, ErrRepeatedInvocation) {
		t.Fatalf("repeated Invoke() error = %v", err)
	}
	if ran {
		t.Fatal("repeated invocation ran operation")
	}
	snapshot := guard.CloseAndWait()
	if snapshot.Calls != 2 || !snapshot.Repeated || !errors.Is(snapshot.Result, want) || snapshot.Outcome() != OutcomeContractViolation {
		t.Fatalf("Snapshot() = %#v", snapshot)
	}
}

func TestExactlyOnceRecordsPanickingInvocationWithoutInspectingValue(t *testing.T) {
	var guard ExactlyOnce
	recovered := capturePanic(func() {
		_ = guard.Invoke(func() error { panic("secret") })
	})
	if recovered != "secret" {
		t.Fatalf("panic = %#v", recovered)
	}
	snapshot := guard.CloseAndWait()
	if snapshot.Calls != 1 || snapshot.InFlight || !snapshot.Completed || snapshot.Returned || snapshot.Repeated || !snapshot.Closed || snapshot.Escaped || snapshot.Result != nil {
		t.Fatalf("Snapshot() = %#v", snapshot)
	}
	if snapshot.Outcome() != OutcomePanicked || !snapshot.SynchronouslyPanickedOnce() || snapshot.SynchronouslyReturnedOnce() || snapshot.NotInvoked() {
		t.Fatalf("panicked state classification = %#v", snapshot)
	}
}

func TestExactlyOnceSnapshotIsRaceSafe(t *testing.T) {
	var guard ExactlyOnce
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_ = guard.Invoke(func() error { return nil })
			_ = guard.Snapshot()
		}()
	}
	wait.Wait()
	snapshot := guard.CloseAndWait()
	if snapshot.Calls != 32 || !snapshot.Completed || !snapshot.Returned || !snapshot.Repeated || !snapshot.Closed || snapshot.Escaped {
		t.Fatalf("Snapshot() = %#v", snapshot)
	}
	if snapshot.Outcome() != OutcomeContractViolation {
		t.Fatalf("repeated invocation outcome = %v", snapshot.Outcome())
	}
}

func TestExactlyOnceCloseRejectsLateInvocationWithoutRunningOperation(t *testing.T) {
	var guard ExactlyOnce
	snapshot := guard.CloseAndWait()
	if !snapshot.Closed || snapshot.Calls != 0 || snapshot.Outcome() != OutcomeNotInvoked || !snapshot.NotInvoked() || snapshot.SynchronouslyReturnedOnce() {
		t.Fatalf("CloseAndWait() = %#v", snapshot)
	}
	ran := false
	if err := guard.Invoke(func() error { ran = true; return nil }); !errors.Is(err, ErrClosedInvocation) {
		t.Fatalf("late Invoke() error = %v", err)
	}
	if ran {
		t.Fatal("late invocation ran operation")
	}
	snapshot = guard.Snapshot()
	if !snapshot.Late || snapshot.Outcome() != OutcomeContractViolation {
		t.Fatalf("late invocation Snapshot() = %#v", snapshot)
	}
}

func TestExactlyOnceCloseRejectsLateInvocationAfterReturnedResult(t *testing.T) {
	for _, test := range []struct {
		name   string
		result error
	}{
		{name: "success"},
		{name: "failure", result: errors.New("operation failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			var guard ExactlyOnce
			if err := guard.Invoke(func() error { return test.result }); !errors.Is(err, test.result) {
				t.Fatalf("Invoke() error = %v", err)
			}
			before := guard.CloseAndWait()
			if before.Outcome() != OutcomeReturned {
				t.Fatalf("initial Snapshot() = %#v", before)
			}
			ran := false
			if err := guard.Invoke(func() error { ran = true; return nil }); !errors.Is(err, ErrClosedInvocation) {
				t.Fatalf("late Invoke() error = %v", err)
			}
			if ran {
				t.Fatal("late invocation ran operation")
			}
			after := guard.Snapshot()
			if !after.Late || after.Calls != 1 || !errors.Is(after.Result, test.result) || after.Outcome() != OutcomeContractViolation {
				t.Fatalf("late Snapshot() = %#v", after)
			}
		})
	}
}

func TestExactlyOncePreservesTypedNilResultWithoutDispatchingMethods(t *testing.T) {
	var typedNil *typedNilError
	var want error = typedNil
	var guard ExactlyOnce
	if got := guard.Invoke(func() error { return want }); got == nil {
		t.Fatal("Invoke() erased typed-nil result")
	}
	snapshot := guard.CloseAndWait()
	if snapshot.Outcome() != OutcomeReturned || snapshot.Result == nil {
		t.Fatalf("Snapshot() = %#v", snapshot)
	}
	got := snapshot.Result.(*typedNilError)
	if got != nil {
		t.Fatalf("typed-nil result = %#v", got)
	}
}

func TestExactlyOnceCloseWaitsForEscapedInvocation(t *testing.T) {
	want := errors.New("escaped operation failed")
	var guard ExactlyOnce
	entered := make(chan struct{})
	release := make(chan struct{})
	invoked := make(chan error, 1)
	go func() {
		invoked <- guard.Invoke(func() error {
			close(entered)
			<-release
			return want
		})
	}()
	<-entered
	closed := make(chan Snapshot, 1)
	go func() { closed <- guard.CloseAndWait() }()
	deadline := time.Now().Add(time.Second)
	for !guard.Snapshot().Closed {
		if time.Now().After(deadline) {
			t.Fatal("CloseAndWait did not close the guard")
		}
		runtime.Gosched()
	}
	select {
	case snapshot := <-closed:
		t.Fatalf("CloseAndWait returned before operation completed: %#v", snapshot)
	default:
	}
	close(release)
	if err := <-invoked; !errors.Is(err, want) {
		t.Fatalf("Invoke() error = %v", err)
	}
	snapshot := <-closed
	if !snapshot.Closed || !snapshot.Escaped || snapshot.InFlight || !snapshot.Completed || !snapshot.Returned || !errors.Is(snapshot.Result, want) {
		t.Fatalf("CloseAndWait() = %#v", snapshot)
	}
	if snapshot.Outcome() != OutcomeContractViolation {
		t.Fatalf("escaped invocation outcome = %v", snapshot.Outcome())
	}
}

func TestExactlyOnceConcurrentCloseWaitsForOneEscapedInvocation(t *testing.T) {
	var guard ExactlyOnce
	entered := make(chan struct{})
	release := make(chan struct{})
	invoked := make(chan error, 1)
	go func() {
		invoked <- guard.Invoke(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	const closers = 16
	closed := make(chan Snapshot, closers)
	for range closers {
		go func() { closed <- guard.CloseAndWait() }()
	}
	deadline := time.Now().Add(time.Second)
	for !guard.Snapshot().Closed {
		if time.Now().After(deadline) {
			t.Fatal("concurrent CloseAndWait calls did not close the guard")
		}
		runtime.Gosched()
	}
	select {
	case snapshot := <-closed:
		t.Fatalf("CloseAndWait returned before operation completed: %#v", snapshot)
	default:
	}
	close(release)
	if err := <-invoked; err != nil {
		t.Fatal(err)
	}
	for range closers {
		snapshot := <-closed
		if !snapshot.Closed || !snapshot.Escaped || snapshot.Outcome() != OutcomeContractViolation {
			t.Fatalf("CloseAndWait() = %#v", snapshot)
		}
	}
}

type typedNilError struct{}

func (*typedNilError) Error() string { panic("typed-nil Error invoked") }
func (*typedNilError) Is(error) bool { panic("typed-nil Is invoked") }
func (*typedNilError) As(any) bool   { panic("typed-nil As invoked") }
func (*typedNilError) Unwrap() error { panic("typed-nil Unwrap invoked") }

func capturePanic(operation func()) (recovered any) {
	defer func() { recovered = recover() }()
	operation()
	return nil
}
