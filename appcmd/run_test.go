package appcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/appkit"
)

func TestRunPureCommandsAndPreflightFailuresNeverStartModules(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		mutate     func(*appkit.Definition, *Options)
		wantUsage  bool
		wantOutput string
	}{
		{name: "default help"},
		{name: "help", args: []string{"help"}},
		{name: "serve help", args: []string{"serve", "--help"}},
		{name: "action help", args: []string{"action", "--help"}},
		{name: "action run help", args: []string{"action", "run", "--help"}},
		{name: "version", args: []string{"version"}, wantOutput: "lifecycle-test 1.0.0\n"},
		{name: "unknown command", args: []string{"unknown"}, wantUsage: true},
		{name: "help arguments", args: []string{"help", "extra"}, wantUsage: true},
		{name: "version arguments", args: []string{"version", "extra"}, wantUsage: true},
		{name: "serve unknown flag", args: []string{"serve", "--unknown"}, wantUsage: true},
		{name: "serve positional", args: []string{"serve", "extra"}, wantUsage: true},
		{name: "serve duplicate listen", args: []string{"serve", "--listen", "127.0.0.1:0", "--listen=127.0.0.1:1"}, wantUsage: true},
		{name: "serve missing listen value", args: []string{"serve", "--listen"}, wantUsage: true},
		{name: "serve help arguments", args: []string{"serve", "--help", "extra"}, wantUsage: true},
		{name: "action missing run", args: []string{"action", "execute"}, wantUsage: true},
		{name: "action missing id", args: []string{"action", "run"}, wantUsage: true},
		{name: "action missing token", args: []string{"action", "run", "example.echo", "--input", "-"}, wantUsage: true},
		{
			name: "invalid command option", args: []string{"serve"}, wantUsage: true,
			mutate: func(_ *appkit.Definition, options *Options) { options.ShutdownTimeout = -time.Second },
		},
		{
			name: "invalid server option", args: []string{"serve"}, wantUsage: true,
			mutate: func(_ *appkit.Definition, options *Options) { options.Server.MaxHeaderBytes = MaximumHeaderBytes + 1 },
		},
		{
			name: "missing handler", args: []string{"serve"}, wantUsage: true,
			mutate: func(_ *appkit.Definition, options *Options) { options.Handler = nil },
		},
		{
			name: "invalid listen address", args: []string{"serve", "--listen", "bad host:8080"}, wantUsage: true,
		},
		{
			name: "invalid version id", args: []string{"version"}, wantUsage: true,
			mutate: func(definition *appkit.Definition, _ *Options) { definition.Metadata.ID = "Invalid" },
		},
		{
			name: "invalid semantic version", args: []string{"version"}, wantUsage: true,
			mutate: func(definition *appkit.Definition, _ *Options) { definition.Metadata.Version = "latest" },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLifecycleFixture()
			definition := fixture.definition()
			var output bytes.Buffer
			options := Options{
				Metadata: definition.Metadata,
				Handler:  func(context.Context, *appkit.Application) (http.Handler, error) { return http.NotFoundHandler(), nil },
				Stdout:   &output,
				Stderr:   io.Discard,
			}
			if test.mutate != nil {
				test.mutate(&definition, &options)
			}
			options.Metadata = definition.Metadata
			var providerCalls atomic.Int64
			err := Run(context.Background(), test.args, func() (appkit.Definition, error) {
				providerCalls.Add(1)
				return definition, nil
			}, options)
			if test.wantUsage && !errors.Is(err, ErrUsage) {
				t.Fatalf("Run() error = %v, want ErrUsage", err)
			}
			if !test.wantUsage && err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if test.wantOutput != "" && output.String() != test.wantOutput {
				t.Fatalf("Run() output = %q, want %q", output.String(), test.wantOutput)
			}
			if fixture.starts.Load() != 0 || fixture.cleanups.Load() != 0 {
				t.Fatalf("pure command started=%d cleaned=%d", fixture.starts.Load(), fixture.cleanups.Load())
			}
			if providerCalls.Load() != 0 {
				t.Fatalf("pure command constructed Definition %d times", providerCalls.Load())
			}
		})
	}
}

