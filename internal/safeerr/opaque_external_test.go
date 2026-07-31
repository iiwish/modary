package safeerr_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/modary/internal/safeerr"
)

func TestOpaqueSupportsStandardInspectionWithoutDispatchingHostileMethods(t *testing.T) {
	cause := newHostileBoundaryError()
	exactCause := errors.Join(cause, fmt.Errorf("trusted nested error: %w", context.Canceled))
	wrapper := fmt.Errorf("framework boundary: %w", safeerr.Opaque(exactCause))

	type result struct {
		isExact    bool
		isCause    bool
		isCanceled bool
		asCause    bool
		found      *hostileBoundaryError
	}
	resultChannel := make(chan result, 1)
	go func() {
		var found *hostileBoundaryError
		resultChannel <- result{
			isExact:    errors.Is(wrapper, exactCause),
			isCause:    errors.Is(wrapper, cause),
			isCanceled: errors.Is(wrapper, context.Canceled),
			asCause:    errors.As(wrapper, &found),
			found:      found,
		}
	}()

	select {
	case method := <-cause.entered:
		cause.releaseOnce.Do(func() { close(cause.release) })
		t.Fatalf("standard error inspection invoked hostile %s", method)
	case got := <-resultChannel:
		if !got.isExact || !got.isCause || !got.isCanceled || !got.asCause || got.found != cause {
			t.Fatalf("standard inspection = %#v", got)
		}
	case <-time.After(3 * time.Second):
		cause.releaseOnce.Do(func() { close(cause.release) })
		t.Fatal("standard error inspection did not complete")
	}
}

func TestOpaqueDoesNotTrustCallerDefinedUnwrap(t *testing.T) {
	cause := newHostileBoundaryError()
	cause.child = context.Canceled
	wrapper := safeerr.Opaque(cause)
	result := make(chan bool, 1)
	go func() { result <- errors.Is(wrapper, context.Canceled) }()
	select {
	case method := <-cause.entered:
		cause.releaseOnce.Do(func() { close(cause.release) })
		t.Fatalf("opaque inspection invoked hostile %s", method)
	case matched := <-result:
		if matched {
			t.Fatal("opaque inspection traversed caller-defined Unwrap")
		}
	case <-time.After(3 * time.Second):
		cause.releaseOnce.Do(func() { close(cause.release) })
		t.Fatal("opaque inspection did not complete")
	}
}

func TestOpaquePreservesTypedNilWithoutDispatchingItsMethods(t *testing.T) {
	var cause *hostileTypedNilError
	wrapper := safeerr.Opaque(cause)
	if wrapper == nil {
		t.Fatal("Opaque discarded a typed-nil error")
	}
	if got := wrapper.Error(); got != "opaque dependency failure" {
		t.Fatalf("Opaque Error() = %q", got)
	}
	if !errors.Is(wrapper, cause) || errors.Is(wrapper, context.Canceled) {
		t.Fatal("errors.Is did not preserve typed-nil identity safely")
	}
	var found *hostileTypedNilError
	if !errors.As(wrapper, &found) || found != nil {
		t.Fatalf("errors.As typed-nil result = %#v", found)
	}
	if classified, ok := safeerr.Find[*hostileTypedNilError](wrapper); ok || classified != nil {
		t.Fatalf("Find classified typed-nil result = %#v, %t", classified, ok)
	}
}

type hostileBoundaryError struct {
	child       error
	entered     chan string
	release     chan struct{}
	releaseOnce sync.Once
}

type hostileTypedNilError struct{}

func (*hostileTypedNilError) Error() string { panic("typed-nil Error invoked") }
func (*hostileTypedNilError) Is(error) bool { panic("typed-nil Is invoked") }
func (*hostileTypedNilError) As(any) bool   { panic("typed-nil As invoked") }
func (*hostileTypedNilError) Unwrap() error { panic("typed-nil Unwrap invoked") }

func newHostileBoundaryError() *hostileBoundaryError {
	return &hostileBoundaryError{entered: make(chan string, 4), release: make(chan struct{})}
}

func (err *hostileBoundaryError) inspect(method string) {
	err.entered <- method
	<-err.release
}

func (err *hostileBoundaryError) Error() string {
	err.inspect("Error")
	return "hostile"
}

func (err *hostileBoundaryError) Is(error) bool {
	err.inspect("Is")
	return false
}

func (err *hostileBoundaryError) As(any) bool {
	err.inspect("As")
	return false
}

func (err *hostileBoundaryError) Unwrap() error {
	err.inspect("Unwrap")
	return err.child
}
