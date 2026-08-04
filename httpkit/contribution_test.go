package httpkit_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/httpkit"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/observe"
)

func TestPlanRejectsMissingContributionCapabilityBeforeModuleStart(t *testing.T) {
	var starts atomic.Int64
	definition := contributionDefinition("missing-capability", func(context.Context, module.Scope) error {
		starts.Add(1)
		return nil
	})
	_, err := httpkit.NewPlan(definition, appkit.Options{}, httpkit.Contribution{
		ID:       "records",
		Requires: []module.Capability{module.CapabilityDatabase, module.CapabilityAuthorization},
		Routes:   []httpkit.RouteSpec{{Method: http.MethodGet, Path: "/api/records"}},
		Build: func(context.Context, *appkit.Application) ([]httpkit.Route, error) {
			t.Fatal("contribution builder ran during preflight")
			return nil, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "database") || !strings.Contains(err.Error(), "authorization") {
		t.Fatalf("NewPlan() error = %v", err)
	}
	if starts.Load() != 0 {
		t.Fatalf("module starts before contribution failure = %d", starts.Load())
	}
}

func TestPlanRejectsBroadIdentityWithoutExplicitSessionsBeforeStart(t *testing.T) {
	var starts atomic.Int64
	definition := appkit.Definition{
		Metadata: appkit.Metadata{ID: "missing-sessions", Name: "Missing Sessions", Version: "1.0.0"},
		Modules: []module.Registration{module.Register(module.Manifest{
			SchemaVersion: module.SchemaVersion, ID: "identity-only", Version: "1.0.0", Type: module.ModuleTypeAdapter,
			Provides: []module.Capability{module.CapabilityIdentity},
		}, func(context.Context, module.Scope) error { starts.Add(1); return nil })},
	}
	_, err := httpkit.NewPlan(definition, appkit.Options{}, httpkit.Contribution{
		ID: "session-route", Requires: []module.Capability{module.CapabilitySessions},
	})
	if err == nil || !strings.Contains(err.Error(), string(module.CapabilitySessions)) {
		t.Fatalf("NewPlan() session error = %v", err)
	}
	if starts.Load() != 0 {
		t.Fatalf("module starts before session dependency failure = %d", starts.Load())
	}
}

func TestPlanRejectsDuplicateRoutesAndInvalidAdminDescriptorsPurely(t *testing.T) {
	definition := contributionDefinition("route-conflict", nil)
	valid := func(id, path string) httpkit.Contribution {
		return httpkit.Contribution{
			ID: id, Routes: []httpkit.RouteSpec{{Method: http.MethodGet, Path: path}},
			Build: func(context.Context, *appkit.Application) ([]httpkit.Route, error) { return nil, nil },
		}
	}
	_, err := httpkit.NewPlan(definition, appkit.Options{}, valid("first", "/api/items"), valid("second", "/api/items"))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate route error = %v", err)
	}

	invalidAdmin := valid("admin", "/api/admin")
	invalidAdmin.Admin = &httpkit.AdminDescriptor{Label: "Admin", Path: "settings", Icon: "settings"}
	if _, err := httpkit.NewPlan(definition, appkit.Options{}, invalidAdmin); err == nil || !strings.Contains(err.Error(), "Admin") {
		t.Fatalf("invalid Admin descriptor error = %v", err)
	}
	undeclaredPermission := valid("undeclared-permission", "/api/undeclared")
	undeclaredPermission.Admin = &httpkit.AdminDescriptor{
		Label: "Undeclared", Path: "/undeclared", Icon: "settings",
		Permissions: []string{"records.list"}, RequiredPermissions: []string{"records.update"},
	}
	if _, err := httpkit.NewPlan(definition, appkit.Options{}, undeclaredPermission); err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("undeclared required permission error = %v", err)
	}
}

func TestPlanBuildsDeclaredRoutesAfterStartupAndOwnsDescriptors(t *testing.T) {
	definition := contributionDefinition("valid-plan", nil)
	var builds atomic.Int64
	permissions := []string{"records.list"}
	allPermissions := []string{"records.create", "records.list"}
	contribution := httpkit.Contribution{
		ID:     "records",
		Routes: []httpkit.RouteSpec{{Method: http.MethodGet, Path: "/api/records"}},
		Admin: &httpkit.AdminDescriptor{
			Label: "Records", Path: "/records", Icon: "database", Permissions: allPermissions, RequiredPermissions: permissions,
		},
		Build: func(context.Context, *appkit.Application) ([]httpkit.Route, error) {
			builds.Add(1)
			return []httpkit.Route{{
				Method: http.MethodGet, Path: "/api/records",
				Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }),
			}}, nil
		},
	}
	plan, err := httpkit.NewPlan(definition, appkit.Options{}, contribution)
	if err != nil {
		t.Fatal(err)
	}
	permissions[0] = "mutated"
	allPermissions[0] = "mutated"
	contribution.Routes[0].Path = "/mutated"
	if builds.Load() != 0 {
		t.Fatal("builder ran during plan construction")
	}
	descriptors := plan.Admin()
	if len(descriptors) != 1 || descriptors[0].ID != "records" || descriptors[0].RequiredPermissions[0] != "records.list" {
		t.Fatalf("Admin() = %#v", descriptors)
	}
	descriptors[0].RequiredPermissions[0] = "changed"
	if plan.Admin()[0].RequiredPermissions[0] != "records.list" {
		t.Fatal("Plan.Admin() exposed mutable permission state")
	}
	if plan.Admin()[0].Permissions[0] != "records.create" {
		t.Fatal("Plan.Admin() exposed mutable permission inventory")
	}

	application, err := appkit.Start(context.Background(), definition, appkit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(context.Background())
	handler, err := plan.Handler(context.Background(), application)
	if err != nil {
		t.Fatal(err)
	}
	if builds.Load() != 1 {
		t.Fatalf("builder calls = %d", builds.Load())
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/records", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("GET /api/records = %d", response.Code)
	}
}

