// Package projecttool provides pure inspection, deterministic generation, and
// Node-free builds for a consumer-owned Modary application Definition. Its F0
// public API and versioned generated documents are alpha contracts.
package projecttool

import (
	"errors"
	"fmt"
	"strings"

	"github.com/iiwish/modary/internal/safeerr"
)

var (
	// ErrContextRequired reports a nil project-tool context.
	ErrContextRequired = errors.New("project tool context is required")
	// ErrUsage reports invalid command syntax or command options.
	ErrUsage = errors.New("invalid project tool usage")
	// ErrDrift reports missing or differing generated artifacts.
	ErrDrift = errors.New("generated artifacts have drift")
	// ErrCallbackPanic reports a contained consumer callback panic.
	ErrCallbackPanic = errors.New("project tool callback panic")
	// ErrBuildUnsupported reports that the current platform cannot enforce the
	// secure compiler-staging contract required by Project.Build.
	ErrBuildUnsupported = errors.New("secure project build is unsupported")
)

type usageError struct{ message string }

// Error returns the validated usage diagnostic.
func (err *usageError) Error() string {
	if err == nil || err.message == "" {
		return "invalid project tool usage"
	}
	return err.message
}

// Unwrap classifies the diagnostic as ErrUsage, including for a typed-nil
// receiver.
func (err *usageError) Unwrap() error { return ErrUsage }

func newUsageError(format string, arguments ...any) error {
	return &usageError{message: fmt.Sprintf(format, arguments...)}
}

// CallbackPanicError reports a recovered consumer callback panic without
// exposing the recovered value, which may contain application secrets.
type CallbackPanicError struct{ Operation string }

// Error describes the callback operation without exposing the panic value.
func (err *CallbackPanicError) Error() string {
	if err == nil || err.Operation == "" {
		return "project tool callback panicked"
	}
	return fmt.Sprintf("%s callback panicked", err.Operation)
}

// Unwrap classifies the failure as ErrCallbackPanic, including for a typed-nil
// receiver.
func (err *CallbackPanicError) Unwrap() error { return ErrCallbackPanic }

type buildWriterError struct {
	operation string
	cause     error
}

type definitionProviderError struct {
	diagnostic string
	cause      error
}

func newDefinitionProviderError(cause error) error {
	if cause == nil {
		return nil
	}
	return &definitionProviderError{
		diagnostic: "construct application Definition: " + safeerr.Diagnostic(cause),
		cause:      cause,
	}
}

func (err *definitionProviderError) Error() string {
	if err == nil || err.diagnostic == "" {
		return "construct application Definition failed"
	}
	return err.diagnostic
}

func (err *definitionProviderError) Unwrap() error {
	if err == nil || err.cause == nil {
		return nil
	}
	return safeerr.Opaque(err.cause)
}

// Error returns stable build-writer context without formatting the cause.
func (err *buildWriterError) Error() string {
	if err == nil || err.operation == "" {
		return "build output writer failed"
	}
	return err.operation + " failed"
}

// Unwrap exposes the writer cause through a safe opaque boundary.
func (err *buildWriterError) Unwrap() error {
	if err == nil || err.cause == nil {
		return nil
	}
	return safeerr.Opaque(err.cause)
}

// DriftStatus classifies a generated artifact mismatch.
type DriftStatus string

const (
	// DriftMissing means the configured artifact does not exist.
	DriftMissing DriftStatus = "missing"
	// DriftDifferent means the artifact bytes differ from canonical output.
	DriftDifferent DriftStatus = "different"
)

// Drift identifies one generated artifact that is absent or differs from its
// deterministic expected bytes.
type Drift struct {
	Path   string      `json:"path"`
	Status DriftStatus `json:"status"`
}

// DriftError is returned by command check mode and Build. Programmatic callers
// can use Project.Check when drift is expected data rather than an error.
type DriftError struct{ Items []Drift }

// Error renders the complete deterministic drift list.
func (err *DriftError) Error() string {
	if err == nil || len(err.Items) == 0 {
		return "generated artifacts have drift"
	}
	parts := make([]string, 0, len(err.Items))
	for _, item := range err.Items {
		parts = append(parts, item.Path+" ("+string(item.Status)+")")
	}
	return "generated artifact drift: " + strings.Join(parts, ", ")
}

// Unwrap classifies the mismatch as ErrDrift, including for a typed-nil
// receiver.
func (err *DriftError) Unwrap() error { return ErrDrift }