func TestRunOutputFailuresAndPanicsAreContainedWithoutStarting(t *testing.T) {
	writeErr := errors.New("write failed")
	for _, test := range []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "error", writer: lifecycleErrorWriter{err: writeErr}, want: writeErr},
		{name: "short", writer: lifecycleShortWriter{}, want: io.ErrShortWrite},
		{name: "panic", writer: lifecyclePanicWriter{}, want: ErrCallbackPanic},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLifecycleFixture()
			definition := fixture.definition()
			var providerCalls atomic.Int64
			err := Run(context.Background(), []string{"serve", "--help"}, func() (appkit.Definition, error) {
				providerCalls.Add(1)
				return definition, nil
			}, Options{
				Metadata: definition.Metadata,
				Stdout:   test.writer,
				Stderr:   io.Discard,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Run() error = %v, want errors.Is(%v)", err, test.want)
			}
			if fixture.starts.Load() != 0 || fixture.cleanups.Load() != 0 {
				t.Fatalf("writer failure started=%d cleaned=%d", fixture.starts.Load(), fixture.cleanups.Load())
			}
			if providerCalls.Load() != 0 {
				t.Fatalf("serve help constructed Definition %d times", providerCalls.Load())
			}
		})
	}
}

func TestRunRejectsNilContextWithoutStarting(t *testing.T) {
	fixture := newLifecycleFixture()
	definition := fixture.definition()
	if err := Run(nil, []string{"serve"}, definitionProvider(definition), Options{Metadata: definition.Metadata}); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("Run(nil) error = %v", err)
	}
	if fixture.starts.Load() != 0 {
		t.Fatalf("Run(nil) started Modules %d times", fixture.starts.Load())
	}
}

func TestExplicitTypedNilIOFailsClosedAcrossCommandEntrypoints(t *testing.T) {
	var typedNil *lifecycleTypedNilIO
	for _, test := range []struct {
		name string
		run  func(appkit.Definition) error
	}{
		{
			name: "Run help stdout",
			run: func(definition appkit.Definition) error {
				return Run(context.Background(), []string{"help"}, definitionProvider(definition), Options{
					Metadata: definition.Metadata, Stdout: typedNil,
				})
			},
		},
		{
			name: "Run version stdin",
			run: func(definition appkit.Definition) error {
				return Run(context.Background(), []string{"version"}, definitionProvider(definition), Options{
					Metadata: definition.Metadata, Stdin: typedNil, Stdout: io.Discard,
				})
			},
		},
		{
			name: "Serve stderr",
			run: func(definition appkit.Definition) error {
				return Serve(context.Background(), definition, Options{Handler: lifecycleHandlerFactory, Stderr: typedNil})
			},
		},
		{
			name: "RunAction stdin",
			run: func(definition appkit.Definition) error {
				return RunAction(context.Background(), []string{"run", "example.echo", "--token-file", "token", "--input", "-", "--scope-kind", "tenant", "--scope-id", "acme"}, definition, Options{Stdin: typedNil, Stdout: io.Discard})
			},
		},
		{
			name: "RunAction help stdout",
			run: func(definition appkit.Definition) error {
				return RunAction(context.Background(), []string{"--help"}, definition, Options{Stdout: typedNil})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLifecycleFixture()
			err := test.run(fixture.definition())
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("command error = %v, want ErrUsage", err)
			}
			if fixture.starts.Load() != 0 || fixture.cleanups.Load() != 0 {
				t.Fatalf("typed-nil IO started=%d cleaned=%d", fixture.starts.Load(), fixture.cleanups.Load())
			}
		})
	}

	prepared, err := prepareIOOptions(Options{})
	if err != nil || prepared.Stdin == nil || prepared.Stdout == nil || prepared.Stderr == nil {
		t.Fatalf("plain nil IO defaults = %#v, %v", prepared, err)
	}
}

