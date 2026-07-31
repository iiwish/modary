package appkit

import (
	"context"
	"testing"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/module"
)

func TestApplicationCatalogOwnsDescriptorErrors(t *testing.T) {
	registration := runtimeRegistration(runtimeRegistrationOptions{withAction: true})
	registration.Definition.Actions[0].Descriptor.Errors = []action.ErrorSpec{{
		Code: "EXAMPLE.VERSION_CONFLICT", Kind: action.ErrorKindConflict,
	}}
	application, err := Start(context.Background(), Definition{
		Metadata: validMetadata(),
		Modules:  []module.Registration{registration},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	registration.Definition.Actions[0].Descriptor.Errors[0] = action.ErrorSpec{
		Code: "FORGED.INPUT", Kind: action.ErrorKindUnavailable,
	}
	first := application.Catalog()
	if got := first[0].Descriptor.Errors[0]; got.Code != "EXAMPLE.VERSION_CONFLICT" || got.Kind != action.ErrorKindConflict {
		t.Fatalf("catalog was mutated through Definition input: %#v", got)
	}

	first[0].Descriptor.Errors[0] = action.ErrorSpec{Code: "FORGED.OUTPUT", Kind: action.ErrorKindUnavailable}
	reloaded := application.Catalog()
	if got := reloaded[0].Descriptor.Errors[0]; got.Code != "EXAMPLE.VERSION_CONFLICT" || got.Kind != action.ErrorKindConflict {
		t.Fatalf("catalog was mutated through returned copy: %#v", got)
	}
}
