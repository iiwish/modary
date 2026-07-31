package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
)

func TestDecodeRequestJSONDoesNotDispatchBodyErrorMethods(t *testing.T) {
	hostile := &hostileBodyReadError{}
	cause := errors.Join(hostile, context.Canceled)
	request := httptest.NewRequest("POST", "/", nil)
	request.Body = bodyReadFailure{cause: cause}
	err := decodeRequestJSON(httptest.NewRecorder(), request, 1024, &struct{}{})
	if err == nil || err.Error() != "read HTTP request body failed: opaque dependency failure" {
		t.Fatalf("decodeRequestJSON error = %v", err)
	}
	var found *hostileBodyReadError
	if !errors.Is(err, cause) || !errors.Is(err, hostile) || !errors.Is(err, context.Canceled) ||
		!errors.As(err, &found) || found != hostile {
		t.Fatal("request body error did not retain its safely inspectable cause graph")
	}
}

type bodyReadFailure struct{ cause error }

func (body bodyReadFailure) Read([]byte) (int, error) { return 0, body.cause }
func (bodyReadFailure) Close() error                  { return nil }

type hostileBodyReadError struct{}

func (*hostileBodyReadError) Error() string { panic("hostile body Error invoked") }
func (*hostileBodyReadError) Is(error) bool { panic("hostile body Is invoked") }
func (*hostileBodyReadError) As(any) bool   { panic("hostile body As invoked") }
func (*hostileBodyReadError) Unwrap() error { panic("hostile body Unwrap invoked") }
