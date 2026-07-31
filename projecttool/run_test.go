package projecttool

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/iiwish/modary/appkit"
)

func TestRunParsesHelpAndInvalidCommandsBeforeProjectOrDefinition(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "missing")
	var providerCalls atomic.Int64
	provider := func() (appkit.Definition, error) {
		providerCalls.Add(1)
		panic("provider must not run")
	}
	for _, args := range [][]string{{"unknown"}, {"verify", "extra"}, {"generate", "--write"}, {"help", "extra"}} {
		if err := Run(context.Background(), args, provider, Options{Root: missingRoot, Stdout: io.Discard, Stderr: io.Discard}); !errors.Is(err, ErrUsage) {
			t.Fatalf("Run(%v) error = %v, want ErrUsage", args, err)
		}
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("provider calls = %d", providerCalls.Load())
	}

	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}} {
		var output bytes.Buffer
		if err := Run(context.Background(), args, provider, Options{Root: missingRoot, Stdout: &output, Stderr: io.Discard}); err != nil {
			t.Fatalf("Run(%v): %v", args, err)
		}
		if output.String() != CommandUsage {
			t.Fatalf("help output = %q", output.String())
		}
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("provider calls after help = %d", providerCalls.Load())
	}
}

func TestRunValidatesManifestBeforeDefinitionProvider(t *testing.T) {
	root := writeFixtureProject(t, "application: [invalid\n")
	var providerCalls atomic.Int64
	err := Run(context.Background(), []string{"verify"}, func() (appkit.Definition, error) {
		providerCalls.Add(1)
		return fixtureDefinition(&inspectionCounters{}, false), nil
	}, Options{Root: root, Stdout: io.Discard, Stderr: io.Discard})
	if err == nil {
		t.Fatal("Run accepted malformed manifest")
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("provider calls = %d, want 0", providerCalls.Load())
	}
	if entries, readErr := os.ReadDir(root); readErr != nil || len(entries) != 1 {
		t.Fatalf("invalid manifest caused writes: %v, %v", entries, readErr)
	}
}

