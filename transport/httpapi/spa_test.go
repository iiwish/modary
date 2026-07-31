package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const (
	spaTestIndex = "<!doctype html><title>Modary</title>"
	spaTestJS    = "console.log('modary')"
)

func TestNewSPAValidatesFilesystemIndexAndOptions(t *testing.T) {
	if handler, err := NewSPA(nil, SPAOptions{}); err == nil || handler != nil {
		t.Fatalf("NewSPA(nil) = %#v, %v", handler, err)
	}
	var typedNil *spaTestNilFS
	if handler, err := NewSPA(typedNil, SPAOptions{}); err == nil || handler != nil {
		t.Fatalf("NewSPA(typed nil) = %#v, %v", handler, err)
	}

	valid := spaTestFiles()
	for _, indexFile := range []string{".", "/index.html", "../index.html", "ui//index.html", `ui\index.html`, "ui/../index.html", "index\x00.html", "index\n.html"} {
		indexFile := strings.ReplaceAll(strings.ReplaceAll(indexFile, `\x00`, "\x00"), `\n`, "\n")
		t.Run("invalid index "+url.PathEscape(indexFile), func(t *testing.T) {
			if handler, err := NewSPA(valid, SPAOptions{IndexFile: indexFile}); err == nil || handler != nil {
				t.Fatalf("NewSPA(IndexFile=%q) = %#v, %v", indexFile, handler, err)
			}
		})
	}
	for _, test := range []struct {
		name    string
		content fs.FS
		options SPAOptions
	}{
		{name: "missing index", content: fstest.MapFS{}},
		{name: "directory index", content: fstest.MapFS{"index.html/child": &fstest.MapFile{Data: []byte("x")}}},
		{name: "empty index", content: fstest.MapFS{"index.html": &fstest.MapFile{}}},
		{name: "unreadable index", content: &spaTestFaultFS{base: valid, failIndex: true}},
		{name: "index cache injection", content: valid, options: SPAOptions{IndexCacheControl: "no-cache\r\nX-Injected: true"}},
		{name: "asset cache injection", content: valid, options: SPAOptions{AssetCacheControl: "public\nX-Injected: true"}},
		{name: "cache leading whitespace", content: valid, options: SPAOptions{AssetCacheControl: " public"}},
		{name: "cache trailing comma", content: valid, options: SPAOptions{AssetCacheControl: "public,"}},
		{name: "cache missing value", content: valid, options: SPAOptions{AssetCacheControl: "max-age="}},
		{name: "cache malformed value", content: valid, options: SPAOptions{AssetCacheControl: "max-age=30 40"}},
		{name: "cache non ASCII", content: valid, options: SPAOptions{AssetCacheControl: "public, 私有"}},
		{name: "cache too long", content: valid, options: SPAOptions{AssetCacheControl: strings.Repeat("a", maximumCacheControlLength+1)}},
		{name: "negative file count", content: valid, options: SPAOptions{MaxFiles: -1}},
		{name: "excessive file count", content: valid, options: SPAOptions{MaxFiles: MaximumSPAMaxFiles + 1}},
		{name: "negative file bytes", content: valid, options: SPAOptions{MaxFileBytes: -1}},
		{name: "excessive file bytes", content: valid, options: SPAOptions{MaxFileBytes: MaximumSPAMaxFileBytes + 1}},
		{name: "negative total bytes", content: valid, options: SPAOptions{MaxTotalBytes: -1}},
		{name: "excessive total bytes", content: valid, options: SPAOptions{MaxTotalBytes: MaximumSPAMaxTotalBytes + 1}},
		{name: "file count limit", content: valid, options: SPAOptions{MaxFiles: 2}},
		{name: "directory count limit", content: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(spaTestIndex)},
			"empty-a":    &fstest.MapFile{Mode: fs.ModeDir | 0o755},
			"empty-b":    &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		}, options: SPAOptions{MaxFiles: 1}},
		{name: "per-file limit", content: valid, options: SPAOptions{MaxFileBytes: int64(len(spaTestIndex) - 1)}},
		{name: "total limit", content: valid, options: SPAOptions{MaxTotalBytes: int64(len(spaTestIndex) + len(spaTestJS))}},
		{name: "non-regular file", content: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(spaTestIndex)},
			"device":     &fstest.MapFile{Mode: fs.ModeDevice | 0o600},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if handler, err := NewSPA(test.content, test.options); err == nil || handler != nil {
				t.Fatalf("NewSPA() = %#v, %v", handler, err)
			}
		})
	}

	handler, err := NewSPA(valid, SPAOptions{
		IndexCacheControl: `private, max-age=0, no-cache="Set-Cookie"`,
		AssetCacheControl: "public, max-age=60",
	})
	if err != nil || handler == nil {
		t.Fatalf("NewSPA(valid options) = %#v, %v", handler, err)
	}
}

