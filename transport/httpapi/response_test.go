package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestContainResponsePanicUsesFallbackOnlyBeforeCommit(t *testing.T) {
	cause := &hostileResponsePanic{}
	recorder := httptest.NewRecorder()
	tracked := &trackedResponseWriter{ResponseWriter: recorder}

	invokeWithResponsePanicGuard(tracked, func() {
		tracked.Header().Set("Content-Type", "text/plain")
		tracked.WriteHeader(http.StatusInternalServerError)
		_, _ = tracked.Write([]byte("internal server error\n"))
	}, func() {
		panic(cause)
	})

	if recorder.Code != http.StatusInternalServerError || recorder.Body.String() != "internal server error\n" {
		t.Fatalf("fallback response = %d %q", recorder.Code, recorder.Body.String())
	}
	if calls := cause.calls.Load(); calls != 0 {
		t.Fatalf("hostile panic methods called %d times", calls)
	}
}

func TestContainResponsePanicAbortsCommittedAndFailedFallbackResponses(t *testing.T) {
	for _, test := range []struct {
		name     string
		prepare  func(*trackedResponseWriter)
		fallback func()
	}{
		{
			name: "committed",
			prepare: func(writer *trackedResponseWriter) {
				writer.WriteHeader(http.StatusOK)
			},
			fallback: func() { t.Fatal("fallback ran after response commitment") },
		},
		{
			name:     "fallback panic",
			prepare:  func(*trackedResponseWriter) {},
			fallback: func() { panic("fallback writer failed") },
		},
		{
			name:     "fallback did not commit",
			prepare:  func(*trackedResponseWriter) {},
			fallback: func() {},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cause := &hostileResponsePanic{}
			tracked := &trackedResponseWriter{ResponseWriter: httptest.NewRecorder()}
			test.prepare(tracked)
			recovered := captureResponsePanic(func() {
				invokeWithResponsePanicGuard(tracked, test.fallback, func() { panic(cause) })
			})
			if recovered != http.ErrAbortHandler {
				t.Fatalf("panic = %#v, want http.ErrAbortHandler", recovered)
			}
			if calls := cause.calls.Load(); calls != 0 {
				t.Fatalf("hostile panic methods called %d times", calls)
			}
		})
	}
}

func TestResponsePanicGuardContainsNilPanics(t *testing.T) {
	t.Run("uncommitted stable fallback", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		tracked := &trackedResponseWriter{ResponseWriter: recorder}
		invokeWithResponsePanicGuard(tracked, func() {
			tracked.Header().Set("Content-Type", "text/plain")
			tracked.WriteHeader(http.StatusInternalServerError)
			_, _ = tracked.Write([]byte("internal server error\n"))
		}, func() {
			panic(nil)
		})
		if recorder.Code != http.StatusInternalServerError || recorder.Body.String() != "internal server error\n" {
			t.Fatalf("fallback response = %d %q", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("committed abort", func(t *testing.T) {
		tracked := &trackedResponseWriter{ResponseWriter: httptest.NewRecorder()}
		tracked.WriteHeader(http.StatusNoContent)
		recovered := captureResponsePanic(func() {
			invokeWithResponsePanicGuard(tracked, func() {
				t.Fatal("fallback ran after response commitment")
			}, func() {
				panic(nil)
			})
		})
		if recovered != http.ErrAbortHandler {
			t.Fatalf("panic = %#v, want http.ErrAbortHandler", recovered)
		}
	})

	t.Run("fallback abort", func(t *testing.T) {
		tracked := &trackedResponseWriter{ResponseWriter: httptest.NewRecorder()}
		recovered := captureResponsePanic(func() {
			invokeWithResponsePanicGuard(tracked, func() {
				panic(nil)
			}, func() {
				panic(nil)
			})
		})
		if recovered != http.ErrAbortHandler {
			t.Fatalf("panic = %#v, want http.ErrAbortHandler", recovered)
		}
	})
}

func TestTransportHandlersAbortWhenResponseCommitPanics(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
		request *http.Request
	}{
		{
			name:    "API",
			handler: &apiServer{timeout: time.Second},
			request: httptest.NewRequest(http.MethodGet, "/missing", nil),
		},
		{
			name:    "MCP",
			handler: &mcpHandler{},
			request: httptest.NewRequest(http.MethodGet, "/mcp", nil),
		},
		{
			name:    "SPA",
			handler: &spaServer{},
			request: httptest.NewRequest(http.MethodPost, "/", nil),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cause := &hostileResponsePanic{}
			writer := &panicCommitResponseWriter{header: make(http.Header), cause: cause}
			recovered := captureResponsePanic(func() {
				test.handler.ServeHTTP(writer, test.request)
			})
			if recovered != http.ErrAbortHandler {
				t.Fatalf("panic = %#v, want http.ErrAbortHandler", recovered)
			}
			if calls := cause.calls.Load(); calls != 0 {
				t.Fatalf("hostile panic methods called %d times", calls)
			}
		})
	}
}

func invokeWithResponsePanicGuard(writer *trackedResponseWriter, fallback, operation func()) {
	returned := false
	defer func() {
		if returned {
			return
		}
		_ = recover()
		containResponsePanic(writer, fallback)
	}()
	operation()
	returned = true
}

func captureResponsePanic(operation func()) (recovered any) {
	defer func() { recovered = recover() }()
	operation()
	return nil
}

type hostileResponsePanic struct{ calls atomic.Int32 }

func (panicValue *hostileResponsePanic) Error() string {
	panicValue.calls.Add(1)
	return "hostile response panic"
}

func (panicValue *hostileResponsePanic) String() string {
	panicValue.calls.Add(1)
	return "hostile response panic"
}

type panicCommitResponseWriter struct {
	header http.Header
	cause  any
}

func (writer *panicCommitResponseWriter) Header() http.Header { return writer.header }

func (writer *panicCommitResponseWriter) WriteHeader(int) { panic(writer.cause) }

func (writer *panicCommitResponseWriter) Write([]byte) (int, error) { panic(writer.cause) }
