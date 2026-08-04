package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/observe"
)

func TestDisposableOTLPCollectorExport(t *testing.T) {
	endpoint := os.Getenv("MODARY_TEST_OTEL_ENDPOINT")
	if endpoint == "" {
		t.Skip("MODARY_TEST_OTEL_ENDPOINT is required for disposable Collector acceptance")
	}
	registration, err := Module(Options{
		Endpoint: endpoint, ServiceName: "modary-collector-acceptance",
		ServiceVersion: "0.3.0-alpha.1", Environment: "acceptance", Insecure: true,
		ExportInterval: time.Second, ExportTimeout: 2 * time.Second, ReadyTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := appkit.Start(context.Background(), appkit.Definition{
		Metadata: appkit.Metadata{ID: "collector-acceptance", Name: "Collector Acceptance", Version: "0.3.0"},
		Modules:  []module.Registration{registration},
	}, appkit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	observer, err := application.Observability()
	if err != nil {
		t.Fatal(err)
	}
	if err := observer.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := observer.WrapHTTP(http.MethodGet, "/collector-acceptance/{id}", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		operationContext, finish := observer.StartOperation(request.Context(), observe.OperationTaskHandle)
		if operationContext == nil {
			t.Error("operation context is nil")
		}
		finish(observe.OutcomeSuccess)
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/collector-acceptance/private-value?token=private", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("HTTP status = %d", response.Code)
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := application.Shutdown(shutdownContext); err != nil {
		t.Fatal(err)
	}
}
