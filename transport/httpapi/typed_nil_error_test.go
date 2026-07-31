package httpapi

import (
	"errors"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDependencyErrorsFailClosedOnTypedNil(t *testing.T) {
	var cause *typedNilHTTPDependencyError
	if err := wrapDependencyError("nil dependency", nil); err != nil {
		t.Fatalf("nil dependency error = %v", err)
	}
	assertTypedNilHTTPDependencyFailure(t, wrapDependencyError("typed-nil dependency", cause), cause)

	request := httptest.NewRequest("POST", "/", nil)
	request.Body = typedNilHTTPBody{err: cause}
	err := decodeRequestJSON(httptest.NewRecorder(), request, 1024, &struct{}{})
	assertTypedNilHTTPDependencyFailure(t, err, cause)

	handler, err := NewSPA(typedNilHTTPFileSystem{err: cause}, SPAOptions{})
	if handler != nil {
		t.Fatal("typed-nil SPA filesystem error returned a handler")
	}
	assertTypedNilHTTPDependencyFailure(t, err, cause)
}

func assertTypedNilHTTPDependencyFailure(t *testing.T, err error, cause *typedNilHTTPDependencyError) {
	t.Helper()
	if err == nil {
		t.Fatal("typed-nil dependency error was treated as success")
	}
	if got := err.Error(); got == "" || strings.Contains(got, "dependency secret") {
		t.Fatalf("typed-nil dependency diagnostic = %q", got)
	}
	var found *typedNilHTTPDependencyError
	if !errors.Is(err, cause) || !errors.As(err, &found) || found != cause {
		t.Fatalf("typed-nil dependency cause was not safely preserved: Is=%t As=%t value=%#v",
			errors.Is(err, cause), errors.As(err, &found), found)
	}
}

type typedNilHTTPDependencyError struct{}

func (*typedNilHTTPDependencyError) Error() string { panic("dependency secret Error invoked") }
func (*typedNilHTTPDependencyError) Is(error) bool { panic("dependency secret Is invoked") }
func (*typedNilHTTPDependencyError) As(any) bool   { panic("dependency secret As invoked") }
func (*typedNilHTTPDependencyError) Unwrap() error { panic("dependency secret Unwrap invoked") }

type typedNilHTTPBody struct{ err error }

func (body typedNilHTTPBody) Read([]byte) (int, error) { return 0, body.err }
func (typedNilHTTPBody) Close() error                  { return nil }

type typedNilHTTPFileSystem struct{ err error }

func (content typedNilHTTPFileSystem) Open(string) (fs.File, error) { return nil, content.err }
