package actionruntime

import (
	"context"
	"errors"
	. "github.com/iiwish/modary/action"
	"testing"
)

func TestActionErrorWrappersSupportStandardSafeInspection(t *testing.T) {
	typed := &actionErrorChainCause{}
	cause := errors.Join(typed, context.Canceled)
	wrappers := map[string]error{
		"public Action error": &Error{Code: CodeInternal, Message: "failed", Cause: cause},
		"dependency error":    &dependencyError{operation: "callback", cause: cause},
		"required audit error": &requiredAuditError{
			cause: cause,
		},
	}
	for name, wrapper := range wrappers {
		t.Run(name, func(t *testing.T) {
			assertActionErrorChain(t, wrapper, cause, typed)
		})
	}
}

func assertActionErrorChain(t *testing.T, wrapper, exact error, typed *actionErrorChainCause) {
	t.Helper()
	var found *actionErrorChainCause
	if !errors.Is(wrapper, exact) || !errors.Is(wrapper, typed) || !errors.Is(wrapper, context.Canceled) ||
		!errors.As(wrapper, &found) || found != typed {
		t.Fatal("standard errors.Is/errors.As did not preserve the safe Action cause graph")
	}
}

type actionErrorChainCause struct{}

func (*actionErrorChainCause) Error() string { return "action test cause" }
