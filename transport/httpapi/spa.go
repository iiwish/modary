package httpapi

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Static asset defaults keep both bootstrap HTML and consumer-owned assets
// revalidated unless the consumer explicitly selects a stronger cache policy.
const (
	DefaultSPAIndexFile         = "index.html"
	DefaultSPAIndexCacheControl = "no-cache"
	DefaultSPAAssetCacheControl = "no-cache"
	DefaultSPAMaxFiles          = 4096
	MaximumSPAMaxFiles          = 100_000

	DefaultSPAMaxFileBytes  int64 = 16 << 20
	MaximumSPAMaxFileBytes  int64 = 64 << 20
	DefaultSPAMaxTotalBytes int64 = 64 << 20
	MaximumSPAMaxTotalBytes int64 = 512 << 20

	maximumCacheControlLength = 1024
)

// SPAOptions controls the bootstrap document and the cache policy applied to
// it and to all other static assets. Empty fields select the exported defaults.
type SPAOptions struct {
	IndexFile         string
	IndexCacheControl string
	AssetCacheControl string
	MaxFiles          int
	MaxFileBytes      int64
	MaxTotalBytes     int64
}

type spaServer struct {
	assets            map[string]spaAsset
	directories       map[string]struct{}
	index             spaAsset
	indexFile         string
	indexCacheControl string
	assetCacheControl string
}

type spaAsset struct {
	data        []byte
	modTime     time.Time
	etag        string
	contentType string
}

type spaSnapshot struct {
	assets      map[string]spaAsset
	directories map[string]struct{}
}

// NewSPA builds a static SPA handler over a filesystem supplied and owned by
// the consumer. The handler does not claim a route prefix; consumers mount it
// explicitly after mounting API and health endpoints.
func NewSPA(content fs.FS, options SPAOptions) (http.Handler, error) {
	if isNilSPAFileSystem(content) {
		return nil, fmt.Errorf("SPA content filesystem is required")
	}

	normalized, err := normalizeSPAOptions(options)
	if err != nil {
		return nil, err
	}
	snapshot, err := snapshotSPA(content, normalized)
	if err != nil {
		return nil, err
	}
	index, ok := snapshot.assets[normalized.IndexFile]
	if !ok {
		return nil, fmt.Errorf("SPA index %q does not exist", normalized.IndexFile)
	}
	if len(index.data) == 0 {
		return nil, fmt.Errorf("SPA index %q cannot be empty", normalized.IndexFile)
	}

	return &spaServer{
		assets:            snapshot.assets,
		directories:       snapshot.directories,
		index:             index,
		indexFile:         normalized.IndexFile,
		indexCacheControl: normalized.IndexCacheControl,
		assetCacheControl: normalized.AssetCacheControl,
	}, nil
}

