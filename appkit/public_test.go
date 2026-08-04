package appkit_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/testcomponent"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/scope"
	"github.com/iiwish/modary/task"
)

var _ appkit.Runtime = action.Runtime(nil)

func TestApplicationHasNoPublicFieldsOrKernelEscape(t *testing.T) {
	typeOfApplication := reflect.TypeOf(appkit.Application{})
	for index := range typeOfApplication.NumField() {
		field := typeOfApplication.Field(index)
		if field.IsExported() {
			t.Errorf("Application field %s is exported", field.Name)
		}
		fieldType := field.Type.String()
		for _, forbidden := range []string{
			"module.Host",
			"actionruntime.Registry",
			"sql.DB",
			"action.Handler",
			"module.Resolver",
			"module.Scope",
		} {
			if strings.Contains(fieldType, forbidden) {
				t.Errorf("Application field %s exposes %s through %s", field.Name, forbidden, fieldType)
			}
		}
	}
}

func TestExternalConsumerCanStartExecuteAndShutdown(t *testing.T) {
	executionScope := scope.Must("tenant", "external-appkit")
	descriptor := action.Descriptor{
		ID: "probe.read", Version: "1.0.0", Title: "Read probe", Permission: "probe.read",
		Preview: action.PreviewNone, AuditLevel: action.AuditMetadata, Channels: []action.Channel{"test"},
		InputSchema:  action.Object(nil).JSON(),
		OutputSchema: action.Object(map[string]action.Field{"value": action.RequiredField(action.Integer())}).JSON(),
	}
	registration := module.Register(module.Manifest{
		SchemaVersion: module.SchemaVersion, ID: "external-probe", Version: "1.0.0", Type: module.ModuleTypeFeature,
		Provides: []module.Capability{
			module.CapabilityAuthorization,
			module.CapabilityAudit,
			"probe",
		},
	}, func(_ context.Context, installation module.Scope) error {
		if err := module.Provide(installation, module.Authorizer(), authz.Authorizer(externalAllowAll{})); err != nil {
			return err
		}
		return module.Provide(installation, module.AuditHook(), audit.Hook(externalAudit{}))
	}, module.ActionBinding{
		Descriptor: descriptor,
		NewHandler: func(context.Context, module.Resolver) (action.Handler, error) {
			return externalProbeHandler{}, nil
		},
	})
	application, err := appkit.Start(context.Background(), appkit.Definition{
		Metadata: appkit.Metadata{ID: "external-app", Name: "External App", Version: "0.1.0"},
		Modules:  []module.Registration{testcomponent.RuntimeModule(true), registration},
	}, appkit.Options{})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	result, err := application.Runtime().Execute(context.Background(), action.Request{
		Actor:    externalActor(executionScope),
		Channel:  "test",
		ActionID: descriptor.ID,
		Scope:    executionScope,
		Input:    json.RawMessage(`{}`),
	})
	if err != nil || string(result.Data) != `{"value":1}` {
		t.Fatalf("Execute() = %s, %v", result.Data, err)
	}
	tasks := application.Tasks()
	if tasks == nil {
		t.Fatal("Tasks() is nil")
	}
	runner, err := tasks.NewRunner(task.HandlerFunc(func(context.Context, task.Job) error { return nil }), task.RunnerOptions{})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	select {
	case <-runner.Stopped():
	default:
		t.Fatal("application shutdown did not stop the registered task runner")
	}
	if _, err := tasks.NewRunner(task.HandlerFunc(func(context.Context, task.Job) error { return nil }), task.RunnerOptions{}); !errors.Is(err, task.ErrUnavailable) {
		t.Fatalf("NewRunner() after shutdown error = %v", err)
	}
	if _, err := tasks.Enqueue(context.Background(), task.Request{Kind: "probe.run"}); !errors.Is(err, task.ErrUnavailable) {
		t.Fatalf("Enqueue() after shutdown error = %v", err)
	}
}

type externalAllowAll struct{}

type externalAudit struct{}

func (externalAudit) Record(context.Context, audit.Event) error { return nil }

func (externalAllowAll) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Allowed: true, Fingerprint: "external-policy-v1"}, nil
}

type externalProbeHandler struct{}

func externalActor(executionScope scope.Execution) identity.Actor {
	return identity.Actor{ID: "external-user", Type: "user", DisplayName: "External User"}
}

func (externalProbeHandler) Plan(context.Context, action.Request) (action.PlanData, error) {
	return action.PlanData{Payload: json.RawMessage(`{}`), Impact: authz.Impact{}}, nil
}

func (externalProbeHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	return action.Result{Data: json.RawMessage(`{"value":1}`)}, nil
}

func TestValidateMetadataIsPublicAndPure(t *testing.T) {
	valid := appkit.Metadata{ID: "example-app", Name: "Example App", Version: "1.2.3"}
	if err := appkit.ValidateMetadata(valid); err != nil {
		t.Fatalf("ValidateMetadata(valid) error = %v", err)
	}
	invalid := valid
	invalid.ID = "Invalid"
	if err := appkit.ValidateMetadata(invalid); err == nil {
		t.Fatal("ValidateMetadata accepted an invalid id")
	}
}
