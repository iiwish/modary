package processkit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbesDistinguishLocalLivenessReadinessAndDrain(t *testing.T) {
	var dependencyHealthy atomic.Bool
	manager, err := New(Options{Checks: []Check{{Name: "database", Run: func(context.Context) error {
		if !dependencyHealthy.Load() {
			return errors.New("database unavailable")
		}
		return nil
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	assertProbe(t, manager.LivenessHandler(), http.MethodGet, "/livez", http.StatusOK, `{"status":"live"}`)
	assertProbe(t, manager.ReadinessHandler(), http.MethodGet, "/readyz", http.StatusServiceUnavailable, `{"status":"not_ready"}`)
	if err := manager.MarkReady(); err != nil {
		t.Fatal(err)
	}
	assertProbe(t, manager.LivenessHandler(), http.MethodGet, "/livez", http.StatusOK, `{"status":"live"}`)
	assertProbe(t, manager.ReadinessHandler(), http.MethodGet, "/readyz", http.StatusServiceUnavailable, `{"status":"not_ready"}`)
	dependencyHealthy.Store(true)
	assertProbe(t, manager.ReadinessHandler(), http.MethodHead, "/readyz", http.StatusOK, "")
	manager.BeginDrain()
	assertProbe(t, manager.LivenessHandler(), http.MethodGet, "/livez", http.StatusOK, `{"status":"live"}`)
	assertProbe(t, manager.ReadinessHandler(), http.MethodGet, "/readyz", http.StatusServiceUnavailable, `{"status":"not_ready"}`)
	if err := manager.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.MarkStopped(); err != nil {
		t.Fatal(err)
	}
	assertProbe(t, manager.LivenessHandler(), http.MethodGet, "/livez", http.StatusServiceUnavailable, `{"status":"stopped"}`)
}

func TestServeUsesStructuredLifecycleAndStopsApplication(t *testing.T) {
	manager, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	var shutdowns atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- Serve(ctx, ServerOptions{
			Address: "127.0.0.1:0", Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
			Manager: manager, Logger: logger, Shutdown: func(context.Context) error { shutdowns.Add(1); return nil },
		})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for manager.Phase() != PhaseReady && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.Phase() != PhaseReady {
		t.Fatal("server did not become ready")
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop")
	}
	if shutdowns.Load() != 1 || manager.Phase() != PhaseStopped {
		t.Fatalf("shutdowns=%d phase=%s", shutdowns.Load(), manager.Phase())
	}
	for _, event := range []string{"process.ready", "http.server.started", "process.draining", "http.server.draining", "http.server.stopped", "process.stopped"} {
		if !strings.Contains(logs.String(), `"event":"`+event+`"`) {
			t.Errorf("structured logs missing %s: %s", event, logs.String())
		}
	}
}

func TestServeStopsStartedApplicationWhenListenerCannotStart(t *testing.T) {
	manager, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	var shutdowns atomic.Int32
	err = Serve(context.Background(), ServerOptions{
		Address: "not-a-listen-address", Handler: http.NotFoundHandler(), Manager: manager,
		Shutdown: func(context.Context) error { shutdowns.Add(1); return nil },
	})
	if err == nil || shutdowns.Load() != 1 || manager.Phase() != PhaseStopped {
		t.Fatalf("Serve() error=%v shutdowns=%d phase=%s", err, shutdowns.Load(), manager.Phase())
	}
}

func TestServeContainsShutdownPanicAndStopsProcess(t *testing.T) {
	manager, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	err = Serve(context.Background(), ServerOptions{
		Address: "not-a-listen-address", Handler: http.NotFoundHandler(), Manager: manager,
		Shutdown: func(context.Context) error { panic("private shutdown value") },
	})
	if err == nil || !strings.Contains(err.Error(), "shutdown callback panicked") {
		t.Fatalf("Serve() error = %v", err)
	}
	if strings.Contains(err.Error(), "private shutdown value") || manager.Phase() != PhaseStopped {
		t.Fatalf("Serve() leaked panic or failed to stop: error=%v phase=%s", err, manager.Phase())
	}
}

func TestHTTPPanicDiagnosticDoesNotExposePanicValue(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := containHandlerPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("private request value")
	}), logger)
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/private/path", nil))
	}()
	if recovered != http.ErrAbortHandler {
		t.Fatalf("contained panic = %#v", recovered)
	}
	if output := logs.String(); !strings.Contains(output, `"event":"http.handler.panicked"`) ||
		strings.Contains(output, "private request value") || strings.Contains(output, "/private/path") {
		t.Fatalf("panic diagnostic = %q", output)
	}
}

func TestProbeInputAndDependencyExecutionAreBounded(t *testing.T) {
	blocked := make(chan struct{})
	manager, err := New(Options{CheckTimeout: 20 * time.Millisecond, Checks: []Check{{Name: "blocked", Run: func(ctx context.Context) error {
		select {
		case <-blocked:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.MarkReady(); err != nil {
		t.Fatal(err)
	}
	assertProbe(t, manager.ReadinessHandler(), http.MethodGet, "/readyz", http.StatusServiceUnavailable, `{"status":"not_ready"}`)
	close(blocked)
	deadline := time.Now().Add(time.Second)
	for {
		response := httptest.NewRecorder()
		manager.ReadinessHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if response.Code == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("readiness did not recover after dependency release: %d %s", response.Code, response.Body.String())
		}
		time.Sleep(time.Millisecond)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/readyz", nil),
		httptest.NewRequest(http.MethodGet, "/readyz?verbose=true", nil),
		httptest.NewRequest(http.MethodGet, "/readyz", strings.NewReader("body")),
	} {
		response := httptest.NewRecorder()
		manager.ReadinessHandler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || len(response.Body.Bytes()) > 64 {
			t.Fatalf("invalid probe = %d %q", response.Code, response.Body.String())
		}
	}
}

func TestDrainRejectsNewWorkAndWaitsForAcceptedRequest(t *testing.T) {
	manager, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.MarkReady(); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	handler, err := manager.Middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		writer.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/work", nil))
		close(firstDone)
	}()
	<-entered
	manager.BeginDrain()
	drainDone := make(chan error, 1)
	go func() { drainDone <- manager.Drain(context.Background()) }()
	assertProbe(t, handler, http.MethodGet, "/new-work", http.StatusServiceUnavailable, `{"status":"draining"}`)
	select {
	case <-drainDone:
		t.Fatal("drain returned before accepted work completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-firstDone
	if err := <-drainDone; err != nil {
		t.Fatal(err)
	}
}

func assertProbe(t *testing.T, handler http.Handler, method, path string, status int, body string) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
	if response.Code != status || strings.TrimSpace(response.Body.String()) != body {
		t.Fatalf("%s %s = %d %q, want %d %q", method, path, response.Code, strings.TrimSpace(response.Body.String()), status, body)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("probe response is cacheable")
	}
}