func normalizeSPAOptions(options SPAOptions) (SPAOptions, error) {
	if options.IndexFile == "" {
		options.IndexFile = DefaultSPAIndexFile
	}
	if !validSPAFilePath(options.IndexFile) {
		return SPAOptions{}, fmt.Errorf("SPA index file %q is invalid", options.IndexFile)
	}
	if options.IndexCacheControl == "" {
		options.IndexCacheControl = DefaultSPAIndexCacheControl
	}
	if !validCacheControl(options.IndexCacheControl) {
		return SPAOptions{}, fmt.Errorf("SPA index Cache-Control value is invalid")
	}
	if options.AssetCacheControl == "" {
		options.AssetCacheControl = DefaultSPAAssetCacheControl
	}
	if !validCacheControl(options.AssetCacheControl) {
		return SPAOptions{}, fmt.Errorf("SPA asset Cache-Control value is invalid")
	}
	if options.MaxFiles < 0 {
		return SPAOptions{}, fmt.Errorf("SPA max files cannot be negative")
	}
	if options.MaxFiles == 0 {
		options.MaxFiles = DefaultSPAMaxFiles
	}
	if options.MaxFiles > MaximumSPAMaxFiles {
		return SPAOptions{}, fmt.Errorf("SPA max files cannot exceed %d", MaximumSPAMaxFiles)
	}
	if options.MaxFileBytes < 0 {
		return SPAOptions{}, fmt.Errorf("SPA max file bytes cannot be negative")
	}
	if options.MaxFileBytes == 0 {
		options.MaxFileBytes = DefaultSPAMaxFileBytes
	}
	if options.MaxFileBytes > MaximumSPAMaxFileBytes {
		return SPAOptions{}, fmt.Errorf("SPA max file bytes cannot exceed %d", MaximumSPAMaxFileBytes)
	}
	if options.MaxTotalBytes < 0 {
		return SPAOptions{}, fmt.Errorf("SPA max total bytes cannot be negative")
	}
	if options.MaxTotalBytes == 0 {
		options.MaxTotalBytes = DefaultSPAMaxTotalBytes
	}
	if options.MaxTotalBytes > MaximumSPAMaxTotalBytes {
		return SPAOptions{}, fmt.Errorf("SPA max total bytes cannot exceed %d", MaximumSPAMaxTotalBytes)
	}
	return options, nil
}

func snapshotSPA(content fs.FS, options SPAOptions) (snapshot spaSnapshot, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			snapshot = spaSnapshot{}
			err = fmt.Errorf("snapshot SPA filesystem: callback panicked")
		}
	}()
	snapshot, err = snapshotSPAUnchecked(content, options)
	returned = true
	return snapshot, err
}

func snapshotSPAUnchecked(content fs.FS, options SPAOptions) (snapshot spaSnapshot, err error) {
	snapshot.assets = make(map[string]spaAsset)
	snapshot.directories = make(map[string]struct{})
	var totalBytes int64
	visitedEntries := 0
	err = fs.WalkDir(content, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if dependencyErr := wrapDependencyError("walk SPA filesystem", walkErr); dependencyErr != nil {
			return dependencyErr
		}
		if entry == nil {
			return fmt.Errorf("SPA filesystem returned an empty directory entry")
		}
		if name != "." && !validSPAFilePath(name) {
			return fmt.Errorf("SPA filesystem path %q is invalid", name)
		}
		if name != "." {
			visitedEntries++
			if visitedEntries > options.MaxFiles*2 {
				return fmt.Errorf("SPA filesystem exceeds the bounded entry limit")
			}
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("SPA filesystem path %q is a symbolic link", name)
		}
		info, infoErr := entry.Info()
		if dependencyErr := wrapDependencyError("inspect SPA filesystem path "+strconv.Quote(name), infoErr); dependencyErr != nil {
			return dependencyErr
		}
		if info == nil {
			return fmt.Errorf("SPA filesystem path %q returned no file information", name)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("SPA filesystem path %q is a symbolic link", name)
		}
		if info.IsDir() {
			if name != "." {
				if len(snapshot.directories) >= options.MaxFiles {
					return fmt.Errorf("SPA filesystem exceeds the %d-directory limit", options.MaxFiles)
				}
				if _, duplicate := snapshot.directories[name]; duplicate {
					return fmt.Errorf("SPA filesystem directory %q is duplicated", name)
				}
				snapshot.directories[name] = struct{}{}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("SPA filesystem path %q is not a regular file", name)
		}
		if len(snapshot.assets) >= options.MaxFiles {
			return fmt.Errorf("SPA filesystem exceeds the %d-file limit", options.MaxFiles)
		}
		if _, duplicate := snapshot.assets[name]; duplicate {
			return fmt.Errorf("SPA filesystem path %q is duplicated", name)
		}
		data, stableInfo, readErr := readSPASnapshotFile(content, name, info, options.MaxFileBytes)
		if readErr != nil {
			return readErr
		}
		if int64(len(data)) > options.MaxTotalBytes-totalBytes {
			return fmt.Errorf("SPA filesystem exceeds the %d-byte total limit", options.MaxTotalBytes)
		}
		totalBytes += int64(len(data))
		snapshot.assets[name] = spaAsset{
			data:        data,
			modTime:     stableInfo.ModTime(),
			etag:        contentETag(data),
			contentType: spaContentType(name, data),
		}
		return nil
	})
	if err != nil {
		return spaSnapshot{}, fmt.Errorf("snapshot SPA filesystem: %w", err)
	}
	return snapshot, nil
}

