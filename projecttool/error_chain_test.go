package projecttool

import (
	"context"
	"errors"
	"testing"
)

func TestBuildWriterErrorSupportsStandardSafeInspection(t *testing.T) {
	typed := &buildWriterErrorChainCause{}
	cause := errors.Join(typed, context.Canceled)
	wrapper := &buildWriterError{operation: "build stdout", cause: cause}
	var found *buildWriterErrorChainCause
	if !errors.Is(wrapper, cause) || !errors.Is(wrapper, typed) || !errors.Is(wrapper, context.Canceled) ||
		!errors.As(wrapper, &found) || found != typed {
		t.Fatal("standard errors.Is/errors.As did not preserve the safe build-writer cause graph")
	}
}

type buildWriterErrorChainCause struct{}

func (*buildWriterErrorChainCause) Error() string { return "build writer test cause" }
