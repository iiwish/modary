package module

import (
	"context"
	"errors"
	"testing"
)

func TestModuleDependencyErrorSupportsStandardSafeInspection(t *testing.T) {
	typed := &moduleErrorChainCause{}
	cause := errors.Join(typed, context.Canceled)
	wrapper := &dependencyError{operation: "callback", cause: cause}
	var found *moduleErrorChainCause
	if !errors.Is(wrapper, cause) || !errors.Is(wrapper, typed) || !errors.Is(wrapper, context.Canceled) ||
		!errors.As(wrapper, &found) || found != typed {
		t.Fatal("standard errors.Is/errors.As did not preserve the safe Module cause graph")
	}
}

type moduleErrorChainCause struct{}

func (*moduleErrorChainCause) Error() string { return "module test cause" }