func TestSPAServesFilesAndFallsBackOnlyForHTMLRoutes(t *testing.T) {
	handler := spaTestHandler(t, spaTestFiles(), SPAOptions{})

	asset := spaDoRequest(handler, http.MethodGet, "/assets/app.js?v=1", "application/json", nil)
	if asset.Code != http.StatusOK || asset.Body.String() != spaTestJS {
		t.Fatalf("asset = %d %q", asset.Code, asset.Body.String())
	}
	if contentType := asset.Header().Get("Content-Type"); !strings.Contains(contentType, "javascript") {
		t.Fatalf("asset Content-Type = %q", contentType)
	}
	spaAssertContentHeaders(t, asset, DefaultSPAAssetCacheControl)

	head := spaDoRequest(handler, http.MethodHead, "/assets/app.js", "*/*", nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != "21" {
		t.Fatalf("HEAD asset = %d len=%d headers=%v", head.Code, head.Body.Len(), head.Header())
	}
	if head.Header().Get("ETag") != asset.Header().Get("ETag") {
		t.Fatalf("HEAD ETag = %q, GET ETag = %q", head.Header().Get("ETag"), asset.Header().Get("ETag"))
	}

	for _, target := range []string{"/", "/items/new"} {
		response := spaDoRequest(handler, http.MethodGet, target, "text/html,application/xhtml+xml;q=0.9", nil)
		if response.Code != http.StatusOK || response.Body.String() != spaTestIndex {
			t.Fatalf("fallback %q = %d %q", target, response.Code, response.Body.String())
		}
		spaAssertContentHeaders(t, response, DefaultSPAIndexCacheControl)
		if !strings.Contains(strings.Join(response.Header().Values("Vary"), ","), "Accept") {
			t.Fatalf("fallback Vary = %v", response.Header().Values("Vary"))
		}
	}
	absentAccept := spaDoRequest(handler, http.MethodGet, "/dashboard", "", nil)
	if absentAccept.Code != http.StatusOK || absentAccept.Body.String() != spaTestIndex {
		t.Fatalf("fallback without Accept = %d %q", absentAccept.Code, absentAccept.Body.String())
	}
	fallbackHead := spaDoRequest(handler, http.MethodHead, "/dashboard", "text/html", nil)
	if fallbackHead.Code != http.StatusOK || fallbackHead.Body.Len() != 0 || fallbackHead.Header().Get("Content-Length") != "36" {
		t.Fatalf("fallback HEAD = %d len=%d headers=%v", fallbackHead.Code, fallbackHead.Body.Len(), fallbackHead.Header())
	}

	index := spaDoRequest(handler, http.MethodGet, "/index.html", "text/html", nil)
	if index.Code != http.StatusOK || index.Body.String() != spaTestIndex || index.Header().Get("Cache-Control") != DefaultSPAIndexCacheControl {
		t.Fatalf("direct index = %d %q headers=%v", index.Code, index.Body.String(), index.Header())
	}

	extensionless := spaDoRequest(handler, http.MethodGet, "/robots", "application/json", nil)
	if extensionless.Code != http.StatusOK || extensionless.Body.String() != "allow" || extensionless.Header().Get("Cache-Control") != DefaultSPAAssetCacheControl {
		t.Fatalf("extensionless asset = %d %q", extensionless.Code, extensionless.Body.String())
	}

	for _, test := range []struct {
		name   string
		target string
		accept string
	}{
		{name: "route rejects JSON", target: "/items/new", accept: "application/json"},
		{name: "specific exclusion overrides wildcard", target: "/items/new", accept: "text/html;q=0, */*;q=1"},
		{name: "invalid exact quality is ignored", target: "/items/new", accept: "text/html;q=NaN, application/json"},
		{name: "missing extension", target: "/missing.json", accept: "text/html"},
		{name: "existing directory", target: "/assets", accept: "text/html"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := spaDoRequest(handler, http.MethodGet, test.target, test.accept, nil)
			if response.Code != http.StatusNotFound || response.Header().Get("Location") != "" {
				t.Fatalf("response = %d Location=%q body=%q", response.Code, response.Header().Get("Location"), response.Body.String())
			}
			spaAssertErrorHeaders(t, response)
		})
	}

	wildcard := spaDoRequest(handler, http.MethodGet, "/settings", "text/*;q=0.5", nil)
	if wildcard.Code != http.StatusOK || wildcard.Body.String() != spaTestIndex {
		t.Fatalf("text wildcard fallback = %d %q", wildcard.Code, wildcard.Body.String())
	}
	charset := spaDoRequest(handler, http.MethodGet, "/settings", "text/html; charset=utf-8", nil)
	if charset.Code != http.StatusOK {
		t.Fatalf("charset fallback = %d %q", charset.Code, charset.Body.String())
	}
	invalidQuality := spaDoRequest(handler, http.MethodGet, "/settings", "text/html;q=NaN, */*;q=1", nil)
	if invalidQuality.Code != http.StatusOK {
		t.Fatalf("fallback after invalid media range = %d %q", invalidQuality.Code, invalidQuality.Body.String())
	}
	missingHead := spaDoRequest(handler, http.MethodHead, "/missing.json", "text/html", nil)
	if missingHead.Code != http.StatusNotFound || missingHead.Body.Len() != 0 {
		t.Fatalf("missing HEAD = %d body=%q", missingHead.Code, missingHead.Body.String())
	}
}

