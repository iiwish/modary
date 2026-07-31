// Package safeerr classifies errors received across dependency boundaries
// without invoking caller-defined Is, As, or Unwrap methods.
package safeerr

import (
	"reflect"
)

const maxNodes = 64

const opaqueDiagnostic = "opaque dependency failure"

var trustedFrameworkPackages = map[string]struct{}{
	"github.com/iiwish/modary/action":                      {},
	"github.com/iiwish/modary/appcmd":                      {},
	"github.com/iiwish/modary/database":                    {},
	"github.com/iiwish/modary/internal/actionruntime":      {},
	"github.com/iiwish/modary/internal/databasecontrol":    {},
	"github.com/iiwish/modary/internal/safeerr":            {},
	"github.com/iiwish/modary/internal/transactionoutcome": {},
	"github.com/iiwish/modary/module":                      {},
	"github.com/iiwish/modary/projecttool":                 {},
}

var trustedStandardPackages = map[string]struct{}{
	"context":  {},
	"errors":   {},
	"fmt":      {},
	"io/fs":    {},
	"net":      {},
	"net/http": {},
	"net/url":  {},
	"os":       {},
	"os/exec":  {},
	"syscall":  {},
}

// Diagnostic returns err's diagnostic unless err is nil or typed nil. Nil
// values receive a stable opaque diagnostic without invoking any error method.
// A panic from a non-nil Error method is intentionally left to the caller's
// panic boundary.
func Diagnostic(err error) string {
	if isNil(err) {
		return opaqueDiagnostic
	}
	return err.Error()
}

// Is reports whether root contains target. It compares error values directly,
// never invokes custom Is or As methods, and unwraps only standard-library or
// explicitly trusted framework error types. Traversal is bounded.
func Is(root, target error) bool {
	if target == nil {
		return root == nil
	}
	return Walk(root, func(candidate error) bool {
		return same(candidate, target)
	})
}

// Find returns the first error in root's bounded trusted graph assignable to T.
func Find[T error](root error) (T, bool) {
	var result T
	found := Walk(root, func(candidate error) bool {
		var ok bool
		result, ok = candidate.(T)
		return ok && !isNil(result)
	})
	return result, found
}

// Assign stores the first error in root's bounded trusted graph that is
// assignable to the value addressed by target. It never invokes a custom As
// method. Invalid or nil targets simply do not match; callers using errors.As
// still receive that function's ordinary target validation before Assign is
// reached through an opaque boundary.
func Assign(root error, target any) (matched bool) {
	targetValue := reflect.ValueOf(target)
	if !targetValue.IsValid() || targetValue.Kind() != reflect.Pointer || targetValue.IsNil() {
		return false
	}
	destination := targetValue.Elem()
	if !destination.IsValid() || !destination.CanSet() {
		return false
	}
	targetType := destination.Type()
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			matched = false
		}
	}()
	matched = Walk(root, func(candidate error) bool {
		value := reflect.ValueOf(candidate)
		if !value.IsValid() || !value.Type().AssignableTo(targetType) {
			return false
		}
		destination.Set(value)
		return true
	})
	returned = true
	return matched
}

// Opaque retains cause behind an errors.Is/errors.As-compatible boundary that
// never exposes cause through Unwrap. Standard inspection can match the raw
// cause, trusted nested sentinels, and assignable error types without invoking
// caller-defined Is, As, or Unwrap methods.
func Opaque(cause error) error {
	if cause == nil {
		return nil
	}
	if _, ok := cause.(*opaqueError); ok {
		return cause
	}
	return &opaqueError{cause: cause}
}

type opaqueError struct{ cause error }

func (*opaqueError) Error() string { return opaqueDiagnostic }

func (err *opaqueError) Is(target error) bool {
	return err != nil && Is(err.cause, target)
}

func (err *opaqueError) As(target any) bool {
	return err != nil && Assign(err.cause, target)
}

// Walk visits at most 64 errors in root's trusted unwrap graph. The visitor
// must not retain or mutate errors that are owned by another goroutine.
func Walk(root error, visit func(error) bool) bool {
	if root == nil || visit == nil {
		return false
	}
	stack := []error{root}
	for visited := 0; len(stack) > 0 && visited < maxNodes; visited++ {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if current == nil {
			continue
		}
		if visit(current) {
			return true
		}
		children := unwrapTrusted(current)
		remaining := maxNodes - visited - 1
		if len(children) > remaining {
			children = children[:remaining]
		}
		for index := len(children) - 1; index >= 0; index-- {
			stack = append(stack, children[index])
		}
	}
	return false
}

func unwrapTrusted(err error) (children []error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			children = nil
		}
	}()
	children = unwrapTrustedUnchecked(err)
	returned = true
	return children
}

func unwrapTrustedUnchecked(err error) []error {
	if isNil(err) {
		return nil
	}
	if opaque, ok := err.(*opaqueError); ok {
		if opaque != nil && opaque.cause != nil {
			return []error{opaque.cause}
		}
		return nil
	}
	if !trustedUnwrapper(reflect.TypeOf(err)) {
		return nil
	}
	if many, ok := err.(interface{ Unwrap() []error }); ok {
		values := many.Unwrap()
		if len(values) > maxNodes {
			values = values[:maxNodes]
		}
		return append([]error(nil), values...)
	}
	if one, ok := err.(interface{ Unwrap() error }); ok {
		if child := one.Unwrap(); child != nil {
			return []error{child}
		}
	}
	return nil
}

func trustedUnwrapper(errorType reflect.Type) bool {
	if errorType == nil {
		return false
	}
	for errorType.Kind() == reflect.Pointer {
		errorType = errorType.Elem()
	}
	packagePath := errorType.PkgPath()
	if packagePath == "" {
		return false
	}
	return trustedPackagePath(packagePath)
}

func trustedPackagePath(packagePath string) bool {
	if _, trusted := trustedStandardPackages[packagePath]; trusted {
		return true
	}
	_, trusted := trustedFrameworkPackages[packagePath]
	return trusted
}

func same(first, second error) (result bool) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			result = false
		}
	}()
	firstType := reflect.TypeOf(first)
	result = firstType != nil && firstType == reflect.TypeOf(second) && firstType.Comparable() && first == second
	returned = true
	return result
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
