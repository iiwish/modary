package appcmd

import (
	"errors"
	"testing"
)

func TestCommandErrorsAreTypedNilSafeAndDoNotFormatCauses(t *testing.T) {
	var command *commandError
	if got := command.Error(); got != "application command failed" || command.Unwrap() != nil {
		t.Fatalf("typed-nil command error = %q, unwrap=%v", got, command.Unwrap())
	}

	cause := commandPanicFormattingError{}
	redacted := &commandError{message: "authenticate CLI token failed", cause: cause}
	if got := redacted.Error(); got != "authenticate CLI token failed" {
		t.Fatalf("redacted command error = %q", got)
	}
	if !errors.Is(redacted, cause) {
		t.Fatal("command error did not retain its inspectable cause")
	}

	var callback *CallbackPanicError
	if got := callback.Error(); got != "application command callback panicked" || !errors.Is(callback, ErrCallbackPanic) {
		t.Fatalf("typed-nil callback error = %q, classified=%t", got, errors.Is(callback, ErrCallbackPanic))
	}
	if got := (&CallbackPanicError{}).Error(); got != "application command callback panicked" {
		t.Fatalf("empty callback operation = %q", got)
	}
}

type commandPanicFormattingError struct{}

func (commandPanicFormattingError) Error() string { panic("external cause must not be formatted") }
