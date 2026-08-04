package appkit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/processkit"
)

func TestExternalConsumerCanStartDatabaseFreeHTTPApplication(t *testing.T) {
	var starts atomic.Int32
	var stops atomic.Int32
	feature := module.Register(module.Manifest{
		SchemaVersion: module.SchemaVersion,
		ID:            "ping",
		Version:       "1.0.0",
		Type:          module.ModuleTypeFeature,
		Provides:      []module.Capability{"ping"},
	}, func(_ context.Context, scope module.Scope) error {
		starts.Add(1)
		return module.OnStop(scope, func(context.Context) error {
			stops.Add(1)
			return nil
		})
	})

	application, err := appkit.Start(context.Background(), appkit.Definition{
		Metadata: appkit.Metadata{ID: "minimal-api", Name: "Minimal API", Version: "0.1.0"},
		Modules:  []module.Registration{feature},
	}, appkit.Options{})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if starts.Load() != 1 || !application.Ready() {
		t.Fatalf("starts=%d ready=%v", starts.Load(), application.Ready())
	}
	if application.Runtime() != nil || application.Tasks() != nil || len(application.Catalog()) != 0 {
		t.Fatalf("unexpected optional surfaces: runtime=%v tasks=%v catalog=%#v", application.Runtime(), application.Tasks(), application.Catalog())
	}
	if _, err := application.Identities(); !errors.Is(err, appkit.ErrIdentitiesUnavailable) {
		t.Fatalf("Identities() error = %v", err)
	}

	process, err := processkit.New(processkit.Options{})
	if err != nil {
		t.Fatalf("processkit.New() error = %v", err)
	}
	if err := process.MarkReady(); err != nil {
		t.Fatalf("MarkReady() error = %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /livez", process.LivenessHandler())
	mux.Handle("GET /readyz", process.ReadinessHandler())
	mux.HandleFunc("GET /api/ping", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"message":"pong"}`))
	})

	healthRequest := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	healthResponse := httptest.NewRecorder()
	mux.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK || !strings.Contains(healthResponse.Body.String(), `"status":"ready"`) {
		t.Fatalf("GET /readyz = %d %s", healthResponse.Code, healthResponse.Body.String())
	}

	pingResponse := httptest.NewRecorder()
	mux.ServeHTTP(pingResponse, httptest.NewRequest(http.MethodGet, "/api/ping", nil))
	if pingResponse.Code != http.StatusOK || pingResponse.Body.String() != `{"message":"pong"}` {
		t.Fatalf("GET /api/ping = %d %s", pingResponse.Code, pingResponse.Body.String())
	}

	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if application.Ready() || stops.Load() != 1 {
		t.Fatalf("ready=%v stops=%d after shutdown", application.Ready(), stops.Load())
	}
	process.BeginDrain()
	if err := process.MarkStopped(); err != nil {
		t.Fatalf("MarkStopped() error = %v", err)
	}
	healthResponse = httptest.NewRecorder()
	mux.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz after shutdown = %d, want %d", healthResponse.Code, http.StatusServiceUnavailable)
	}
}
