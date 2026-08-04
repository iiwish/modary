package httpapi_test

import (
	"io/fs"
	"net/http"
	"testing"
	"testing/fstest"
	"time"

	"github.com/iiwish/modary/transport/httpapi"
)

func TestExternalConsumerCanConstructPublicTransportSurface(t *testing.T) {
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>consumer</title>"), Mode: fs.FileMode(0o444)},
	}
	spa, err := httpapi.NewSPA(assets, httpapi.SPAOptions{})
	if err != nil {
		t.Fatalf("NewSPA() error = %v", err)
	}
	var _ http.Handler = spa

	if handler, err := httpapi.NewAPI(nil, httpapi.APIOptions{Timeout: time.Second}); err == nil || handler != nil {
		t.Fatalf("NewAPI(nil) = %#v, %v", handler, err)
	}
	if handler, err := httpapi.NewMCP(nil, httpapi.MCPOptions{RequestTimeout: time.Second}); err == nil || handler != nil {
		t.Fatalf("NewMCP(nil) = %#v, %v", handler, err)
	}
	if httpapi.MCPProtocolVersion == "" || httpapi.DefaultTimeout <= 0 || httpapi.DefaultMCPRequestTimeout <= 0 {
		t.Fatal("public protocol defaults are unavailable")
	}
}
