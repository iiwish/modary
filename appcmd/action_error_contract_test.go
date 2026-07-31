package appcmd

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
)

func TestRunActionPreservesGovernedBusinessErrors(t *testing.T) {
	tests := []struct {
		name       string
		handlerErr *action.Error
		declared   []action.ErrorSpec
	}{
		{
			name:       "framework business error",
			handlerErr: action.NewError(action.CodeValidationFailed, "message is invalid"),
		},
		{
			name: "consumer business error",
			handlerErr: &action.Error{
				Code: "EXAMPLE.VERSION_CONFLICT", Kind: action.ErrorKindConflict,
				Message: "example version changed",
			},
			declared: []action.ErrorSpec{{Code: "EXAMPLE.VERSION_CONFLICT", Kind: action.ErrorKindConflict}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCLIActionFixture()
			fixture.handler.planErr = test.handlerErr
			definition := fixture.definition()
			definition.Modules[0].Definition.Actions[0].Descriptor.Errors = test.declared
			err := runCLIActionPreviewForErrorContract(t, fixture, definition)

			wantText := "preview Action example.echo: " + test.handlerErr.Code + ": " + test.handlerErr.Message
			if err == nil || err.Error() != wantText {
				t.Fatalf("RunAction() error = %v, want %q", err, wantText)
			}
			var actionErr *action.Error
			if !errors.As(err, &actionErr) || actionErr == nil {
				t.Fatalf("RunAction() error does not retain Action error: %v", err)
			}
			if actionErr.Code != test.handlerErr.Code || actionErr.Kind != test.handlerErr.Kind || actionErr.Message != test.handlerErr.Message {
				t.Fatalf("retained Action error = %#v; source=%#v", actionErr, test.handlerErr)
			}
			if actionErr.ActionID != "example.echo" || actionErr.RequestID != "req_cli_error_contract" {
				t.Fatalf("retained Action request context = %#v", actionErr)
			}
			fixture.assertLifecycle(t, 1, 1, 1)
		})
	}
}

func TestRunActionHidesInternalAndInvalidErrorEnvelopes(t *testing.T) {
	tests := []struct {
		name       string
		handlerErr *action.Error
		declared   []action.ErrorSpec
	}{
		{
			name: "internal error",
			handlerErr: &action.Error{
				Code: action.CodeInternal, Kind: action.ErrorKindInternal,
				Message: "database password secret",
			},
		},
		{
			name: "built-in kind mismatch",
			handlerErr: &action.Error{
				Code: action.CodeValidationFailed, Kind: action.ErrorKindConflict,
				Message: "kind mismatch secret",
			},
		},
		{
			name: "invalid public message",
			handlerErr: &action.Error{
				Code: "EXAMPLE.VERSION_CONFLICT", Kind: action.ErrorKindConflict,
				Message: "line one\nmessage secret",
			},
			declared: []action.ErrorSpec{{Code: "EXAMPLE.VERSION_CONFLICT", Kind: action.ErrorKindConflict}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCLIActionFixture()
			fixture.handler.planErr = test.handlerErr
			definition := fixture.definition()
			definition.Modules[0].Definition.Actions[0].Descriptor.Errors = test.declared
			err := runCLIActionPreviewForErrorContract(t, fixture, definition)

			if err == nil || err.Error() != "preview Action example.echo: Action Runtime preview failed" || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "database") {
				t.Fatalf("RunAction() disclosed private error envelope: %v", err)
			}
			var actionErr *action.Error
			if !errors.As(err, &actionErr) || actionErr == nil || actionErr.Code != action.CodeInternal || actionErr.Kind != action.ErrorKindInternal {
				t.Fatalf("RunAction() retained error = %#v; err=%v", actionErr, err)
			}
			if strings.Contains(actionErr.Message, "secret") || strings.Contains(actionErr.Message, "database") {
				t.Fatalf("normalized Action error disclosed private message: %#v", actionErr)
			}
			fixture.assertLifecycle(t, 1, 1, 1)
		})
	}
}

func runCLIActionPreviewForErrorContract(t *testing.T, fixture *cliActionFixture, definition appkit.Definition) error {
	t.Helper()
	tokenFile := writeCLIActionTokenFile(t, cliActionToken)
	return RunAction(context.Background(), []string{
		"run", "example.echo", "--token-file", tokenFile, "--input", "-", "--preview",
		"--request-id", "req_cli_error_contract",
	}, definition, Options{
		Stdin:  cliInput(strings.NewReader(`{"message":"hello"}`)),
		Stdout: io.Discard,
	})
}