func TestPlanInstrumentsOnlyPreflightedMethodAndRouteWhenSelected(t *testing.T) {
	observer := &recordingObserver{}
	definition := contributionDefinition("observed-plan", nil)
	definition.Modules = append(definition.Modules, module.Register(module.Manifest{
		SchemaVersion: module.SchemaVersion, ID: "observer", Version: "1.0.0", Type: module.ModuleTypeAdapter,
		Provides: []module.Capability{module.CapabilityObservability},
	}, func(_ context.Context, scope module.Scope) error {
		return module.Provide(scope, module.Observability(), observe.Service(observer))
	}))
	plan, err := httpkit.NewPlan(definition, appkit.Options{}, httpkit.Contribution{
		ID: "accounts", Routes: []httpkit.RouteSpec{{Method: http.MethodGet, Path: "/accounts/{accountID}"}},
		Build: func(context.Context, *appkit.Application) ([]httpkit.Route, error) {
			return []httpkit.Route{{Method: http.MethodGet, Path: "/accounts/{accountID}", Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusNoContent)
			})}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := appkit.Start(context.Background(), definition, appkit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(context.Background())
	handler, err := plan.Handler(context.Background(), application)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/accounts/secret-value?token=secret", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("response = %d", response.Code)
	}
	if observer.method != http.MethodGet || observer.route != "/accounts/{accountID}" {
		t.Fatalf("observer dimensions = %q %q", observer.method, observer.route)
	}
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/accounts/another-secret", nil))
	if second.Code != http.StatusNoContent || observer.wraps != 1 {
		t.Fatalf("second response = %d, observer wrappers = %d", second.Code, observer.wraps)
	}
}

func TestPlanRejectsBuilderRouteDrift(t *testing.T) {
	definition := contributionDefinition("route-drift", nil)
	plan, err := httpkit.NewPlan(definition, appkit.Options{}, httpkit.Contribution{
		ID: "drift", Routes: []httpkit.RouteSpec{{Method: http.MethodGet, Path: "/declared"}},
		Build: func(context.Context, *appkit.Application) ([]httpkit.Route, error) {
			return []httpkit.Route{{Method: http.MethodGet, Path: "/built", Handler: http.NotFoundHandler()}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := appkit.Start(context.Background(), definition, appkit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(context.Background())
	if handler, err := plan.Handler(context.Background(), application); err == nil || handler != nil || !strings.Contains(err.Error(), "declared") {
		t.Fatalf("Handler() = %#v, %v", handler, err)
	}
}

func TestPlanBoundsNestedContributionInputs(t *testing.T) {
	definition := contributionDefinition("bounded-plan", nil)
	routes := make([]httpkit.RouteSpec, 1025)
	for index := range routes {
		routes[index] = httpkit.RouteSpec{Method: http.MethodGet, Path: fmt.Sprintf("/route/%d", index)}
	}
	_, err := httpkit.NewPlan(definition, appkit.Options{}, httpkit.Contribution{
		ID: "too-many-routes", Routes: routes,
		Build: func(context.Context, *appkit.Application) ([]httpkit.Route, error) { return nil, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "route count") {
		t.Fatalf("unbounded route list error = %v", err)
	}

	requires := make([]module.Capability, 65)
	for index := range requires {
		requires[index] = module.Capability(fmt.Sprintf("feature.%d", index))
	}
	_, err = httpkit.NewPlan(definition, appkit.Options{}, httpkit.Contribution{ID: "too-many-requirements", Requires: requires})
	if err == nil || !strings.Contains(err.Error(), "requirement count") {
		t.Fatalf("unbounded requirement list error = %v", err)
	}

	longPath := "/" + strings.Repeat("a", 2048)
	_, err = httpkit.NewPlan(definition, appkit.Options{}, httpkit.Contribution{
		ID: "long-route", Routes: []httpkit.RouteSpec{{Method: http.MethodGet, Path: longPath}},
		Build: func(context.Context, *appkit.Application) ([]httpkit.Route, error) { return nil, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("unbounded route path error = %v", err)
	}

	_, err = httpkit.NewPlan(definition, appkit.Options{}, httpkit.Contribution{
		ID: "long-admin-path", Admin: &httpkit.AdminDescriptor{
			Label: "Long path", Path: longPath, Icon: "settings",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Admin path") {
		t.Fatalf("unbounded Admin path error = %v", err)
	}
}

func contributionDefinition(id string, start module.StartFunc) appkit.Definition {
	return appkit.Definition{
		Metadata: appkit.Metadata{ID: id, Name: "Contribution Test", Version: "1.0.0"},
		Modules: []module.Registration{module.Register(module.Manifest{
			SchemaVersion: module.SchemaVersion, ID: "base", Version: "1.0.0", Type: module.ModuleTypeFeature,
			Provides: []module.Capability{"base"},
		}, start)},
	}
}

type recordingObserver struct {
	method string
	route  string
	wraps  int
}

func (observer *recordingObserver) WrapHTTP(method, route string, next http.Handler) http.Handler {
	observer.method = method
	observer.route = route
	observer.wraps++
	return next
}

func (*recordingObserver) StartOperation(ctx context.Context, _ observe.Operation) (context.Context, func(observe.Outcome)) {
	return ctx, func(observe.Outcome) {}
}

func (*recordingObserver) Ready(context.Context) error { return nil }
