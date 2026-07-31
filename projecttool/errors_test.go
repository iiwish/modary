package projecttool

import (
	"errors"
	"testing"
)

func TestProjectToolErrorsAreTypedNilSafeAndStableWhenEmpty(t *testing.T) {
	var usage *usageError
	if got := usage.Error(); got != "invalid project tool usage" || !errors.Is(usage, ErrUsage) {
		t.Fatalf("typed-nil usage error = %q, classified=%t", got, errors.Is(usage, ErrUsage))
	}
	if got := (&usageError{}).Error(); got != "invalid project tool usage" {
		t.Fatalf("empty usage error = %q", got)
	}

	var callback *CallbackPanicError
	if got := callback.Error(); got != "project tool callback panicked" || !errors.Is(callback, ErrCallbackPanic) {
		t.Fatalf("typed-nil callback error = %q, classified=%t", got, errors.Is(callback, ErrCallbackPanic))
	}
	if got := (&CallbackPanicError{}).Error(); got != "project tool callback panicked" {
		t.Fatalf("empty callback operation = %q", got)
	}

	var drift *DriftError
	if got := drift.Error(); got != "generated artifacts have drift" || !errors.Is(drift, ErrDrift) {
		t.Fatalf("typed-nil drift error = %q, classified=%t", got, errors.Is(drift, ErrDrift))
	}
	if got := (&DriftError{}).Error(); got != "generated artifacts have drift" {
		t.Fatalf("empty drift error = %q", got)
	}

	var cause *nilWriteError
	writer := &buildWriterError{operation: "build stdout", cause: cause}
	assertTypedNilWriterFailure(t, writer, cause)
}
