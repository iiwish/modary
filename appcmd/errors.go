package appcmd

import (
	"fmt"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/internal/safeerr"
)

type commandError struct {
	kind    error
	message string
	cause   error
}

// Error returns the caller-safe command diagnostic.
func (err *commandError) Error() string {
	if err == nil || err.message == "" {
		return "application command failed"
	}
	return err.message
}

// Unwrap exposes command classifications and opaque dependency causes to
// errors.Is and errors.As without dispatching caller-defined error methods.
func (err *commandError) Unwrap() []error {
	if err == nil {
		return nil
	}
	result := make([]error, 0, 2)
	if err.kind != nil {
		result = append(result, err.kind)
	}
	if err.cause != nil {
		if cause := safeerr.Opaque(err.cause); cause != nil {
			result = append(result, cause)
		}
	}
	return result
}

func usageError(format string, arguments ...any) error {
	return &commandError{kind: ErrUsage, message: fmt.Sprintf(format, arguments...)}
}

// opaqueCommandError retains an extension or dependency failure for bounded
// framework classification without formatting or otherwise inspecting it.
func opaqueCommandError(message string, cause error) error {
	if cause == nil {
		return nil
	}
	return &commandError{message: message, cause: cause}
}

func definitionProviderError(cause error) error {
	if cause == nil {
		return nil
	}
	return &commandError{
		message: "construct application Definition: " + safeerr.Diagnostic(cause),
		cause:   cause,
	}
}

func actionCommandError(message string, cause error) error {
	if cause == nil {
		return nil
	}
	actionErr, ok := safeerr.Find[*action.Error](cause)
	if !ok || actionErr == nil || !action.ValidErrorMessage(actionErr.Message) {
		return opaqueCommandError(message, cause)
	}
	kind := action.ErrorKindOf(actionErr)
	if kind == action.ErrorKindInternal {
		return opaqueCommandError(message, cause)
	}
	if _, builtin := action.BuiltinErrorKind(actionErr.Code); !builtin && !action.ValidCustomErrorCode(actionErr.Code) {
		return opaqueCommandError(message, cause)
	}
	return &commandError{message: actionErr.Error(), cause: cause}
}

// CallbackPanicError reports a contained consumer callback panic without
// exposing the recovered value.
type CallbackPanicError struct {
	Operation string
}

// Error describes the callback operation without exposing the panic value.
func (err *CallbackPanicError) Error() string {
	if err == nil || err.Operation == "" {
		return "application command callback panicked"
	}
	return fmt.Sprintf("%s callback panicked", err.Operation)
}

// Unwrap classifies the failure as ErrCallbackPanic, including for a typed-nil
// receiver.
func (err *CallbackPanicError) Unwrap() error {
	return ErrCallbackPanic
}
