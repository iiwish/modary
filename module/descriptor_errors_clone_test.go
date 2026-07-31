package module

import (
	"context"
	"testing"

	"github.com/iiwish/modary/action"
)

func TestHostOwnsDescriptorErrorsAcrossRegistrationAndCatalog(t *testing.T) {
	binding := ActionBinding{
		Descriptor: testActionDescriptor("counter.increment"),
		NewHandler: func(context.Context, Resolver) (action.Handler, error) {
			return inertActionHandler{}, nil
		},
	}
	binding.Descriptor.Errors = []action.ErrorSpec{{
		Code: "COUNTER.VERSION_CONFLICT", Kind: action.ErrorKindConflict,
	}}
	host := NewHost()
	if err := host.Register(Register(
		validManifest("counter", "feature", nil, []string{"counter"}), nil, binding,
	)); err != nil {
		t.Fatal(err)
	}

	binding.Descriptor.Errors[0] = action.ErrorSpec{Code: "FORGED.INPUT", Kind: action.ErrorKindUnavailable}
	first, err := host.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if got := first[0].Descriptor.Errors[0]; got.Code != "COUNTER.VERSION_CONFLICT" || got.Kind != action.ErrorKindConflict {
		t.Fatalf("Host catalog was mutated through registration input: %#v", got)
	}

	first[0].Descriptor.Errors[0] = action.ErrorSpec{Code: "FORGED.OUTPUT", Kind: action.ErrorKindUnavailable}
	reloaded, err := host.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded[0].Descriptor.Errors[0]; got.Code != "COUNTER.VERSION_CONFLICT" || got.Kind != action.ErrorKindConflict {
		t.Fatalf("Host catalog was mutated through returned copy: %#v", got)
	}
}
