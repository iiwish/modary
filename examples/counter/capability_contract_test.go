package consumer_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"example.com/modary-counter-consumer/modules/clockcontract"
	"example.com/modary-counter-consumer/modules/systemclock"
	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/module"
)

func TestCustomCapabilityRejectsUndeclaredAndRecreatedKeys(t *testing.T) {
	t.Run("undeclared access", func(t *testing.T) {
		rejected := errors.New("undeclared access was rejected")
		consumer := module.Register(
			customCapabilityManifest("undeclared-clock-user", nil),
			func(_ context.Context, scope module.Scope) error {
				if _, err := module.Resolve(scope, clockcontract.Key); err == nil ||
					!strings.Contains(err.Error(), "not declared") {
					return fmt.Errorf("undeclared Resolve error = %v", err)
				}
				return rejected
			},
		)
		host := module.NewHost()
		if err := host.Register(systemclock.Module(), consumer); err != nil {
			t.Fatal(err)
		}
		if err := host.Start(context.Background()); !errors.Is(err, rejected) {
			t.Fatalf("Start() error = %v, want undeclared-access rejection", err)
		}
	})

	t.Run("recreated same-name key", func(t *testing.T) {
		rejected := errors.New("recreated key was rejected")
		forged := module.MustKey[clockcontract.Clock](
			clockcontract.Key.Name(),
			clockcontract.Key.Capability(),
		)
		consumer := module.Register(
			customCapabilityManifest("forged-clock-user", []module.Capability{clockcontract.Capability}),
			func(_ context.Context, scope module.Scope) error {
				if _, err := module.Resolve(scope, forged); err == nil ||
					!strings.Contains(err.Error(), "does not match the registered key") {
					return fmt.Errorf("recreated-key Resolve error = %v", err)
				}
				return rejected
			},
		)
		host := module.NewHost()
		if err := host.Register(systemclock.Module(), consumer); err != nil {
			t.Fatal(err)
		}
		if err := host.Start(context.Background()); !errors.Is(err, rejected) {
			t.Fatalf("Start() error = %v, want recreated-key rejection", err)
		}
	})
}

func TestCustomCapabilityResolverLifetimeAndConcurrency(t *testing.T) {
	var retained module.Resolver
	var clock clockcontract.Clock
	consumer := module.Register(
		customCapabilityManifest("clock-user", []module.Capability{clockcontract.Capability}),
		nil,
		module.ActionBinding{
			Descriptor: customCapabilityDescriptor(),
			NewHandler: func(_ context.Context, resolver module.Resolver) (action.Handler, error) {
				retained = resolver
				resolved, err := module.Resolve(resolver, clockcontract.Key)
				if err != nil {
					return nil, err
				}
				clock = resolved
				return customCapabilityHandler{}, nil
			},
		},
	)
	host := module.NewHost()
	if err := host.Register(systemclock.Module(), consumer); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Resolve(retained, clockcontract.Key); !errors.Is(err, module.ErrInvalidResolver) ||
		!errors.Is(err, module.ErrInvalidScope) {
		t.Fatalf("retained Resolver error = %v, want ErrInvalidResolver with F0 compatibility", err)
	}

	var calls sync.WaitGroup
	calls.Add(32)
	for range 32 {
		go func() {
			defer calls.Done()
			for range 100 {
				if clock.Now().IsZero() {
					t.Error("custom Clock returned zero time")
					return
				}
			}
		}()
	}
	calls.Wait()
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func customCapabilityManifest(id string, requires []module.Capability) module.Manifest {
	return module.Manifest{
		SchemaVersion: module.SchemaVersion,
		ID:            id,
		Version:       "0.1.0",
		Type:          module.ModuleTypeFeature,
		Requires:      requires,
	}
}

func customCapabilityDescriptor() action.Descriptor {
	return action.Descriptor{
		ID:           "clock.read",
		Version:      "1.0.0",
		Title:        "Read clock",
		InputSchema:  action.Object(nil).JSON(),
		OutputSchema: action.Object(nil).JSON(),
		Permission:   "clock.read",
		Preview:      action.PreviewNone,
		AuditLevel:   action.AuditMetadata,
		Channels:     []action.Channel{"test"},
	}
}

type customCapabilityHandler struct{}

func (customCapabilityHandler) Plan(context.Context, action.Request) (action.PlanData, error) {
	return action.PlanData{}, nil
}

func (customCapabilityHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	return action.Result{Data: json.RawMessage(`{}`)}, nil
}
