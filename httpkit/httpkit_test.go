package httpkit_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iiwish/modary/httpkit"
)

func TestNewHandlerComposesExplicitRoutes(t *testing.T) {
	handler, err := httpkit.NewHandler(
		httpkit.Route{
			Method: http.MethodGet,
			Path:   "/api/ping",
			Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusNoContent)
			}),
		},
		httpkit.Route{
			Method: http.MethodPost,
			Path:   "/api/ping",
			Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusCreated)
			}),
		},
	)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	for _, test := range []struct {
		method string
		want   int
	}{
		{method: http.MethodGet, want: http.StatusNoContent},
		{method: http.MethodPost, want: http.StatusCreated},
		{method: http.MethodDelete, want: http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, "/api/ping", nil))
		if response.Code != test.want {
			t.Fatalf("%s /api/ping = %d, want %d", test.method, response.Code, test.want)
		}
	}
}

func TestNewHandlerRejectsInvalidOrAmbiguousRoutes(t *testing.T) {
	valid := httpkit.Route{Method: http.MethodGet, Path: "/api/ping", Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}
	var typedNil http.HandlerFunc
	tests := []struct {
		name   string
		routes []httpkit.Route
		want   string
	}{
		{name: "missing method", routes: []httpkit.Route{{Path: valid.Path, Handler: valid.Handler}}, want: "method"},
		{name: "lowercase method", routes: []httpkit.Route{{Method: "get", Path: valid.Path, Handler: valid.Handler}}, want: "method"},
		{name: "invalid path", routes: []httpkit.Route{{Method: valid.Method, Path: "api/ping", Handler: valid.Handler}}, want: "path"},
		{name: "malformed standard pattern", routes: []httpkit.Route{{Method: valid.Method, Path: "/api/{", Handler: valid.Handler}}, want: "path"},
		{name: "nil handler", routes: []httpkit.Route{{Method: valid.Method, Path: valid.Path}}, want: "handler"},
		{name: "typed nil handler", routes: []httpkit.Route{{Method: valid.Method, Path: valid.Path, Handler: typedNil}}, want: "handler"},
		{name: "duplicate", routes: []httpkit.Route{valid, valid}, want: "duplicate"},
		{name: "ambiguous patterns", routes: []httpkit.Route{
			{Method: valid.Method, Path: "/api/{first}", Handler: valid.Handler},
			{Method: valid.Method, Path: "/api/{second}", Handler: valid.Handler},
		}, want: "path"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, err := httpkit.NewHandler(test.routes...)
			if err == nil || handler != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewHandler() = %#v, %v; want %q", handler, err, test.want)
			}
		})
	}
}

func TestNewHandlerOwnsItsRouteSlice(t *testing.T) {
	routes := []httpkit.Route{{
		Method:  http.MethodGet,
		Path:    "/before",
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }),
	}}
	handler, err := httpkit.NewHandler(routes...)
	if err != nil {
		t.Fatal(err)
	}
	routes[0].Path = "/after"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/before", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("GET /before = %d", response.Code)
	}
}
