package httpkit

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Route is one explicitly selected HTTP method, path, and handler.
type Route struct {
	Method  string
	Path    string
	Handler http.Handler
}

// NewHandler validates the complete route set before returning a standard Go
// HTTP handler. Duplicate method and path pairs fail instead of replacing an
// earlier component route.
func NewHandler(routes ...Route) (http.Handler, error) {
	mux := http.NewServeMux()
	seen := make(map[string]struct{}, len(routes))
	for index, route := range routes {
		if !validMethod(route.Method) {
			return nil, fmt.Errorf("http route %d method %q is invalid", index, route.Method)
		}
		if !validPath(route.Path) {
			return nil, fmt.Errorf("http route %d path %q is invalid", index, route.Path)
		}
		if isNilHandler(route.Handler) {
			return nil, fmt.Errorf("http route %d handler is required", index)
		}
		pattern := route.Method + " " + route.Path
		if _, exists := seen[pattern]; exists {
			return nil, fmt.Errorf("duplicate http route %s", pattern)
		}
		if err := register(mux, pattern, route.Handler); err != nil {
			return nil, fmt.Errorf("http route %d path %q is invalid: %w", index, route.Path, err)
		}
		seen[pattern] = struct{}{}
	}
	return mux, nil
}

func register(mux *http.ServeMux, pattern string, handler http.Handler) (err error) {
	err = fmt.Errorf("standard HTTP pattern was rejected")
	defer func() {
		_ = recover()
	}()
	mux.Handle(pattern, handler)
	return nil
}

func validMethod(method string) bool {
	if method == "" || method != strings.ToUpper(method) {
		return false
	}
	for index := 0; index < len(method); index++ {
		value := method[index]
		if (value >= 'A' && value <= 'Z') || (value >= '0' && value <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(value)) {
			continue
		}
		return false
	}
	return true
}

func validPath(value string) bool {
	if value == "" || value[0] != '/' || !utf8.ValidString(value) {
		return false
	}
	return !strings.ContainsFunc(value, func(character rune) bool {
		return character <= ' ' || character == '\x7f'
	})
}

func isNilHandler(handler http.Handler) bool {
	if handler == nil {
		return true
	}
	value := reflect.ValueOf(handler)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