func readSPASnapshotFile(content fs.FS, name string, walkInfo fs.FileInfo, limit int64) ([]byte, fs.FileInfo, error) {
	if walkInfo.Size() < 0 || walkInfo.Size() > limit {
		return nil, nil, fmt.Errorf("SPA file %q exceeds the %d-byte file limit", name, limit)
	}
	file, err := content.Open(name)
	if dependencyErr := wrapDependencyError("open SPA file "+strconv.Quote(name), err); dependencyErr != nil {
		return nil, nil, dependencyErr
	}
	closed := false
	closeFile := func() error {
		if closed {
			return nil
		}
		closed = true
		return wrapDependencyError("close SPA file "+strconv.Quote(name), file.Close())
	}
	defer func() { _ = closeFile() }()
	openedInfo, err := file.Stat()
	if dependencyErr := wrapDependencyError("inspect opened SPA file "+strconv.Quote(name), err); dependencyErr != nil {
		return nil, nil, errors.Join(dependencyErr, closeFile())
	}
	if !sameSPAFileState(walkInfo, openedInfo) {
		return nil, nil, errors.Join(fmt.Errorf("SPA file %q changed while it was opened", name), closeFile())
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	afterRead, statErr := file.Stat()
	closeErr := closeFile()
	pathInfo, pathErr := fs.Stat(content, name)
	readErr = wrapDependencyError("read SPA file "+strconv.Quote(name), readErr)
	statErr = wrapDependencyError("inspect SPA file after read "+strconv.Quote(name), statErr)
	pathErr = wrapDependencyError("inspect SPA filesystem path after read "+strconv.Quote(name), pathErr)
	if joined := errors.Join(readErr, statErr, closeErr, pathErr); joined != nil {
		return nil, nil, fmt.Errorf("read SPA file %q consistently: %w", name, joined)
	}
	if int64(len(data)) > limit {
		return nil, nil, fmt.Errorf("SPA file %q exceeds the %d-byte file limit", name, limit)
	}
	if !sameSPAFileState(openedInfo, afterRead) || !sameSPAFileState(afterRead, pathInfo) || int64(len(data)) != afterRead.Size() {
		return nil, nil, fmt.Errorf("SPA file %q changed while it was read", name)
	}
	return bytes.Clone(data), afterRead, nil
}

func sameSPAFileState(first, second fs.FileInfo) bool {
	return first != nil && second != nil && first.Name() == second.Name() &&
		first.Size() == second.Size() && first.Mode() == second.Mode() && first.ModTime().Equal(second.ModTime())
}

// ServeHTTP validates the request boundary and serves only immutable snapshot
// data, containing failures that occur before a response is committed.
func (server *spaServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	tracked := &trackedResponseWriter{ResponseWriter: writer}
	writer = tracked
	returned := false
	defer func() {
		if returned {
			return
		}
		_ = recover()
		containResponsePanic(tracked, func() {
			writeSPAStatus(writer, request.Method, http.StatusInternalServerError)
		})
	}()
	server.serve(writer, request)
	returned = true
}

func (server *spaServer) serve(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		writeSPAStatus(writer, request.Method, http.StatusMethodNotAllowed)
		return
	}

	requested, ok := validSPARequestPath(request)
	if !ok {
		writeSPAStatus(writer, request.Method, http.StatusBadRequest)
		return
	}
	if requested == server.indexFile {
		server.serveIndex(writer, request)
		return
	}
	if requested != "" {
		served := server.serveAsset(writer, request, requested)
		if served {
			return
		}
	}

	if requested != "" && path.Ext(requested) != "" {
		writeSPAStatus(writer, request.Method, http.StatusNotFound)
		return
	}
	writer.Header().Add("Vary", "Accept")
	if !acceptsHTML(request.Header.Values("Accept")) {
		writeSPAStatus(writer, request.Method, http.StatusNotFound)
		return
	}
	server.serveIndex(writer, request)
}

