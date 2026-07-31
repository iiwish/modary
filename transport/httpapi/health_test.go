package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iiwish/modary/appkit"
)

func TestHealthIsExplicitMinimalAndStrict(t *testing.T) {
	if handler, err := NewHealth(nil); err == nil || handler != nil {
		t.Fatalf("NewHealth(nil) = %#v, %v", handler, err)
	}
	if handler, err := NewHealth(&appkit.Application{}); err == nil || handler != nil {
		t.Fatalf("NewHealth(zero application) = %#v, %v", handler, err)
	}
	application := newHTTPTestApplication(t, true)
	handler, err := NewHealth(application.app)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health = %d %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 2 || body["status"] != "ready" {
		t.Fatalf("health body = %#v", body)
	}
	metadata, ok := body["application"].(map[string]any)
	if !ok || metadata["id"] != "example" || metadata["name"] != "Example Application" || metadata["version"] != "1.0.0" || len(metadata) != 3 {
		t.Fatalf("health metadata = %#v", body["application"])
	}
	for _, forbidden := range []string{"module", "host", "database", "startup", "handler", "registry"} {
		if strings.Contains(strings.ToLower(response.Body.String()), forbidden) {
			t.Fatalf("health leaked %q: %s", forbidden, response.Body.String())
		}
	}

	head := httptest.NewRequest(http.MethodHead, "/healthz", nil)
	headResponse := httptest.NewRecorder()
	handler.ServeHTTP(headResponse, head)
	if headResponse.Code != http.StatusOK || headResponse.Body.Len() != 0 || headResponse.Header().Get("Content-Length") == "" {
		t.Fatalf("HEAD health = %d body=%q headers=%v", headResponse.Code, headResponse.Body.String(), headResponse.Header())
	}

	for _, test := range []struct {
		name   string
		method string
		path   string
		accept string
		body   string
		status int
	}{
		{name: "method", method: http.MethodPost, path: "/healthz", status: http.StatusMethodNotAllowed},
		{name: "accept", method: http.MethodGet, path: "/healthz", accept: "text/plain", status: http.StatusNotAcceptable},
		{name: "query", method: http.MethodGet, path: "/healthz?detail=true", status: http.StatusBadRequest},
		{name: "body", method: http.MethodGet, path: "/healthz", body: `{}`, status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			if test.body == "" {
				request.Body = http.NoBody
				request.ContentLength = 0
			}
			if test.accept != "" {
				request.Header.Set("Accept", test.accept)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("health status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
		})
	}

	if err := application.app.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	unavailable := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	unavailable.Header.Set("Accept", "application/json")
	unavailableResponse := httptest.NewRecorder()
	handler.ServeHTTP(unavailableResponse, unavailable)
	if unavailableResponse.Code != http.StatusServiceUnavailable || !strings.Contains(unavailableResponse.Body.String(), `"status":"unavailable"`) {
		t.Fatalf("health after Shutdown = %d %s", unavailableResponse.Code, unavailableResponse.Body.String())
	}
}
