package otel

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/observe"
	globalotel "go.opentelemetry.io/otel"
)

func TestModuleExportsOnlyBoundedHTTPAndOperationDimensions(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	var authorizationHeaders []string
	receiver := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(io.LimitReader(request.Body, 2<<20))
		if err != nil {
			t.Errorf("read OTLP request: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, body)
		authorizationHeaders = append(authorizationHeaders, request.Header.Get("Authorization"))
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/x-protobuf")
		writer.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	globalTracer := globalotel.GetTracerProvider()
	globalMeter := globalotel.GetMeterProvider()
	headers := map[string]string{"Authorization": "Bearer collector-secret"}
	registration, err := Module(Options{
		Endpoint: receiver.URL, Headers: headers, ServiceName: "telemetry-test",
		ServiceVersion: "0.3.0-alpha.1", Environment: "test", Insecure: true,
		ExportInterval: time.Second, ExportTimeout: time.Second, ReadyTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	headers["Authorization"] = "Bearer mutated-after-registration"
	application, err := appkit.Start(context.Background(), appkit.Definition{
		Metadata: appkit.Metadata{ID: "telemetry-test", Name: "Telemetry Test", Version: "0.3.0"},
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
		t.Fatalf("Ready() error = %v", err)
	}
	handler := observer.WrapHTTP(http.MethodGet, "/accounts/{accountID}", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		operationCtx, finish := observer.StartOperation(request.Context(), observe.OperationDatabaseQuery)
		if operationCtx == nil {
			t.Error("operation context is nil")
		}
		finish(observe.OutcomeSuccess)
		finish(observe.OutcomeError)
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/accounts/secret-account?token=secret-query", nil)
	request.Header.Set("Authorization", "Bearer request-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("HTTP status = %d", response.Code)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := application.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	if globalotel.GetTracerProvider() != globalTracer || globalotel.GetMeterProvider() != globalMeter {
		t.Fatal("component mutated an OpenTelemetry global provider")
	}

	mu.Lock()
	joined := bytes.Join(bodies, nil)
	gotHeaders := append([]string(nil), authorizationHeaders...)
	mu.Unlock()
	if len(joined) == 0 {
		t.Fatal("collector received no OTLP payload")
	}
	for _, required := range []string{"/accounts/{accountID}", "database.query", "telemetry-test"} {
		if !bytes.Contains(joined, []byte(required)) {
			t.Errorf("OTLP payload is missing bounded dimension %q", required)
		}
	}
	for _, forbidden := range []string{"secret-account", "secret-query", "request-secret", "collector-secret", "mutated-after-registration"} {
		if bytes.Contains(joined, []byte(forbidden)) {
			t.Errorf("OTLP payload leaked %q", forbidden)
		}
	}
	for _, value := range gotHeaders {
		if value != "Bearer collector-secret" {
			t.Errorf("collector Authorization header = %q", value)
		}
	}
}

func TestOptionsFailClosedBeforeRegistration(t *testing.T) {
	valid := Options{Endpoint: "https://collector.example.com:4318", ServiceName: "service", ServiceVersion: "1.0.0", Environment: "test"}
	tests := []struct {
		name    string
		mutate  func(*Options)
		message string
	}{
		{name: "HTTP without explicit insecure", mutate: func(options *Options) { options.Endpoint = "http://collector.example.com:4318" }, message: "HTTPS"},
		{name: "HTTPS with insecure", mutate: func(options *Options) { options.Insecure = true }, message: "insecure mode"},
		{name: "endpoint path", mutate: func(options *Options) { options.Endpoint += "/tenant" }, message: "endpoint"},
		{name: "endpoint query", mutate: func(options *Options) { options.Endpoint += "?token=secret" }, message: "endpoint"},
		{name: "missing service", mutate: func(options *Options) { options.ServiceName = "" }, message: "service name"},
		{name: "control environment", mutate: func(options *Options) { options.Environment = "test\nprod" }, message: "environment"},
		{name: "header newline", mutate: func(options *Options) { options.Headers = map[string]string{"Authorization": "secret\nleak"} }, message: "header"},
		{name: "case duplicate header", mutate: func(options *Options) {
			options.Headers = map[string]string{"Authorization": "one", "authorization": "two"}
		}, message: "duplicate"},
		{name: "short export interval", mutate: func(options *Options) { options.ExportInterval = time.Millisecond }, message: "at least"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := valid
			test.mutate(&options)
			_, err := Module(options)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Module() error = %v, want %q", err, test.message)
			}
		})
	}
}