func TestRunDispatchesVerifyGenerateAndReadOnlyCheck(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	counters := &inspectionCounters{}
	var providerCalls atomic.Int64
	provider := func() (appkit.Definition, error) {
		providerCalls.Add(1)
		return fixtureDefinition(counters, providerCalls.Load()%2 == 0), nil
	}

	var verifyOutput bytes.Buffer
	if err := Run(context.Background(), []string{"verify"}, provider, Options{Root: root, Stdout: &verifyOutput, Stderr: io.Discard}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got := verifyOutput.String(); !strings.Contains(got, `"modules":2`) || !strings.Contains(got, `"actions":2`) {
		t.Fatalf("verify output = %s", got)
	}

	var generateOutput bytes.Buffer
	if err := Run(context.Background(), []string{"generate"}, provider, Options{Root: root, Stdout: &generateOutput, Stderr: io.Discard}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(generateOutput.String(), `"written"`) {
		t.Fatalf("generate output = %s", generateOutput.String())
	}

	for _, args := range [][]string{{"generate", "--check"}, {"check"}} {
		var checkOutput bytes.Buffer
		before := snapshotTree(t, root)
		if err := Run(context.Background(), args, provider, Options{Root: root, Stdout: &checkOutput, Stderr: io.Discard}); err != nil {
			t.Fatalf("Run(%v): %v", args, err)
		}
		if checkOutput.String() != "{\"current\":true}\n" {
			t.Fatalf("check output = %q", checkOutput.String())
		}
		if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
			t.Fatalf("Run(%v) check mutated files", args)
		}
	}
	if providerCalls.Load() != 4 {
		t.Fatalf("provider calls = %d, want 4", providerCalls.Load())
	}
	assertNoInspectionSideEffects(t, counters)
}

func TestRunReturnsDeterministicDriftAndContainsCallbacks(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	err := Run(context.Background(), []string{"check"}, func() (appkit.Definition, error) {
		return fixtureDefinition(&inspectionCounters{}, false), nil
	}, Options{Root: root, Stdout: io.Discard, Stderr: io.Discard})
	if !errors.Is(err, ErrDrift) {
		t.Fatalf("check error = %v, want ErrDrift", err)
	}
	var drift *DriftError
	if !errors.As(err, &drift) || len(drift.Items) != 3 {
		t.Fatalf("drift = %#v", drift)
	}
	for index := 1; index < len(drift.Items); index++ {
		if drift.Items[index-1].Path > drift.Items[index].Path {
			t.Fatalf("drift is not sorted: %#v", drift.Items)
		}
	}

	providerCause := errors.New("configure SQLite: database path is required")
	var providerCalls atomic.Int64
	err = Run(context.Background(), []string{"verify"}, func() (appkit.Definition, error) {
		providerCalls.Add(1)
		return fixtureDefinition(&inspectionCounters{}, false), providerCause
	}, Options{Root: root, Stdout: io.Discard, Stderr: io.Discard})
	if !errors.Is(err, providerCause) {
		t.Fatalf("provider error = %v, want provider cause", err)
	}
	if text := err.Error(); !strings.Contains(text, "construct application Definition") ||
		!strings.Contains(text, providerCause.Error()) {
		t.Fatalf("provider diagnostic = %q", text)
	}
	if providerCalls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", providerCalls.Load())
	}

	err = Run(context.Background(), []string{"verify"}, func() (appkit.Definition, error) {
		panic("secret panic value")
	}, Options{Root: root, Stdout: io.Discard, Stderr: io.Discard})
	if !errors.Is(err, ErrCallbackPanic) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("provider panic error = %v", err)
	}

	err = Run(context.Background(), []string{"verify"}, func() (appkit.Definition, error) {
		panic(nil)
	}, Options{Root: root, Stdout: io.Discard, Stderr: io.Discard})
	if !errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("nil provider panic error = %v", err)
	}

	err = Run(context.Background(), []string{"verify"}, func() (appkit.Definition, error) {
		return appkit.Definition{}, providerDiagnosticPanic{}
	}, Options{Root: root, Stdout: io.Discard, Stderr: io.Discard})
	if !errors.Is(err, ErrCallbackPanic) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("provider error diagnostic panic = %v", err)
	}

	var typedNilProviderCause *definitionTypedNilError
	typedNilCounters := &inspectionCounters{}
	var typedNilCalls atomic.Int64
	var typedNilOutput bytes.Buffer
	beforeTypedNil := snapshotTree(t, root)
	err = Run(context.Background(), []string{"generate"}, func() (appkit.Definition, error) {
		typedNilCalls.Add(1)
		return fixtureDefinition(typedNilCounters, false), typedNilProviderCause
	}, Options{Root: root, Stdout: &typedNilOutput, Stderr: io.Discard})
	if got := err.Error(); got != "construct application Definition: opaque dependency failure" {
		t.Fatalf("typed-nil provider diagnostic = %q", got)
	}
	if errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("typed-nil provider error was classified as callback panic: %v", err)
	}
	var foundTypedNil *definitionTypedNilError
	if !errors.Is(err, typedNilProviderCause) || !errors.As(err, &foundTypedNil) || foundTypedNil != typedNilProviderCause {
		t.Fatalf("typed-nil provider identity was not safely preserved: Is=%t As=%t value=%#v",
			errors.Is(err, typedNilProviderCause), errors.As(err, &foundTypedNil), foundTypedNil)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal("typed-nil provider Unwrap was invoked")
	}
	if typedNilCalls.Load() != 1 {
		t.Fatalf("typed-nil provider calls = %d, want 1", typedNilCalls.Load())
	}
	if typedNilOutput.Len() != 0 {
		t.Fatalf("typed-nil provider wrote output %q", typedNilOutput.String())
	}
	if afterTypedNil := snapshotTree(t, root); !reflect.DeepEqual(afterTypedNil, beforeTypedNil) {
		t.Fatal("typed-nil provider failure changed project files")
	}
	assertNoInspectionSideEffects(t, typedNilCounters)

	err = Run(context.Background(), []string{"help"}, nil, Options{Stdout: panicWriter{}, Stderr: io.Discard})
	if !errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("output panic error = %v", err)
	}

	var typedNil *bytes.Buffer
	err = Run(context.Background(), []string{"help"}, nil, Options{Stdout: typedNil})
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("typed nil output error = %v", err)
	}

	var typedNilCause *nilWriteError
	err = Run(context.Background(), []string{"help"}, nil, Options{Stdout: typedNilErrorWriter{}, Stderr: io.Discard})
	assertTypedNilWriterFailure(t, err, typedNilCause)
	err = Run(context.Background(), []string{"help"}, nil, Options{Stdout: typedNilInvalidCountWriter{}, Stderr: io.Discard})
	assertTypedNilWriterFailure(t, err, typedNilCause)
	if err := Run(context.Background(), []string{"help"}, nil, Options{Stdout: invalidCountWriter{}, Stderr: io.Discard}); err == nil {
		t.Fatal("invalid writer count was accepted")
	}
}

