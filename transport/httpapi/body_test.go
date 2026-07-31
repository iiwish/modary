package httpapi

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestDeadlineInterruptsBlockingBodyRead(t *testing.T) {
	for _, test := range []struct {
		name    string
		handler func(*testing.T) http.Handler
		request func(io.ReadCloser) *http.Request
	}{
		{
			name: "API",
			handler: func(t *testing.T) http.Handler {
				application := newHTTPTestApplication(t, true)
				return mustNewAPI(t, application.app, APIOptions{Timeout: 20 * time.Millisecond})
			},
			request: func(body io.ReadCloser) *http.Request {
				request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
				request.Body = body
				request.ContentLength = -1
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Accept", "application/json")
				return request
			},
		},
		{
			name: "MCP",
			handler: func(t *testing.T) http.Handler {
				application := newMCPTestApplication(t)
				return mustNewMCP(t, application.app, MCPOptions{RequestTimeout: 20 * time.Millisecond})
			},
			request: func(body io.ReadCloser) *http.Request {
				request := newMCPRequest(http.MethodPost, "")
				request.Body = body
				request.ContentLength = -1
				return request
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := newBlockingRequestBody()
			handler := test.handler(t)
			request := test.request(body)
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				handler.ServeHTTP(response, request)
			}()

			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("handler did not interrupt a blocking request body at its deadline")
			}
			if response.Code != http.StatusGatewayTimeout {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusGatewayTimeout, response.Body.String())
			}
			if calls := body.closeCalls.Load(); calls != 1 {
				t.Fatalf("Body.Close calls = %d, want 1", calls)
			}
		})
	}
}

func TestRequestBodyClosePanicIsContained(t *testing.T) {
	application := newHTTPTestApplication(t, true)
	handler := mustNewAPI(t, application.app, APIOptions{})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	request.Body = &panicCloseRequestBody{Reader: strings.NewReader(`{"username":"admin","password":"secret"}`)}
	request.ContentLength = -1
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
}

type blockingRequestBody struct {
	closed     chan struct{}
	closeOnce  sync.Once
	closeCalls atomic.Int32
}

func newBlockingRequestBody() *blockingRequestBody {
	return &blockingRequestBody{closed: make(chan struct{})}
}

func (body *blockingRequestBody) Read([]byte) (int, error) {
	<-body.closed
	return 0, errors.New("request body was closed")
}

func (body *blockingRequestBody) Close() error {
	body.closeCalls.Add(1)
	body.closeOnce.Do(func() { close(body.closed) })
	return nil
}

type panicCloseRequestBody struct{ *strings.Reader }

func (*panicCloseRequestBody) Close() error { panic("request body close secret") }
