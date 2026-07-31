package databasecontrol

import (
	"context"
	"errors"
	"testing"
)

func TestDatabaseErrorWrappersSupportStandardSafeInspection(t *testing.T) {
	typed := &databaseErrorChainCause{}
	cause := errors.Join(typed, context.Canceled)
	wrappers := map[string]error{
		"dependency": &dependencyError{operation: "backend", cause: cause},
	}
	for name, wrapper := range wrappers {
		t.Run(name, func(t *testing.T) {
			var found *databaseErrorChainCause
			if !errors.Is(wrapper, cause) || !errors.Is(wrapper, typed) || !errors.Is(wrapper, context.Canceled) ||
				!errors.As(wrapper, &found) || found != typed {
				t.Fatal("standard errors.Is/errors.As did not preserve the safe database cause graph")
			}
		})
	}
}

type databaseErrorChainCause struct{}

func (*databaseErrorChainCause) Error() string { return "database test cause" }
