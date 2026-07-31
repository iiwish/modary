package actionruntime

import (
	"fmt"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/internal/safeerr"
)

func normalizeHandlerFailure(err error, descriptor action.Descriptor) error {
	if err == nil {
		return nil
	}
	actionErr, unique := uniqueActionError(err)
	if !unique {
		return invalidHandlerFailure(err)
	}
	kind, allowed := handlerErrorKind(descriptor, actionErr.Code)
	if !allowed || (actionErr.Kind != "" && actionErr.Kind != kind) || validatePublicErrorMessage(actionErr.Message) != nil {
		return invalidHandlerFailure(err)
	}
	return &action.Error{
		Code:    actionErr.Code,
		Kind:    kind,
		Message: actionErr.Message,
		Cause:   err,
	}
}

func uniqueActionError(err error) (*action.Error, bool) {
	seen := make(map[*action.Error]struct{}, 1)
	invalid := false
	safeerr.Walk(err, func(candidate error) bool {
		actionErr, ok := candidate.(*action.Error)
		if !ok {
			return false
		}
		if actionErr == nil {
			invalid = true
			return false
		}
		seen[actionErr] = struct{}{}
		return false
	})
	if invalid || len(seen) != 1 {
		return nil, false
	}
	for actionErr := range seen {
		return actionErr, true
	}
	return nil, false
}

func handlerErrorKind(descriptor action.Descriptor, code string) (action.ErrorKind, bool) {
	if kind, builtin := action.BuiltinErrorKind(code); builtin {
		switch code {
		case action.CodeValidationFailed,
			action.CodePreconditionFailed,
			action.CodePlanStale,
			action.CodeLimitExceeded:
			return kind, true
		default:
			return "", false
		}
	}
	kind, declared := descriptorErrorKind(descriptor, code)
	if !declared || kind == action.ErrorKindDenied || kind == action.ErrorKindInternal {
		return "", false
	}
	return kind, true
}

func descriptorErrorKind(descriptor action.Descriptor, code string) (action.ErrorKind, bool) {
	if kind, builtin := action.BuiltinErrorKind(code); builtin {
		return kind, true
	}
	for _, spec := range descriptor.Errors {
		if spec.Code == code {
			return spec.Kind, spec.Kind.Valid()
		}
	}
	return "", false
}

func validatePublicErrorMessage(message string) error {
	if !action.ValidErrorMessage(message) {
		return fmt.Errorf("public error message is invalid or exceeds %d characters", action.MaxErrorMessageRunes)
	}
	return nil
}

// finalizeRuntimeFailure is the last invariant gate before an Engine error is
// returned or written to audit. Malformed framework and extension envelopes
// fail closed without exposing their fields.
func finalizeRuntimeFailure(err error) error {
	if err == nil {
		return nil
	}
	actionErr, ok := findActionError(err)
	if !ok || actionErr == nil || !action.ValidErrorMessage(actionErr.Message) {
		return invalidRuntimeFailure(err)
	}

	kind, builtin := action.BuiltinErrorKind(actionErr.Code)
	if builtin {
		if actionErr.Kind != "" && actionErr.Kind != kind {
			return invalidRuntimeFailure(err)
		}
	} else {
		kind = actionErr.Kind
		if !action.ValidCustomErrorCode(actionErr.Code) || !kind.Valid() || kind == action.ErrorKindInternal {
			return invalidRuntimeFailure(err)
		}
	}
	if actionErr.Kind == kind {
		return err
	}
	clone := *actionErr
	clone.Kind = kind
	return &clone
}

func invalidRuntimeFailure(cause error) error {
	return &action.Error{
		Code:    action.CodeInternal,
		Kind:    action.ErrorKindInternal,
		Message: "action execution failed",
		Cause:   cause,
	}
}

func invalidHandlerFailure(cause error) error {
	return &action.Error{
		Code:    action.CodeInternal,
		Kind:    action.ErrorKindInternal,
		Message: "handler returned an invalid error",
		Cause:   cause,
	}
}
