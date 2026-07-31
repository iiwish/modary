package action

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxDescriptorErrors = 64
	maxErrorCodeBytes   = 64

	// MaxErrorMessageRunes bounds every consumer-visible Action error message.
	MaxErrorMessageRunes = 512
)

var customErrorCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,30}\.[A-Z][A-Z0-9_]{0,31}$`)

// ValidCustomErrorCode reports whether code is a bounded namespace-qualified
// consumer error code. Descriptor validation additionally rejects framework
// codes and duplicate declarations.
func ValidCustomErrorCode(code string) bool {
	return len(code) <= maxErrorCodeBytes && customErrorCodePattern.MatchString(code)
}

// ValidErrorMessage reports whether message is safe for direct presentation
// across Action transports and audit records.
func ValidErrorMessage(message string) bool {
	return utf8.ValidString(message) && message != "" && strings.TrimSpace(message) == message &&
		utf8.RuneCountInString(message) <= MaxErrorMessageRunes &&
		!strings.ContainsFunc(message, func(character rune) bool {
			return unicode.IsControl(character) || character == '\u2028' || character == '\u2029'
		})
}

// ErrorKind is the transport-independent semantic class of a governed Action
// error. Transports may map a kind to their native status representation, while
// the declared error code remains the stable consumer-facing identifier.
type ErrorKind string

const (
	// ErrorKindValidation identifies malformed or semantically invalid caller input.
	ErrorKindValidation ErrorKind = "validation"
	// ErrorKindDenied identifies an authorization denial.
	ErrorKindDenied ErrorKind = "denied"
	// ErrorKindNotFound identifies a requested resource that does not exist.
	ErrorKindNotFound ErrorKind = "not_found"
	// ErrorKindPrecondition identifies a supplied precondition that is not current.
	ErrorKindPrecondition ErrorKind = "precondition"
	// ErrorKindPreconditionRequired identifies a required precondition that is absent.
	ErrorKindPreconditionRequired ErrorKind = "precondition_required"
	// ErrorKindConflict identifies a request that conflicts with current state.
	ErrorKindConflict ErrorKind = "conflict"
	// ErrorKindLimit identifies a request or authorized impact outside a declared limit.
	ErrorKindLimit ErrorKind = "limit"
	// ErrorKindUnavailable identifies a temporarily unavailable governed operation.
	ErrorKindUnavailable ErrorKind = "unavailable"
	// ErrorKindInternal identifies an unexpected framework or consumer failure.
	ErrorKindInternal ErrorKind = "internal"
)

// Valid reports whether kind is one of the closed ErrorKind values understood
// by the framework.
func (kind ErrorKind) Valid() bool {
	switch kind {
	case ErrorKindValidation,
		ErrorKindDenied,
		ErrorKindNotFound,
		ErrorKindPrecondition,
		ErrorKindPreconditionRequired,
		ErrorKindConflict,
		ErrorKindLimit,
		ErrorKindUnavailable,
		ErrorKindInternal:
		return true
	default:
		return false
	}
}

// ErrorSpec declares one consumer-owned public error code and its stable
// semantic kind. Code must contain exactly two uppercase dot-separated
// segments and must not reuse a framework-owned code.
type ErrorSpec struct {
	Code string    `json:"code"`
	Kind ErrorKind `json:"kind"`
}

var builtinErrorKinds = map[string]ErrorKind{
	CodeActionNotFound:      ErrorKindNotFound,
	CodeValidationFailed:    ErrorKindValidation,
	CodeAuthzDenied:         ErrorKindDenied,
	CodePreconditionFailed:  ErrorKindPrecondition,
	CodePlanRequired:        ErrorKindPreconditionRequired,
	CodePlanNotFound:        ErrorKindNotFound,
	CodePlanStale:           ErrorKindConflict,
	CodeLimitExceeded:       ErrorKindLimit,
	CodeIdempotencyRequired: ErrorKindPreconditionRequired,
	CodeIdempotencyConflict: ErrorKindConflict,
	CodeIdempotencyProgress: ErrorKindConflict,
	CodeUnavailable:         ErrorKindUnavailable,
	CodeInternal:            ErrorKindInternal,
}

// BuiltinErrorKind returns the semantic kind of a framework-owned error code.
// Descriptor.Errors cannot redeclare these codes.
func BuiltinErrorKind(code string) (ErrorKind, bool) {
	kind, ok := builtinErrorKinds[code]
	return kind, ok
}