type nilWriteError struct{}

type providerDiagnosticPanic struct{}

func (providerDiagnosticPanic) Error() string { panic("provider error secret") }

type definitionTypedNilError struct{}

func (*definitionTypedNilError) Error() string { panic("typed-nil provider Error invoked") }
func (*definitionTypedNilError) Is(error) bool { panic("typed-nil provider Is invoked") }
func (*definitionTypedNilError) As(any) bool   { panic("typed-nil provider As invoked") }
func (*definitionTypedNilError) Unwrap() error { return ErrCallbackPanic }

func (*nilWriteError) Error() string { panic("typed-nil writer Error invoked") }
func (*nilWriteError) Is(error) bool { panic("typed-nil writer Is invoked") }
func (*nilWriteError) As(any) bool   { panic("typed-nil writer As invoked") }
func (*nilWriteError) Unwrap() error { panic("typed-nil writer Unwrap invoked") }

type typedNilErrorWriter struct{}

func (typedNilErrorWriter) Write(data []byte) (int, error) {
	var err *nilWriteError
	return len(data), err
}

type typedNilInvalidCountWriter struct{}

func (typedNilInvalidCountWriter) Write(data []byte) (int, error) {
	var err *nilWriteError
	return len(data) + 1, err
}

type invalidCountWriter struct{}

func (invalidCountWriter) Write(data []byte) (int, error) { return len(data) + 1, nil }

func assertTypedNilWriterFailure(t *testing.T, err error, cause *nilWriteError) {
	t.Helper()
	if err == nil {
		t.Fatal("typed-nil writer error was treated as success")
	}
	if got := err.Error(); got == "" || strings.Contains(got, "typed-nil writer") {
		t.Fatalf("typed-nil writer diagnostic = %q", got)
	}
	var found *nilWriteError
	if !errors.Is(err, cause) || !errors.As(err, &found) || found != cause {
		t.Fatalf("typed-nil writer cause was not safely preserved: Is=%t As=%t value=%#v",
			errors.Is(err, cause), errors.As(err, &found), found)
	}
}

func TestRunDispatchesGoOnlyBuild(t *testing.T) {
	requireSecureBuildPlatformForTest(t)
	root := writeFixtureProject(t, validProjectManifest)
	provider := func() (appkit.Definition, error) {
		return fixtureDefinition(&inspectionCounters{}, false), nil
	}
	createFixtureBuildPackage(t, loadFixtureProject(t, root))
	if err := Run(context.Background(), []string{"generate"}, provider, Options{Root: root, Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}
	tooling := installFakeBuildTools(t, `
output=""
: > "$MODARY_GO_LOG"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi
done
printf 'dispatcher binary' > "$output"
`)
	var output bytes.Buffer
	if err := Run(context.Background(), []string{"build"}, provider, Options{Root: root, Stdout: &output, Stderr: io.Discard}); err != nil {
		t.Fatalf("build: %v", err)
	}
	if output.String() != "{\"output\":\"dist/example-app\"}\n" {
		t.Fatalf("build output = %q", output.String())
	}
	if _, err := os.Stat(tooling.goLog); err != nil {
		t.Fatalf("Go was not called: %v", err)
	}
	if _, err := os.Stat(tooling.frontendMarker); !os.IsNotExist(err) {
		t.Fatalf("frontend command was called: %v", err)
	}
}

func TestRunRejectsNilOrCanceledContextAndMissingProvider(t *testing.T) {
	if err := Run(nil, []string{"help"}, nil, Options{}); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("nil context error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, []string{"help"}, nil, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v", err)
	}
	root := writeFixtureProject(t, validProjectManifest)
	if err := Run(context.Background(), []string{"verify"}, nil, Options{Root: root, Stdout: io.Discard, Stderr: io.Discard}); !errors.Is(err, ErrUsage) {
		t.Fatalf("missing Definition provider error = %v, want ErrUsage", err)
	}
}
