package httpapi_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/modary/transport/httpapi"
)

func TestSPAFilesystemErrorChainIsSafeToInspect(t *testing.T) {
	hostile := &hostileFilesystemError{
		entered: make(chan string, 8),
		release: make(chan struct{}),
	}
	t.Cleanup(func() {
		hostile.releaseOnce.Do(func() { close(hostile.release) })
	})
	filesystemCause := errors.Join(hostile, context.Canceled)
	handler, err := httpapi.NewSPA(failingFilesystem{cause: filesystemCause}, httpapi.SPAOptions{})
	if handler != nil || err == nil {
		t.Fatal("NewSPA accepted a failing consumer filesystem")
	}

	type inspection struct {
		text          string
		exact         bool
		hostileExact  bool
		trusted       bool
		typed         bool
		typedIdentity bool
	}
	result := make(chan inspection, 1)
	go func() {
		var typed *hostileFilesystemError
		typedMatch := errors.As(err, &typed)
		result <- inspection{
			text:          err.Error(),
			exact:         errors.Is(err, filesystemCause),
			hostileExact:  errors.Is(err, hostile),
			trusted:       errors.Is(err, context.Canceled),
			typed:         typedMatch,
			typedIdentity: typed == hostile,
		}
	}()

	select {
	case method := <-hostile.entered:
		hostile.releaseOnce.Do(func() { close(hostile.release) })
		select {
		case <-result:
		case <-time.After(2 * time.Second):
		}
		t.Fatalf("SPA error inspection invoked hostile %s", method)
	case got := <-result:
		if !strings.Contains(got.text, "walk SPA filesystem failed") || strings.Contains(got.text, filesystemErrorSecret) {
			t.Fatalf("SPA dependency diagnostic = %q", got.text)
		}
		if !got.exact || !got.hostileExact || !got.trusted || !got.typed || !got.typedIdentity {
			t.Fatalf(
				"standard inspection exact=%t hostile=%t trusted=%t typed=%t identity=%t",
				got.exact, got.hostileExact, got.trusted, got.typed, got.typedIdentity,
			)
		}
	case <-time.After(3 * time.Second):
		hostile.releaseOnce.Do(func() { close(hostile.release) })
		t.Fatal("SPA error inspection did not complete")
	}
	select {
	case method := <-hostile.entered:
		t.Fatalf("SPA error inspection invoked hostile %s", method)
	default:
	}
}

type failingFilesystem struct{ cause error }

func (filesystem failingFilesystem) Open(string) (fs.File, error) {
	return nil, filesystem.cause
}

const filesystemErrorSecret = "consumer-filesystem-error-secret"

type hostileFilesystemError struct {
	entered     chan string
	release     chan struct{}
	releaseOnce sync.Once
}

func (err *hostileFilesystemError) inspect(method string) {
	err.entered <- method
	<-err.release
}

func (err *hostileFilesystemError) Error() string {
	err.inspect("Error")
	return filesystemErrorSecret
}

func (err *hostileFilesystemError) Is(error) bool {
	err.inspect("Is")
	return false
}

func (err *hostileFilesystemError) As(any) bool {
	err.inspect("As")
	return false
}

func (err *hostileFilesystemError) Unwrap() error {
	err.inspect("Unwrap")
	return nil
}
