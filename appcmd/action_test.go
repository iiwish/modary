package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/internal/testsupport"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/scope"
)

const cliActionToken = "cli-test-bearer-token-0000000000000000000000000001"

func TestRunActionPreviewAndExecuteUseIdentityRuntimeAndActorScope(t *testing.T) {
	t.Run("preview", func(t *testing.T) {
		fixture := newCLIActionFixture()
		tokenFile := writeCLIActionTokenFile(t, cliActionToken+"\n")
		var output bytes.Buffer
		err := RunAction(context.Background(), []string{
			"run", "example.echo", "--token-file", tokenFile, "--input", "-", "--preview", "--request-id", "req_cli_preview",
		}, fixture.definition(), Options{Stdin: cliInput(strings.NewReader(`{"message":"hello"}`)), Stdout: &output})
		if err != nil {
			t.Fatalf("RunAction(preview) error = %v", err)
		}
		var preview action.Preview
		if err := json.Unmarshal(output.Bytes(), &preview); err != nil {
			t.Fatalf("decode Preview: %v; output=%q", err, output.String())
		}
		if preview.PlanHash == "" || preview.ExpiresAt.IsZero() || string(preview.Summary) != `{"message":"planned"}` || preview.Impact.Rows != 1 {
			t.Fatalf("Preview output = %#v", preview)
		}
		request := fixture.handler.lastRequest(t)
		assertCLIActionRequest(t, request, "req_cli_preview", "")
		if fixture.tokens.lastToken() != cliActionToken {
			t.Fatalf("authenticated token did not match the token-file content")
		}
		fixture.assertLifecycle(t, 1, 1, 1)
	})

	t.Run("execute", func(t *testing.T) {
		fixture := newCLIActionFixture()
		inputPath := filepath.Join(t.TempDir(), "input.json")
		if err := os.WriteFile(inputPath, []byte(" \n{\"message\":\"from-file\"}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		definition := fixture.definition()
		var providerCalls atomic.Int64
		err := Run(context.Background(), []string{
			"action", "run", "example.echo", "--token-file=-", "--input=" + inputPath,
			"--preview=false", "--idempotency-key=execute-once", "--request-id=req_cli_execute",
		}, func() (appkit.Definition, error) {
			providerCalls.Add(1)
			return definition, nil
		}, Options{
			Metadata: definition.Metadata,
			Stdin:    cliInput(strings.NewReader(cliActionToken + "\r\n")),
			Stdout:   &output,
		})
		if err != nil {
			t.Fatalf("RunAction(execute) error = %v", err)
		}
		if providerCalls.Load() != 1 {
			t.Fatalf("Definition provider calls = %d, want 1", providerCalls.Load())
		}
		var result action.Result
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatalf("decode Result: %v; output=%q", err, output.String())
		}
		if string(result.Data) != `{"echo":"ok"}` || result.Summary != "completed" || len(result.References) != 1 || result.References[0] != (audit.Reference{Kind: "example", ID: "result-1"}) {
			t.Fatalf("Result output = %#v", result)
		}
		request := fixture.handler.lastRequest(t)
		assertCLIActionRequest(t, request, "req_cli_execute", "execute-once")
		plan := fixture.handler.lastPlan(t)
		if plan.Channel != action.ChannelCLI || plan.Scope != fixture.actor.Scope || plan.ActorID != fixture.actor.ID || plan.ActorType != fixture.actor.Type {
			t.Fatalf("execution Plan boundary = %#v", plan)
		}
		fixture.assertLifecycle(t, 1, 1, 1)
	})
}

func TestRunActionPreflightFailuresDoNotStartModules(t *testing.T) {
	fixture := newCLIActionFixture()
	directory := t.TempDir()
	tokenFile := filepath.Join(directory, "token")
	if err := os.WriteFile(tokenFile, []byte(cliActionToken), 0o600); err != nil {
		t.Fatal(err)
	}
	validFile := filepath.Join(directory, "valid.json")
	if err := os.WriteFile(validFile, []byte(`{"message":"ok"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(directory, "input-link.json")
	if err := os.Symlink(validFile, symlink); err != nil {
		t.Fatal(err)
	}
	tokenSymlink := filepath.Join(directory, "token-link")
	if err := os.Symlink(tokenFile, tokenSymlink); err != nil {
		t.Fatal(err)
	}
	emptyToken := filepath.Join(directory, "empty-token")
	if err := os.WriteFile(emptyToken, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	shortToken := filepath.Join(directory, "short-token")
	if err := os.WriteFile(shortToken, []byte("too-short"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalidUTF8Token := filepath.Join(directory, "invalid-token")
	if err := os.WriteFile(invalidUTF8Token, []byte{0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	controlToken := filepath.Join(directory, "control-token")
	if err := os.WriteFile(controlToken, []byte("token\nwith-control"), 0o600); err != nil {
		t.Fatal(err)
	}
	whitespaceToken := filepath.Join(directory, "whitespace-token")
	if err := os.WriteFile(whitespaceToken, []byte("token-with enough bytes but one space"), 0o600); err != nil {
		t.Fatal(err)
	}
	largeToken := filepath.Join(directory, "large-token")
	if err := os.WriteFile(largeToken, []byte(strings.Repeat("x", int(maximumCLITokenBytes)+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	insecureToken := filepath.Join(directory, "insecure-token")
	if err := os.WriteFile(insecureToken, []byte(cliActionToken), 0o644); err != nil {
		t.Fatal(err)
	}
	validHash := "sha256:" + strings.Repeat("a", 64)

	tests := []struct {
		name    string
		args    []string
		options Options
	}{
		{name: "missing subcommand"},
		{name: "unknown subcommand", args: []string{"inspect"}},
		{name: "missing action id", args: []string{"run"}},
		{name: "invalid action id", args: []string{"run", "Example.Echo", "--token-file", tokenFile, "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(`{}`))}},
		{name: "path action id", args: []string{"run", "example/../echo", "--token-file", tokenFile, "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(`{}`))}},
		{name: "missing token file", args: []string{"run", "example.echo", "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(`{}`))}},
		{name: "missing input", args: []string{"run", "example.echo", "--token-file", tokenFile}},
		{name: "duplicate token file", args: []string{"run", "example.echo", "--token-file", tokenFile, "--token-file", tokenFile, "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(`{}`))}},
		{name: "stdin conflict", args: []string{"run", "example.echo", "--token-file", "-", "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(cliActionToken))}},
		{name: "unknown flag", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-", "--raw"}, options: Options{Stdin: cliInput(strings.NewReader(`{}`))}},
		{name: "single dash flag", args: []string{"run", "example.echo", "-token-file", tokenFile, "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(`{}`))}},
		{name: "invalid plan", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-", "--plan", "SHA256:bad"}, options: Options{Stdin: cliInput(strings.NewReader(`{}`))}},
		{name: "preview plan conflict", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-", "--preview", "--plan", validHash}, options: Options{Stdin: cliInput(strings.NewReader(`{}`))}},
		{name: "missing token", args: []string{"run", "example.echo", "--token-file", filepath.Join(directory, "missing-token"), "--input", validFile}},
		{name: "directory token", args: []string{"run", "example.echo", "--token-file", directory, "--input", validFile}},
		{name: "symlink token", args: []string{"run", "example.echo", "--token-file", tokenSymlink, "--input", validFile}},
		{name: "empty token", args: []string{"run", "example.echo", "--token-file", emptyToken, "--input", validFile}},
		{name: "short token", args: []string{"run", "example.echo", "--token-file", shortToken, "--input", validFile}},
		{name: "invalid UTF-8 token", args: []string{"run", "example.echo", "--token-file", invalidUTF8Token, "--input", validFile}},
		{name: "control token", args: []string{"run", "example.echo", "--token-file", controlToken, "--input", validFile}},
		{name: "whitespace token", args: []string{"run", "example.echo", "--token-file", whitespaceToken, "--input", validFile}},
		{name: "large token", args: []string{"run", "example.echo", "--token-file", largeToken, "--input", validFile}},
		{name: "missing file", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", filepath.Join(directory, "missing.json")}},
		{name: "directory input", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", directory}},
		{name: "symlink input", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", symlink}},
		{name: "empty JSON", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(" \n"))}},
		{name: "multiple JSON", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(`{} {}`))}},
		{name: "nested duplicate", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(`{"nested":{"id":1,"id":2}}`))}},
		{name: "invalid UTF-8", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-"}, options: Options{Stdin: cliInput(bytes.NewReader([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}))}},
		{name: "too large", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(`{"message":"too large"}`)), MaxActionInputBytes: 8}},
		{name: "too deep", args: []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-"}, options: Options{Stdin: cliInput(strings.NewReader(strings.Repeat("[", action.MaxJSONNestingDepth+1) + "0" + strings.Repeat("]", action.MaxJSONNestingDepth+1)))}},
	}
	if cliTokenFilePermissionsEnforced {
		tests = append(tests, struct {
			name    string
			args    []string
			options Options
		}{name: "insecure token mode", args: []string{"run", "example.echo", "--token-file", insecureToken, "--input", validFile}})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.options.Stdout = io.Discard
			err := RunAction(context.Background(), test.args, fixture.definition(), test.options)
			if err == nil || !errors.Is(err, ErrUsage) {
				t.Fatalf("RunAction() error = %v, want ErrUsage", err)
			}
			if strings.Contains(err.Error(), cliActionToken) {
				t.Fatalf("RunAction() error disclosed token content: %v", err)
			}
			fixture.assertLifecycle(t, 0, 0, 0)
		})
	}
}

func TestActionDependencyBoundariesFailClosedOnTypedNilErrors(t *testing.T) {
	var typedNil *lifecycleTypedNilDependencyError
	actor := identity.Actor{ID: "typed-nil-user", Type: "user", Scope: scope.Must("tenant", "acme")}

	authenticated, err := callTokenAuthenticator(context.Background(), cliTypedNilAuthenticator{actor: actor, err: typedNil}, "token")
	if authenticated != (identity.Actor{}) {
		t.Fatalf("typed-nil authenticator error retained actor = %#v", authenticated)
	}
	assertTypedNilDependencyFailure(t, err, typedNil)

	runtime := cliTypedNilRuntime{err: typedNil}
	preview, err := callActionPreview(context.Background(), runtime, action.Request{})
	if preview.PlanHash != "" || preview.Summary != nil || preview.Impact.Rows != 0 ||
		preview.Impact.Resources != nil || !preview.ExpiresAt.IsZero() {
		t.Fatalf("typed-nil preview error retained result = %#v", preview)
	}
	assertTypedNilDependencyFailure(t, err, typedNil)
	result, err := callActionExecute(context.Background(), runtime, action.Request{})
	if result.Summary != "" || result.Data != nil || result.References != nil {
		t.Fatalf("typed-nil execute error retained result = %#v", result)
	}
	assertTypedNilDependencyFailure(t, err, typedNil)

	input := &actionInput{ReadCloser: cliTypedNilInput{err: typedNil}}
	assertTypedNilDependencyFailure(t, input.Close(), typedNil)
	assertTypedNilDependencyFailure(t, input.Close(), typedNil)
	if _, err := readLimitedActionInput(cliTypedNilInput{err: typedNil}, MaximumActionInputBytes); err == nil {
		t.Fatal("typed-nil Action input read error was treated as success")
	} else {
		assertTypedNilDependencyFailure(t, err, typedNil)
	}
}

func TestActionTokenFilePathHasCredentialSpecificDiagnostic(t *testing.T) {
	_, _, err := parseActionCommand([]string{"run", "example.echo", "--token-file", " invalid", "--input", "input.json"})
	if err == nil || !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "CLI token file path") || strings.Contains(err.Error(), "Action input path") {
		t.Fatalf("token-file path error = %v", err)
	}
}

func TestRunActionContextAndInputReaderBoundaries(t *testing.T) {
	fixture := newCLIActionFixture()
	tokenFile := writeCLIActionTokenFile(t, cliActionToken)
	validArgs := []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-"}
	if err := RunAction(nil, validArgs, fixture.definition(), Options{Stdin: cliInput(strings.NewReader(`{}`)), Stdout: io.Discard}); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("RunAction(nil) error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RunAction(canceled, validArgs, fixture.definition(), Options{Stdin: cliInput(strings.NewReader(`{"message":"ok"}`)), Stdout: io.Discard}); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunAction(canceled) error = %v", err)
	}
	fixture.assertLifecycle(t, 0, 0, 0)

	counting := &cliActionCountingReader{data: []byte(strings.Repeat("x", 100))}
	err := RunAction(context.Background(), validArgs, fixture.definition(), Options{Stdin: counting, Stdout: io.Discard, MaxActionInputBytes: 10})
	if err == nil || !errors.Is(err, ErrUsage) || counting.read.Load() != 11 {
		t.Fatalf("limited stdin error = %v, bytes read = %d", err, counting.read.Load())
	}
	fixture.assertLifecycle(t, 0, 0, 0)

	err = RunAction(context.Background(), validArgs, fixture.definition(), Options{Stdin: cliActionPanicReader{}, Stdout: io.Discard})
	if err == nil || !errors.Is(err, ErrUsage) || !errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("panic stdin error = %v", err)
	}
	fixture.assertLifecycle(t, 0, 0, 0)

	ctx, cancelRead := context.WithCancel(context.Background())
	blocking := newCLIActionBlockingInput()
	done := make(chan error, 1)
	go func() {
		done <- RunAction(ctx, validArgs, fixture.definition(), Options{Stdin: blocking, Stdout: io.Discard})
	}()
	select {
	case <-blocking.started:
	case <-time.After(time.Second):
		t.Fatal("RunAction did not begin reading stdin")
	}
	cancelRead()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunAction cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunAction did not interrupt stdin after cancellation")
	}
	if calls := blocking.closeCalls.Load(); calls != 1 {
		t.Fatalf("stdin Close calls = %d, want 1", calls)
	}
	fixture.assertLifecycle(t, 0, 0, 0)
}

func TestRunActionJoinsFailuresWithIndependentBoundedShutdown(t *testing.T) {
	primaryFailure := errors.New("primary Action failure")
	cleanupFailure := errors.New("cleanup failure")
	tokenFile := writeCLIActionTokenFile(t, cliActionToken)
	validArgs := []string{"run", "example.echo", "--token-file", tokenFile, "--input", "-"}

	tests := []struct {
		name      string
		configure func(*cliActionFixture, *Options)
		wantPanic bool
	}{
		{
			name: "token authentication error",
			configure: func(fixture *cliActionFixture, _ *Options) {
				fixture.tokens.err = primaryFailure
			},
		},
		{
			name: "token authentication panic",
			configure: func(fixture *cliActionFixture, _ *Options) {
				fixture.tokens.panics = true
			},
			wantPanic: true,
		},
		{
			name: "runtime error",
			configure: func(fixture *cliActionFixture, _ *Options) {
				fixture.handler.planErr = primaryFailure
			},
		},
		{
			name: "output error",
			configure: func(_ *cliActionFixture, options *Options) {
				options.Stdout = cliActionErrorWriter{err: primaryFailure}
			},
		},
		{
			name: "output panic",
			configure: func(_ *cliActionFixture, options *Options) {
				options.Stdout = cliActionPanicWriter{}
			},
			wantPanic: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCLIActionFixture()
			fixture.cleanupErr = cleanupFailure
			options := Options{Stdin: cliInput(strings.NewReader(`{"message":"ok"}`)), Stdout: io.Discard}
			test.configure(fixture, &options)
			err := RunAction(context.Background(), validArgs, fixture.definition(), options)
			if err == nil || !errors.Is(err, cleanupFailure) {
				t.Fatalf("RunAction() error = %v, want joined cleanup failure", err)
			}
			if test.wantPanic {
				if !errors.Is(err, ErrCallbackPanic) {
					t.Fatalf("RunAction() error = %v, want callback panic", err)
				}
			} else if !errors.Is(err, primaryFailure) {
				t.Fatalf("RunAction() error = %v, want primary failure", err)
			}
			if !fixture.cleanupSawLive.Load() {
				t.Fatal("cleanup inherited a canceled or expired request context")
			}
			fixture.assertLifecycle(t, 1, 1, 1)
		})
	}

	t.Run("caller cancellation does not cancel cleanup", func(t *testing.T) {
		fixture := newCLIActionFixture()
		ctx, cancel := context.WithCancel(context.Background())
		fixture.tokens.cancel = cancel
		fixture.tokens.err = primaryFailure
		err := RunAction(ctx, validArgs, fixture.definition(), Options{Stdin: cliInput(strings.NewReader(`{"message":"ok"}`)), Stdout: io.Discard})
		if err == nil || !errors.Is(err, primaryFailure) || !fixture.cleanupSawLive.Load() {
			t.Fatalf("RunAction() error = %v, cleanup live = %v", err, fixture.cleanupSawLive.Load())
		}
		fixture.assertLifecycle(t, 1, 1, 1)
	})

	t.Run("authentication error text cannot disclose the token", func(t *testing.T) {
		fixture := newCLIActionFixture()
		fixture.tokens.err = fmt.Errorf("credential %s rejected: %w", cliActionToken, primaryFailure)
		err := RunAction(context.Background(), validArgs, fixture.definition(), Options{Stdin: cliInput(strings.NewReader(`{"message":"ok"}`)), Stdout: io.Discard})
		if err == nil || !errors.Is(err, primaryFailure) || strings.Contains(err.Error(), cliActionToken) {
			t.Fatalf("RunAction() authentication error = %v", err)
		}
		fixture.assertLifecycle(t, 1, 1, 1)
	})

	t.Run("shutdown timeout", func(t *testing.T) {
		fixture := newCLIActionFixture()
		fixture.cleanupWaitForContext = true
		started := time.Now()
		err := RunAction(context.Background(), validArgs, fixture.definition(), Options{
			Stdin:           cliInput(strings.NewReader(`{"message":"ok"}`)),
			Stdout:          io.Discard,
			ShutdownTimeout: 40 * time.Millisecond,
			App: appkit.Options{Shutdown: module.ShutdownPolicy{
				CallbackTimeout: 20 * time.Millisecond,
			}},
		})
		if err == nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("RunAction() error = %v, want cleanup deadline", err)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("bounded shutdown took %s", elapsed)
		}
		fixture.assertLifecycle(t, 1, 1, 1)
	})
}

func TestRunActionRejectsMalformedAuthenticatedActorBeforeRuntime(t *testing.T) {
	fixture := newCLIActionFixture()
	fixture.tokens.actor = identity.Actor{ID: "", Type: "user", Scope: scope.Must("tenant", "acme")}
	tokenFile := writeCLIActionTokenFile(t, cliActionToken)
	err := RunAction(context.Background(), []string{
		"run", "example.echo", "--token-file", tokenFile, "--input", "-",
	}, fixture.definition(), Options{Stdin: cliInput(strings.NewReader(`{"message":"ok"}`)), Stdout: io.Discard})
	if err == nil || err.Error() != "authenticate CLI token failed" {
		t.Fatalf("malformed Actor error = %v", err)
	}
	fixture.handler.mu.Lock()
	runtimeCalls := len(fixture.handler.requests) + len(fixture.handler.plans)
	fixture.handler.mu.Unlock()
	if runtimeCalls != 0 {
		t.Fatalf("Runtime calls after malformed Actor = %d", runtimeCalls)
	}
	fixture.assertLifecycle(t, 1, 1, 1)
}

func TestRunActionHelpIsPureAndOutputPanicsAreContained(t *testing.T) {
	fixture := newCLIActionFixture()
	var output bytes.Buffer
	if err := RunAction(context.Background(), []string{"--help"}, fixture.definition(), Options{Stdout: &output}); err != nil {
		t.Fatalf("RunAction(help) error = %v", err)
	}
	if !strings.Contains(output.String(), "example-app action run") || !strings.Contains(output.String(), "--token-file <path|->") || strings.Contains(output.String(), "--actor") {
		t.Fatalf("help output = %q", output.String())
	}
	if err := RunAction(context.Background(), []string{"run", "--help"}, fixture.definition(), Options{Stdout: cliActionPanicWriter{}}); !errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("panic help writer error = %v", err)
	}
	fixture.assertLifecycle(t, 0, 0, 0)
}

func assertCLIActionRequest(t *testing.T, request action.Request, requestID, idempotencyKey string) {
	t.Helper()
	if request.RequestID != requestID || request.Actor.ID != "cli-user" || request.Channel != action.ChannelCLI || request.ActionID != "example.echo" ||
		request.Scope != request.Actor.Scope || request.Scope != scope.Must("tenant", "acme") || request.IdempotencyKey != idempotencyKey {
		t.Fatalf("Action Request boundary = %#v", request)
	}
}

type cliActionFixture struct {
	starts                atomic.Int32
	factories             atomic.Int32
	cleanups              atomic.Int32
	cleanupSawLive        atomic.Bool
	cleanupErr            error
	cleanupWaitForContext bool
	actor                 identity.Actor
	tokens                *cliActionTokens
	handler               *cliActionHandler
}

func newCLIActionFixture() *cliActionFixture {
	actor := identity.Actor{ID: "cli-user", Type: "user", DisplayName: "CLI User", Scope: scope.Must("tenant", "acme")}
	return &cliActionFixture{
		actor:   actor,
		tokens:  &cliActionTokens{actor: actor},
		handler: &cliActionHandler{},
	}
}

func (fixture *cliActionFixture) definition() appkit.Definition {
	descriptor := action.Descriptor{
		ID:      "example.echo",
		Version: "1.2.3",
		Title:   "Echo",
		InputSchema: action.Object(map[string]action.Field{
			"message": action.RequiredField(action.String()),
		}).JSON(),
		PreviewSchema: action.Object(map[string]action.Field{
			"message": action.RequiredField(action.String()),
		}).JSON(),
		OutputSchema: action.Object(map[string]action.Field{
			"echo": action.RequiredField(action.String()),
		}).JSON(),
		Permission: "example.echo",
		Preview:    action.PreviewOptional,
		AuditLevel: action.AuditDetailed,
		Channels:   []action.Channel{action.ChannelCLI},
	}
	registration := module.Registration{
		Definition: module.Definition{
			Manifest: module.Manifest{
				SchemaVersion: module.SchemaVersion,
				ID:            "cli-runtime",
				Version:       "1.0.0",
				Type:          module.ModuleTypeAdapter,
				Provides: []module.Capability{
					module.CapabilityAudit,
					module.CapabilityAuthorization,
					module.CapabilityDatabase,
					module.CapabilityIdentity,
				},
			},
			Actions: []module.ActionBinding{{
				Descriptor: descriptor,
				NewHandler: func(context.Context, module.Resolver) (action.Handler, error) {
					fixture.factories.Add(1)
					return fixture.handler, nil
				},
			}},
		},
		Start: func(_ context.Context, install module.Scope) error {
			fixture.starts.Add(1)
			if err := module.OnStop(install, func(ctx context.Context) error {
				fixture.cleanups.Add(1)
				fixture.cleanupSawLive.Store(ctx.Err() == nil)
				if fixture.cleanupWaitForContext {
					<-ctx.Done()
					return ctx.Err()
				}
				return fixture.cleanupErr
			}); err != nil {
				return err
			}
			services := []error{
				module.Provide(install, module.Authorizer(), authz.Authorizer(cliActionAuthorizer{})),
				module.Provide(install, module.AuditHook(), audit.Hook(testsupport.DiscardAudit{})),
				moduleassembly.ProvideActionPersistence(install, testsupport.NewMemoryPlanStore(), testsupport.NewMemoryIdempotencyStore(), testsupport.DirectTransactions{}),
				module.Provide(install, module.TokenAuthenticator(), identity.TokenAuthenticator(fixture.tokens)),
			}
			return errors.Join(services...)
		},
	}
	return appkit.Definition{
		Metadata: appkit.Metadata{ID: "example-app", Name: "Example App", Version: "1.2.3"},
		Modules:  []module.Registration{registration},
	}
}

func (fixture *cliActionFixture) assertLifecycle(t *testing.T, starts, factories, cleanups int32) {
	t.Helper()
	if fixture.starts.Load() != starts || fixture.factories.Load() != factories || fixture.cleanups.Load() != cleanups {
		t.Fatalf("lifecycle starts=%d factories=%d cleanups=%d, want %d/%d/%d",
			fixture.starts.Load(), fixture.factories.Load(), fixture.cleanups.Load(), starts, factories, cleanups)
	}
}

type cliActionTokens struct {
	mu     sync.Mutex
	actor  identity.Actor
	token  string
	err    error
	panics bool
	cancel context.CancelFunc
}

func (authenticator *cliActionTokens) AuthenticateToken(_ context.Context, token string) (identity.Actor, error) {
	authenticator.mu.Lock()
	authenticator.token = token
	actor := authenticator.actor
	err := authenticator.err
	panics := authenticator.panics
	cancel := authenticator.cancel
	authenticator.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if panics {
		panic("token secret panic")
	}
	if err != nil {
		return identity.Actor{}, err
	}
	return actor, nil
}

func (authenticator *cliActionTokens) lastToken() string {
	authenticator.mu.Lock()
	defer authenticator.mu.Unlock()
	return authenticator.token
}

func writeCLIActionTokenFile(t *testing.T, token string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(name, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}

type cliActionHandler struct {
	mu         sync.Mutex
	requests   []action.Request
	plans      []action.Plan
	planErr    error
	executeErr error
}

func (handler *cliActionHandler) Plan(_ context.Context, request action.Request) (action.PlanData, error) {
	handler.mu.Lock()
	handler.requests = append(handler.requests, request)
	err := handler.planErr
	handler.mu.Unlock()
	if err != nil {
		return action.PlanData{}, err
	}
	return action.PlanData{
		Payload: request.Input,
		Summary: json.RawMessage(`{"message":"planned"}`),
		Impact:  authz.Impact{Rows: 1, Resources: []string{"example/result-1"}},
	}, nil
}

func (handler *cliActionHandler) Execute(_ context.Context, plan action.Plan) (action.Result, error) {
	handler.mu.Lock()
	handler.plans = append(handler.plans, plan)
	err := handler.executeErr
	handler.mu.Unlock()
	if err != nil {
		return action.Result{}, err
	}
	return action.Result{
		Data:       json.RawMessage(`{"echo":"ok"}`),
		Summary:    "completed",
		References: []audit.Reference{{Kind: "example", ID: "result-1"}},
	}, nil
}

func (handler *cliActionHandler) lastRequest(t *testing.T) action.Request {
	t.Helper()
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.requests) == 0 {
		t.Fatal("Action Handler did not receive a Plan request")
	}
	return handler.requests[len(handler.requests)-1]
}

func (handler *cliActionHandler) lastPlan(t *testing.T) action.Plan {
	t.Helper()
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.plans) == 0 {
		t.Fatal("Action Handler did not receive an execution Plan")
	}
	return handler.plans[len(handler.plans)-1]
}

type cliActionAuthorizer struct{}

func (cliActionAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Allowed: true, Fingerprint: "cli-allow"}, nil
}

type cliActionCountingReader struct {
	data []byte
	read atomic.Int64
}

func (reader *cliActionCountingReader) Read(destination []byte) (int, error) {
	offset := int(reader.read.Load())
	if offset >= len(reader.data) {
		return 0, io.EOF
	}
	count := copy(destination, reader.data[offset:])
	reader.read.Add(int64(count))
	return count, nil
}

func (*cliActionCountingReader) Close() error { return nil }

type cliActionPanicReader struct{}

func (cliActionPanicReader) Read([]byte) (int, error) { panic("reader secret panic") }
func (cliActionPanicReader) Close() error             { return nil }

func cliInput(reader io.Reader) io.ReadCloser { return io.NopCloser(reader) }

type cliActionBlockingInput struct {
	started    chan struct{}
	closed     chan struct{}
	startOnce  sync.Once
	closeOnce  sync.Once
	closeCalls atomic.Int32
}

func newCLIActionBlockingInput() *cliActionBlockingInput {
	return &cliActionBlockingInput{started: make(chan struct{}), closed: make(chan struct{})}
}

func (input *cliActionBlockingInput) Read([]byte) (int, error) {
	input.startOnce.Do(func() { close(input.started) })
	<-input.closed
	return 0, errors.New("stdin was closed")
}

func (input *cliActionBlockingInput) Close() error {
	input.closeCalls.Add(1)
	input.closeOnce.Do(func() { close(input.closed) })
	return nil
}

type cliActionErrorWriter struct{ err error }

func (writer cliActionErrorWriter) Write([]byte) (int, error) { return 0, writer.err }

type cliActionPanicWriter struct{}

func (cliActionPanicWriter) Write([]byte) (int, error) { panic("writer secret panic") }

type cliTypedNilAuthenticator struct {
	actor identity.Actor
	err   error
}

func (authenticator cliTypedNilAuthenticator) AuthenticateToken(context.Context, string) (identity.Actor, error) {
	return authenticator.actor, authenticator.err
}

type cliTypedNilRuntime struct{ err error }

func (runtime cliTypedNilRuntime) Preview(context.Context, action.Request) (action.Preview, error) {
	return action.Preview{PlanHash: "unexpected"}, runtime.err
}

func (runtime cliTypedNilRuntime) Execute(context.Context, action.Request) (action.Result, error) {
	return action.Result{Summary: "unexpected"}, runtime.err
}

func (cliTypedNilRuntime) CleanupExpiredPlans(context.Context) (int64, error) { return 0, nil }

type cliTypedNilInput struct{ err error }

func (input cliTypedNilInput) Read([]byte) (int, error) { return 0, input.err }
func (input cliTypedNilInput) Close() error             { return input.err }