func (server *spaServer) serveAsset(writer http.ResponseWriter, request *http.Request, name string) bool {
	if _, directory := server.directories[name]; directory {
		writeSPAStatus(writer, request.Method, http.StatusNotFound)
		return true
	}
	asset, present := server.assets[name]
	if !present {
		return false
	}

	cacheControl := server.assetCacheControl
	if name == server.indexFile {
		cacheControl = server.indexCacheControl
	}
	serveSPAContent(writer, request, name, asset, cacheControl)
	return true
}

func (server *spaServer) serveIndex(writer http.ResponseWriter, request *http.Request) {
	serveSPAContent(writer, request, server.indexFile, server.index, server.indexCacheControl)
}

func serveSPAContent(writer http.ResponseWriter, request *http.Request, name string, asset spaAsset, cacheControl string) {
	writer.Header().Set("Cache-Control", cacheControl)
	writer.Header().Set("ETag", asset.etag)
	writer.Header().Set("Content-Type", asset.contentType)
	http.ServeContent(writer, request, name, asset.modTime, bytes.NewReader(asset.data))
}

func validSPAFilePath(name string) bool {
	return name != "." && fs.ValidPath(name) && !strings.Contains(name, "\\") &&
		!strings.ContainsRune(name, '\x00') && !strings.ContainsFunc(name, unicode.IsControl)
}

func isNilSPAFileSystem(content fs.FS) bool {
	if content == nil {
		return true
	}
	value := reflect.ValueOf(content)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func validSPARequestPath(request *http.Request) (string, bool) {
	requested := request.URL.Path
	if requested == "" {
		requested = "/"
	}
	if !strings.HasPrefix(requested, "/") || !utf8.ValidString(requested) ||
		strings.Contains(requested, "\\") || strings.ContainsRune(requested, '\x00') ||
		strings.ContainsFunc(requested, unicode.IsControl) || !validSPARawPath(request.URL.RawPath, requested) {
		return "", false
	}
	if requested != "/" && path.Clean(requested) != requested {
		return "", false
	}
	relative := strings.TrimPrefix(requested, "/")
	if relative == "" {
		return "", true
	}
	if !validSPAFilePath(relative) {
		return "", false
	}
	return relative, true
}

func validSPARawPath(rawPath, decodedPath string) bool {
	if rawPath == "" {
		return true
	}
	decoded, err := url.PathUnescape(rawPath)
	if err != nil || decoded != decodedPath {
		return false
	}
	for offset := 0; offset < len(rawPath); offset++ {
		if rawPath[offset] != '%' {
			continue
		}
		if offset+2 >= len(rawPath) {
			return false
		}
		value, err := strconv.ParseUint(rawPath[offset+1:offset+3], 16, 8)
		if err != nil {
			return false
		}
		switch byte(value) {
		case 0, '/', '\\':
			return false
		}
		offset += 2
	}
	return true
}

func spaContentType(name string, data []byte) string {
	contentType := mime.TypeByExtension(strings.ToLower(path.Ext(name)))
	if contentType == "" {
		contentType = http.DetectContentType(firstBytes(data, 512))
	}
	return contentType
}

func firstBytes(data []byte, count int) []byte {
	if len(data) <= count {
		return data
	}
	return data[:count]
}

func contentETag(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("\"sha256-%x\"", sum)
}

func writeSPAStatus(writer http.ResponseWriter, method string, status int) {
	body := []byte(http.StatusText(status) + "\n")
	writer.Header().Del("Accept-Ranges")
	writer.Header().Del("Content-Range")
	writer.Header().Del("ETag")
	writer.Header().Del("Last-Modified")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
	writer.WriteHeader(status)
	if method != http.MethodHead {
		_, _ = writer.Write(body)
	}
}

func acceptsHTML(values []string) bool {
	if len(values) == 0 {
		return true
	}
	bestSpecificity := -1
	bestQuality := -1.0
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			mediaType, parameters, err := mime.ParseMediaType(strings.TrimSpace(part))
			if err != nil {
				continue
			}
			quality := 1.0
			if rawQuality, present := parameters["q"]; present {
				quality, err = parseHTTPQuality(rawQuality)
				if err != nil {
					continue
				}
				delete(parameters, "q")
			}
			if charset, present := parameters["charset"]; present && strings.EqualFold(charset, "utf-8") {
				delete(parameters, "charset")
			}
			if len(parameters) != 0 {
				continue
			}
			specificity := -1
			switch strings.ToLower(mediaType) {
			case "text/html":
				specificity = 2
			case "text/*":
				specificity = 1
			case "*/*":
				specificity = 0
			}
			if specificity > bestSpecificity || specificity == bestSpecificity && quality > bestQuality {
				bestSpecificity = specificity
				bestQuality = quality
			}
		}
	}
	return bestSpecificity >= 0 && bestQuality > 0
}