func TestRunDefinitionProviderErrorsPanicsAndMetadataDrift(t *testing.T) {
	fixture := newLifecycleFixture()
	definition := fixture.definition()
	options := Options{
		Metadata: definition.Metadata,
		Handler:  lifecycleHandlerFactory,
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}

	t.Run("ordinary error", func(t *testing.T) {
		cause := errors.New("configure PostgreSQL: database URL is required")
		var calls atomic.Int64
		err := Run(context.Background(), []string{"serve"}, func() (appkit.Definition, error) {
			calls.Add(1)
			return definition, cause
		}, options)
		if !errors.Is(err, cause) {
			t.Fatalf("Run() error = %v, want provider cause", err)
		}
		if text := err.Error(); !strings.Contains(text, "construct application Definition") ||
			!strings.Contains(text, cause.Error()) {
			t.Fatalf("provider diagnostic = %q", text)
		}
		if calls.Load() != 1 {
			t.Fatalf("provider calls = %d, want 1", calls.Load())
		}
	})

	t.Run("panic", func(t *testing.T) {
		var calls atomic.Int64
		err := Run(context.Background(), []string{"serve"}, func() (appkit.Definition, error) {
			calls.Add(1)
			panic("provider panic secret")
		}, options)
		if !errors.Is(err, ErrCallbackPanic) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("provider panic error = %v", err)
		}
		if calls.Load() != 1 {
			t.Fatalf("provider calls = %d, want 1", calls.Load())
		}
	})

	t.Run("panic nil", func(t *testing.T) {
		err := Run(context.Background(), []string{"serve"}, func() (appkit.Definition, error) {
			panic(nil)
		}, options)
		if !errors.Is(err, ErrCallbackPanic) {
			t.Fatalf("nil provider panic error = %v", err)
		}
	})

	t.Run("error diagnostic panic", func(t *testing.T) {
		err := Run(context.Background(), []string{"serve"}, func() (appkit.Definition, error) {
			return appkit.Definition{}, definitionDiagnosticPanic{}
		}, options)
		if !errors.Is(err, ErrCallbackPanic) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("provider error diagnostic panic = %v", err)
		}
	})

	t.Run("typed nil error", func(t *testing.T) {
		var cause *definitionTypedNilError
		var calls atomic.Int64
		err := Run(context.Background(), []string{"serve"}, func() (appkit.Definition, error) {
			calls.Add(1)
			return definition, cause
		}, options)
		if got := err.Error(); got != "construct application Definition: opaque dependency failure" {
			t.Fatalf("typed-nil provider diagnostic = %q", got)
		}
		if errors.Is(err, ErrCallbackPanic) {
			t.Fatalf("typed-nil provider error was classified as callback panic: %v", err)
		}
		var found *definitionTypedNilError
		if !errors.Is(err, cause) || !errors.As(err, &found) || found != cause {
			t.Fatalf("typed-nil provider identity was not safely preserved: Is=%t As=%t value=%#v",
				errors.Is(err, cause), errors.As(err, &found), found)
		}
		if errors.Is(err, context.Canceled) {
			t.Fatal("typed-nil provider Unwrap was invoked")
		}
		if calls.Load() != 1 {
			t.Fatalf("provider calls = %d, want 1", calls.Load())
		}
		if fixture.starts.Load() != 0 || fixture.cleanups.Load() != 0 {
			t.Fatalf("typed-nil provider failure started=%d cleaned=%d", fixture.starts.Load(), fixture.cleanups.Load())
		}
	})

	t.Run("missing", func(t *testing.T) {
		err := Run(context.Background(), []string{"serve"}, nil, options)
		if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "provider is required") {
			t.Fatalf("missing provider error = %v", err)
		}
	})

	t.Run("metadata drift", func(t *testing.T) {
		drifted := definition
		drifted.Metadata.Name = "Drifted Application"
		var calls atomic.Int64
		err := Run(context.Background(), []string{"serve"}, func() (appkit.Definition, error) {
			calls.Add(1)
			return drifted, nil
		}, options)
		if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "must exactly match") {
			t.Fatalf("metadata drift error = %v", err)
		}
		if calls.Load() != 1 {
			t.Fatalf("provider calls = %d, want 1", calls.Load())
		}
	})

	if fixture.starts.Load() != 0 || fixture.cleanups.Load() != 0 {
		t.Fatalf("provider failures started=%d cleaned=%d", fixture.starts.Load(), fixture.cleanups.Load())
	}
}

func definitionProvider(definition appkit.Definition) DefinitionProvider {
	return func() (appkit.Definition, error) { return definition, nil }
}

type definitionDiagnosticPanic struct{}

func (definitionDiagnosticPanic) Error() string { panic("provider error secret") }

type definitionTypedNilError struct{}

func (*definitionTypedNilError) Error() string { panic("typed-nil provider Error invoked") }
func (*definitionTypedNilError) Is(error) bool { panic("typed-nil provider Is invoked") }
func (*definitionTypedNilError) As(any) bool   { panic("typed-nil provider As invoked") }
func (*definitionTypedNilError) Unwrap() error { return ErrCallbackPanic }

type lifecycleErrorWriter struct{ err error }

func (writer lifecycleErrorWriter) Write([]byte) (int, error) { return 0, writer.err }

type lifecycleShortWriter struct{}

func (lifecycleShortWriter) Write(data []byte) (int, error) { return len(data) / 2, nil }

type lifecyclePanicWriter struct{}

func (lifecyclePanicWriter) Write([]byte) (int, error) { panic("writer secret") }

type lifecycleTypedNilIO struct{}

func (*lifecycleTypedNilIO) Read([]byte) (int, error)  { panic("typed-nil reader invoked") }
func (*lifecycleTypedNilIO) Write([]byte) (int, error) { panic("typed-nil writer invoked") }
func (*lifecycleTypedNilIO) Close() error              { panic("typed-nil closer invoked") }
