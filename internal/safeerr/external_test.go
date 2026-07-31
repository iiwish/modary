package safeerr_test

import (
	"errors"
	"testing"
	"time"

	"github.com/iiwish/modary/internal/safeerr"
)

func TestClassifiersNeverInvokeExternalErrorMethods(t *testing.T) {
	target := errors.New("target")
	hostile := &externalHostileError{entered: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if safeerr.Is(hostile, target) {
			t.Error("hostile error matched target")
		}
		if _, ok := safeerr.Find[*externalHostileError](hostile); !ok {
			t.Error("direct type assertion did not find hostile root")
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("classification blocked in an external error method")
	}
	select {
	case <-hostile.entered:
		t.Fatal("external Unwrap was invoked")
	default:
	}
}

type externalHostileError struct{ entered chan struct{} }

func (*externalHostileError) Error() string { panic("external Error invoked") }
func (*externalHostileError) Is(error) bool { panic("external Is invoked") }
func (*externalHostileError) As(any) bool   { panic("external As invoked") }
func (err *externalHostileError) Unwrap() error {
	close(err.entered)
	select {}
}
