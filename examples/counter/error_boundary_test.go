package consumer_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/modary-counter-consumer/internal/project"
	"example.com/modary-counter-consumer/modules/counter"
	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appcmd"
	"github.com/iiwish/modary/module"
)

func TestPublicCommandWriterErrorChainIsSafeToInspect(t *testing.T) {
	hostile, callbackCause := newConsumerBoundaryCause(t, context.Canceled)
	err := appcmd.Run(context.Background(), []string{"help"}, nil, appcmd.Options{
		Metadata: project.ApplicationMetadata(),
		Stdout:   consumerErrorWriter{cause: callbackCause},
		Stderr:   io.Discard,
	})
	assertPublicSafeErrorChain(t, err, callbackCause, hostile, context.Canceled, "write command output failed")
}

func TestPublicCommandReaderErrorChainIsSafeToInspect(t *testing.T) {
	hostile, callbackCause := newConsumerBoundaryCause(t, fs.ErrPermission)
	err := appcmd.RunAction(context.Background(), []string{
		"run", counter.ActionID,
		"--token-file", "-",
		"--input", "unused.json",
	}, mustDefinition(t, project.DefaultConfig()), appcmd.Options{
		Stdin:  consumerErrorReader{cause: callbackCause},
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	assertPublicSafeErrorChain(t, err, callbackCause, hostile, fs.ErrPermission, "read CLI token file")
}

func TestPublicActionCallbackErrorChainIsSafeToInspect(t *testing.T) {
	hostile, callbackCause := newConsumerBoundaryCause(t, context.Canceled)
	definition := mustDefinition(t, postgresTestConfig(t))
	definition.Modules = append([]module.Registration(nil), definition.Modules...)
	replaced := false
	for moduleIndex := range definition.Modules {
		actions := append([]module.ActionBinding(nil), definition.Modules[moduleIndex].Definition.Actions...)
		for actionIndex := range actions {
			if actions[actionIndex].Descriptor.ID != counter.ActionID {
				continue
			}
			actions[actionIndex].NewHandler = func(context.Context, module.Resolver) (action.Handler, error) {
				return consumerFailureHandler{cause: callbackCause}, nil
			}
			replaced = true
		}
		definition.Modules[moduleIndex].Definition.Actions = actions
	}
	if !replaced {
		t.Fatal("Counter Action binding was not found")
	}

	application := startApplication(t, definition)
	defer shutdownApplication(t, application)
	actor := resolveActor(t, application, project.PrimaryActorID)
	_, err := application.Runtime().Preview(context.Background(), action.Request{
		RequestID: "consumer-hostile-callback",
		Actor:     actor,
		Channel:   "test",
		ActionID:  counter.ActionID,
		Scope:     actor.Scope,
		Input:     counterInput(0, 1),
	})
	assertPublicSafeErrorChain(t, err, callbackCause, hostile, context.Canceled, "INTERNAL_ERROR")
}

func newConsumerBoundaryCause(t *testing.T, trusted error) (*consumerHostileError, error) {
	t.Helper()
	hostile := &consumerHostileError{
		entered: make(chan string, 8),
		release: make(chan struct{}),
	}
	t.Cleanup(func() {
		hostile.releaseOnce.Do(func() { close(hostile.release) })
	})
	return hostile, errors.Join(hostile, trusted)
}

func assertPublicSafeErrorChain(
	t *testing.T,
	err error,
	exactCause error,
	hostile *consumerHostileError,
	trusted error,
	wantText string,
) {
	t.Helper()
	if err == nil {
		t.Fatal("public boundary discarded the dependency error")
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
		var typed *consumerHostileError
		typedMatch := errors.As(err, &typed)
		result <- inspection{
			text:          err.Error(),
			exact:         errors.Is(err, exactCause),
			hostileExact:  errors.Is(err, hostile),
			trusted:       errors.Is(err, trusted),
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
		t.Fatalf("standard error inspection invoked hostile %s", method)
	case got := <-result:
		if !strings.Contains(got.text, wantText) || strings.Contains(got.text, consumerErrorSecret) {
			t.Fatalf("public diagnostic = %q, want %q without dependency data", got.text, wantText)
		}
		if !got.exact || !got.hostileExact || !got.trusted || !got.typed || !got.typedIdentity {
			t.Fatalf(
				"standard inspection exact=%t hostile=%t trusted=%t typed=%t identity=%t",
				got.exact, got.hostileExact, got.trusted, got.typed, got.typedIdentity,
			)
		}
	case <-time.After(3 * time.Second):
		hostile.releaseOnce.Do(func() { close(hostile.release) })
		t.Fatal("standard error inspection did not complete")
	}
	select {
	case method := <-hostile.entered:
		t.Fatalf("standard error inspection invoked hostile %s", method)
	default:
	}
}

const consumerErrorSecret = "external-consumer-error-secret"

type consumerHostileError struct {
	entered     chan string
	release     chan struct{}
	releaseOnce sync.Once
}

func (err *consumerHostileError) inspect(method string) {
	err.entered <- method
	<-err.release
}

func (err *consumerHostileError) Error() string {
	err.inspect("Error")
	return consumerErrorSecret
}

func (err *consumerHostileError) Is(error) bool {
	err.inspect("Is")
	return false
}

func (err *consumerHostileError) As(any) bool {
	err.inspect("As")
	return false
}

func (err *consumerHostileError) Unwrap() error {
	err.inspect("Unwrap")
	return nil
}

type consumerErrorWriter struct{ cause error }

func (writer consumerErrorWriter) Write(data []byte) (int, error) {
	return len(data), writer.cause
}

type consumerErrorReader struct{ cause error }

func (reader consumerErrorReader) Read([]byte) (int, error) { return 0, reader.cause }
func (consumerErrorReader) Close() error                    { return nil }

type consumerFailureHandler struct{ cause error }

func (handler consumerFailureHandler) Plan(context.Context, action.Request) (action.PlanData, error) {
	return action.PlanData{}, handler.cause
}

func (handler consumerFailureHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	return action.Result{}, handler.cause
}