func TestSPARejectsAmbiguousPathsAndUnsupportedMethods(t *testing.T) {
	handler := spaTestHandler(t, spaTestFiles(), SPAOptions{})

	method := spaDoRequest(handler, http.MethodPost, "/assets/app.js", "*/*", nil)
	if method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST = %d Allow=%q", method.Code, method.Header().Get("Allow"))
	}
	spaAssertErrorHeaders(t, method)

	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/../secret", nil),
		httptest.NewRequest(http.MethodGet, "/./assets/app.js", nil),
		httptest.NewRequest(http.MethodGet, "//assets/app.js", nil),
		httptest.NewRequest(http.MethodGet, "/assets/", nil),
		httptest.NewRequest(http.MethodGet, "/a%5Cb", nil),
		httptest.NewRequest(http.MethodGet, "/a%00b", nil),
		httptest.NewRequest(http.MethodGet, "/a%01b", nil),
		httptest.NewRequest(http.MethodGet, "/assets%2Fapp.js", nil),
		{Method: http.MethodGet, URL: &url.URL{Path: "/asset", RawPath: "/asset%"}, Header: make(http.Header)},
		{Method: http.MethodGet, URL: &url.URL{Path: "/asset", RawPath: "/different"}, Header: make(http.Header)},
	}
	for index, request := range requests {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || response.Header().Get("Location") != "" {
			t.Fatalf("invalid path %d (%q, raw %q) = %d Location=%q", index, request.URL.Path, request.URL.RawPath, response.Code, response.Header().Get("Location"))
		}
		spaAssertErrorHeaders(t, response)
	}
}

func TestSPACacheValidatorsRangesAndConfiguration(t *testing.T) {
	handler := spaTestHandler(t, spaTestFiles(), SPAOptions{
		IndexCacheControl: "private, max-age=0",
		AssetCacheControl: "public, max-age=60",
	})

	asset := spaDoRequest(handler, http.MethodGet, "/assets/app.js", "*/*", nil)
	if asset.Header().Get("Cache-Control") != "public, max-age=60" || asset.Header().Get("ETag") == "" {
		t.Fatalf("asset cache headers = %v", asset.Header())
	}
	notModified := spaDoRequest(handler, http.MethodGet, "/assets/app.js", "*/*", map[string]string{"If-None-Match": asset.Header().Get("ETag")})
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 || notModified.Header().Get("ETag") != asset.Header().Get("ETag") {
		t.Fatalf("conditional asset = %d body=%q headers=%v", notModified.Code, notModified.Body.String(), notModified.Header())
	}

	fallback := spaDoRequest(handler, http.MethodGet, "/items", "text/html", nil)
	if fallback.Header().Get("Cache-Control") != "private, max-age=0" || fallback.Header().Get("ETag") == "" {
		t.Fatalf("fallback cache headers = %v", fallback.Header())
	}
	fallbackNotModified := spaDoRequest(handler, http.MethodHead, "/items", "text/html", map[string]string{"If-None-Match": fallback.Header().Get("ETag")})
	if fallbackNotModified.Code != http.StatusNotModified || fallbackNotModified.Body.Len() != 0 {
		t.Fatalf("conditional fallback HEAD = %d body=%q", fallbackNotModified.Code, fallbackNotModified.Body.String())
	}

	rangeResponse := spaDoRequest(handler, http.MethodGet, "/assets/app.js", "*/*", map[string]string{"Range": "bytes=0-7"})
	if rangeResponse.Code != http.StatusPartialContent || rangeResponse.Body.String() != "console." || rangeResponse.Header().Get("Content-Range") != "bytes 0-7/21" {
		t.Fatalf("range = %d %q headers=%v", rangeResponse.Code, rangeResponse.Body.String(), rangeResponse.Header())
	}
}

