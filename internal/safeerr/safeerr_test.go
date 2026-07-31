package safeerr

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
)

func TestDiagnosticDoesNotInspectTypedNil(t *testing.T) {
	diagnosticTypedNilMethodCalls.Store(0)
	var typedNil *diagnosticTypedNilError
	if got := Diagnostic(typedNil); got != "opaque dependency failure" {
		t.Fatalf("Diagnostic(typed nil) = %q", got)
	}
	if got := Diagnostic(errors.New("consumer diagnostic")); got != "consumer diagnostic" {
		t.Fatalf("Diagnostic(ordinary) = %q", got)
	}
	wrapped := Opaque(typedNil)
	if errors.Is(wrapped, diagnosticTypedNilChild) {
		t.Fatal("typed-nil Unwrap was invoked")
	}
	var found *diagnosticTypedNilError
	if !errors.Is(wrapped, typedNil) || !errors.As(wrapped, &found) || found != typedNil {
		t.Fatalf("typed-nil identity was not safely preserved: Is=%t As=%t value=%#v",
			errors.Is(wrapped, typedNil), errors.As(wrapped, &found), found)
	}
	if calls := diagnosticTypedNilMethodCalls.Load(); calls != 0 {
		t.Fatalf("typed-nil error methods called %d times", calls)
	}
}

func TestWalkBoundsCyclesAndLargeGraphs(t *testing.T) {
	cycle := &trustedTestError{}
	cycle.child = cycle
	if Is(cycle, errors.New("absent")) {
		t.Fatal("cyclic graph reported an absent target")
	}

	target := errors.New("deep target")
	var root error = target
	for range maxNodes + 8 {
		root = &trustedTestError{child: root}
	}
	if Is(root, target) {
		t.Fatal("walk exceeded its documented node bound")
	}

	wrapped := fmt.Errorf("outer: %w", target)
	if !Is(wrapped, target) {
		t.Fatal("standard-library wrapping was not traversed")
	}
}

func TestFindUsesTypeAssertionsWithoutCustomAs(t *testing.T) {
	type classified struct{ trustedTestError }
	value := &classified{}
	wrapped := fmt.Errorf("outer: %w", value)
	if got, ok := Find[*classified](wrapped); !ok || got != value {
		t.Fatalf("Find() = %#v, %t", got, ok)
	}
}

func TestTrustedPackagePathsAreExactAllowlistEntries(t *testing.T) {
	for _, packagePath := range []string{
		"fmt",
		"context",
		"github.com/iiwish/modary/action",
		"github.com/iiwish/modary/internal/actionruntime",
		"github.com/iiwish/modary/internal/databasecontrol",
	} {
		if !trustedPackagePath(packagePath) {
			t.Fatalf("trusted package %q was rejected", packagePath)
		}
	}
	for _, packagePath := range []string{
		"consumer",
		"consumer/errors",
		"consumer/fmt",
		"fmt/extension",
		"github.com/iiwish/modary/actionextension",
		"example.com/dependency/errors",
	} {
		if trustedPackagePath(packagePath) {
			t.Fatalf("untrusted package %q was accepted", packagePath)
		}
	}
}

type trustedTestError struct{ child error }

func (*trustedTestError) Error() string { return "trusted test error" }
func (err *trustedTestError) Unwrap() error {
	return err.child
}

type diagnosticTypedNilError struct{}

var (
	diagnosticTypedNilChild       = errors.New("typed-nil child")
	diagnosticTypedNilMethodCalls atomic.Int64
)

func (*diagnosticTypedNilError) Error() string {
	diagnosticTypedNilMethodCalls.Add(1)
	panic("typed-nil Error invoked")
}

func (*diagnosticTypedNilError) Is(error) bool {
	diagnosticTypedNilMethodCalls.Add(1)
	panic("typed-nil Is invoked")
}

func (*diagnosticTypedNilError) As(any) bool {
	diagnosticTypedNilMethodCalls.Add(1)
	panic("typed-nil As invoked")
}

func (*diagnosticTypedNilError) Unwrap() error {
	diagnosticTypedNilMethodCalls.Add(1)
	return diagnosticTypedNilChild
}