func parseHTTPQuality(value string) (float64, error) {
	integer, fraction, hasFraction := strings.Cut(value, ".")
	if integer != "0" && integer != "1" || hasFraction && len(fraction) > 3 {
		return 0, fmt.Errorf("invalid quality")
	}
	for _, digit := range fraction {
		if digit < '0' || digit > '9' || integer == "1" && digit != '0' {
			return 0, fmt.Errorf("invalid quality")
		}
	}
	quality, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid quality: %w", err)
	}
	return quality, nil
}

func validCacheControl(value string) bool {
	if value == "" || len(value) > maximumCacheControlLength || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x20 || character >= 0x7f {
			return false
		}
	}

	remaining := value
	for {
		remaining = strings.TrimLeft(remaining, " ")
		directiveLength := scanHTTPToken(remaining)
		if directiveLength == 0 {
			return false
		}
		remaining = strings.TrimLeft(remaining[directiveLength:], " ")
		if strings.HasPrefix(remaining, "=") {
			remaining = strings.TrimLeft(remaining[1:], " ")
			if remaining == "" {
				return false
			}
			if remaining[0] == '"' {
				length, ok := scanHTTPQuotedString(remaining)
				if !ok {
					return false
				}
				remaining = remaining[length:]
			} else {
				valueLength := scanHTTPToken(remaining)
				if valueLength == 0 {
					return false
				}
				remaining = remaining[valueLength:]
			}
			remaining = strings.TrimLeft(remaining, " ")
		}
		if remaining == "" {
			return true
		}
		if remaining[0] != ',' {
			return false
		}
		remaining = remaining[1:]
		if strings.TrimSpace(remaining) == "" {
			return false
		}
	}
}

func scanHTTPToken(value string) int {
	for index, character := range []byte(value) {
		if !isHTTPTokenCharacter(character) {
			return index
		}
	}
	return len(value)
}

func isHTTPTokenCharacter(character byte) bool {
	if character >= '0' && character <= '9' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' {
		return true
	}
	return strings.ContainsRune("!#$%&'*+-.^_`|~", rune(character))
}

func scanHTTPQuotedString(value string) (int, bool) {
	for index := 1; index < len(value); index++ {
		switch value[index] {
		case '"':
			return index + 1, true
		case '\\':
			index++
			if index >= len(value) || value[index] < 0x20 || value[index] >= 0x7f {
				return 0, false
			}
		}
	}
	return 0, false
}