func TestSPAFilesystemSnapshotFailuresAreContained(t *testing.T) {
	for _, test := range []struct {
		name      string
		panicRead bool
	}{
		{name: "read error"},
		{name: "panic", panicRead: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := &spaTestFaultFS{base: spaTestFiles(), failAsset: true, panicAsset: test.panicRead}
			handler, err := NewSPA(content, SPAOptions{})
			if err == nil || handler != nil {
				t.Fatalf("NewSPA(fault) = %#v, %v", handler, err)
			}
			if strings.Contains(err.Error(), "filesystem secret") {
				t.Fatalf("filesystem failure leaked through constructor error: %v", err)
			}
		})
	}
}

func TestSPASnapshotIsImmutableAndRejectsSymbolicLinks(t *testing.T) {
	content := spaTestFiles()
	handler := spaTestHandler(t, content, SPAOptions{})
	content["assets/app.js"].Data = []byte("mutated")
	delete(content, "index.html")

	asset := spaDoRequest(handler, http.MethodGet, "/assets/app.js", "*/*", nil)
	if asset.Code != http.StatusOK || asset.Body.String() != spaTestJS {
		t.Fatalf("snapshotted asset = %d %q", asset.Code, asset.Body.String())
	}
	index := spaDoRequest(handler, http.MethodGet, "/", "text/html", nil)
	if index.Code != http.StatusOK || index.Body.String() != spaTestIndex {
		t.Fatalf("snapshotted index = %d %q", index.Code, index.Body.String())
	}
	faulting := &spaTestFaultFS{base: spaTestFiles()}
	faultingHandler := spaTestHandler(t, faulting, SPAOptions{})
	faulting.failAsset = true
	faulting.panicAsset = true
	fromSnapshot := spaDoRequest(faultingHandler, http.MethodGet, "/assets/app.js", "*/*", nil)
	if fromSnapshot.Code != http.StatusOK || fromSnapshot.Body.String() != spaTestJS {
		t.Fatalf("runtime consulted the source filesystem: %d %q", fromSnapshot.Code, fromSnapshot.Body.String())
	}

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte(spaTestIndex), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "target.js"), []byte(spaTestJS), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.js", filepath.Join(directory, "linked.js")); err != nil {
		t.Fatal(err)
	}
	if linked, err := NewSPA(os.DirFS(directory), SPAOptions{}); err == nil || linked != nil {
		t.Fatalf("NewSPA(symlink) = %#v, %v", linked, err)
	}
}

func spaTestFiles() fstest.MapFS {
	modTime := time.Date(2026, time.July, 30, 9, 0, 0, 0, time.UTC)
	return fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte(spaTestIndex), ModTime: modTime},
		"assets/app.js": &fstest.MapFile{Data: []byte(spaTestJS), ModTime: modTime},
		"robots":        &fstest.MapFile{Data: []byte("allow"), ModTime: modTime},
	}
}

func spaTestHandler(t *testing.T, content fs.FS, options SPAOptions) http.Handler {
	t.Helper()
	handler, err := NewSPA(content, options)
	if err != nil {
		t.Fatalf("NewSPA() error = %v", err)
	}
	return handler
}

func spaDoRequest(handler http.Handler, method, target, accept string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func spaAssertContentHeaders(t *testing.T, response *httptest.ResponseRecorder, cacheControl string) {
	t.Helper()
	if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Cache-Control") != cacheControl || response.Header().Get("ETag") == "" {
		t.Fatalf("content headers = %v", response.Header())
	}
}

func spaAssertErrorHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
		t.Fatalf("error headers = %v", response.Header())
	}
}

type spaTestNilFS struct{}

func (*spaTestNilFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

type spaTestFaultFS struct {
	base       fstest.MapFS
	failIndex  bool
	failAsset  bool
	panicAsset bool
}

func (content *spaTestFaultFS) Open(name string) (fs.File, error) {
	if content.failIndex && name == "index.html" {
		return nil, errors.New("filesystem secret: index open failed")
	}
	if content.failAsset && name == "assets/app.js" {
		if content.panicAsset {
			panic("filesystem secret: asset open panic")
		}
		return nil, errors.New("filesystem secret: asset open failed")
	}
	return content.base.Open(name)
}

func (content *spaTestFaultFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(content.base, name)
}

func (content *spaTestFaultFS) ReadFile(name string) ([]byte, error) {
	if content.failIndex && name == "index.html" {
		return nil, errors.New("filesystem secret: index read failed")
	}
	if content.failAsset && name == "assets/app.js" {
		if content.panicAsset {
			panic("filesystem secret: asset read panic")
		}
		return nil, errors.New("filesystem secret: asset read failed")
	}
	return fs.ReadFile(content.base, name)
}
