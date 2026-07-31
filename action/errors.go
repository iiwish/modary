package action

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/safeerr"
	"github.com/iiwish/modary/scope"
)

// ErrCallbackPanic identifies a recovered panic from an Action callback.
var ErrCallbackPanic = errors.New("action callback panic")

// CallbackPanicError reports which Action callback panicked without retaining
// the recovered value. Panic values may contain secrets or implement unsafe
// formatting methods, so they never cross the Runtime boundary.
type CallbackPanicError struct {
	Operation string
}

// Error returns a stable description that never formats the recovered value.
func (err *CallbackPanicError) Error() string {
	if err == nil || err.Operation == "" {
		return "action callback panicked"
	}
	return err.Operation + " panicked"
}

// Unwrap identifies the failure as ErrCallbackPanic.
func (err *CallbackPanicError) Unwrap() error {
	if err == nil {
		return nil
	}
	return ErrCallbackPanic
}

const (
	// CodeActionNotFound identifies a request for an unregistered Action.
	CodeActionNotFound = "ACTION_NOT_FOUND"
	// CodeValidationFailed identifies an invalid request or JSON value.
	CodeValidationFailed = "VALIDATION_FAILED"
	// CodeAuthzDenied identifies a denied authorization decision.
	CodeAuthzDenied = "AUTHZ_DENIED"
	// CodePreconditionFailed identifies an unmet Action precondition.
	CodePreconditionFailed = "PRECONDITION_FAILED"
	// CodePlanRequired identifies execution attempted without a required Preview plan.
	CodePlanRequired = "PLAN_REQUIRED"
	// CodePlanNotFound identifies a requested Preview plan that does not exist.
	CodePlanNotFound = "PLAN_NOT_FOUND"
	// CodePlanStale identifies a Preview plan whose bindings are no longer current.
	CodePlanStale = "PLAN_STALE"
	// CodeLimitExceeded identifies an authorized impact that exceeds its constraints.
	CodeLimitExceeded = "LIMIT_EXCEEDED"
	// CodeIdempotencyRequired identifies a missing required idempotency key.
	CodeIdempotencyRequired = "IDEMPOTENCY_REQUIRED"
	// CodeIdempotencyConflict identifies reuse of a key for a different execution.
	CodeIdempotencyConflict = "IDEMPOTENCY_CONFLICT"
	// CodeIdempotencyProgress identifies an execution already in progress for a key.
	CodeIdempotencyProgress = "IDEMPOTENCY_IN_PROGRESS"
	// CodeUnavailable identifies a Runtime that is no longer accepting execution.
	CodeUnavailable = "UNAVAILABLE"
	// CodeInternal identifies an unexpected framework, adapter, or Handler failure.
	CodeInternal = "INTERNAL_ERROR"
)

// Error is the stable, request-aware error envelope returned by the Action
// Runtime. Cause participates in errors.Is and errors.As through an opaque
// boundary that never dispatches caller-defined error methods. It is not
// serialized.
type Error struct {
	Code               string           `json:"error_code"`
	Kind               ErrorKind        `json:"error_kind"`
	Message            string           `json:"human_readable_reason"`
	ActionID           string           `json:"action_id,omitempty"`
	RequiredPermission string           `json:"required_permission,omitempty"`
	ActorID            string           `json:"actor_id,omitempty"`
	Scope              *scope.Execution `json:"scope,omitempty"`
	RequestID          string           `json:"request_id,omitempty"`
	Cause              error            `json:"-"`
}

// Error returns the stable public code and message without formatting Cause.
func (e *Error) Error() string {
	if e == nil {
		return "INTERNAL_ERROR: action execution failed"
	}
	code := e.Code
	if code == "" {
		code = CodeInternal
	}
	message := e.Message
	if message == "" {
		message = "action execution failed"
	}
	return code + ": " + message
}

// Unwrap exposes Cause through a safe opaque error-chain boundary.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return safeerr.Opaque(e.Cause)
}

// NewError constructs an Action error with the supplied stable code and message.
func NewError(code, message string) *Error {
	kind, _ := BuiltinErrorKind(code)
	return &Error{Code: code, Kind: kind, Message: message}
}

// ErrorCode returns the code of the first Error in the bounded trusted unwrap
// graph, or CodeInternal when no Action error is found. Caller-defined Is, As,
// and Unwrap methods are never invoked.
func ErrorCode(err error) string {
	if actionErr, ok := findActionError(err); ok {
		if actionErr.Code == "" {
			return CodeInternal
		}
		return actionErr.Code
	}
	return CodeInternal
}

// ErrorKindOf returns the authoritative semantic kind carried by the first
// Error in the bounded trusted unwrap graph. A missing or invalid kind falls
// back to the stable built-in code mapping; otherwise it fails closed as
// ErrorKindInternal.
func ErrorKindOf(err error) ErrorKind {
	actionErr, ok := findActionError(err)
	if !ok {
		return ErrorKindInternal
	}
	if kind, builtin := BuiltinErrorKind(actionErr.Code); builtin {
		if actionErr.Kind == "" || actionErr.Kind == kind {
			return kind
		}
		return ErrorKindInternal
	}
	if actionErr.Kind.Valid() {
		return actionErr.Kind
	}
	return ErrorKindInternal
}

// IsKind reports whether err contains an Error with the supplied governed
// semantic kind.
func IsKind(err error, kind ErrorKind) bool {
	return kind.Valid() && ErrorKindOf(err) == kind
}

// IsCode reports whether err contains an Error with the supplied code using
// the same bounded traversal as ErrorCode.
func IsCode(err error, code string) bool {
	actionErr, ok := findActionError(err)
	if !ok {
		return false
	}
	actual := actionErr.Code
	if actual == "" {
		actual = CodeInternal
	}
	return actual == code
}

// WithRequest enriches err with request and permission context without mutating
// an existing Error in its chain.
func WithRequest(err error, request Request, permission string) error {
	if err == nil {
		return nil
	}
	actionErr, ok := findActionError(err)
	if !ok {
		actionErr = &Error{Code: CodeInternal, Message: "action execution failed", Cause: err}
	} else {
		clone := *actionErr
		// Preserve the selected governed error's original cause instead of
		// wrapping the entire graph around a second copy of the same Error.
		// Handler validation deliberately requires one unambiguous Error.
		actionErr = &clone
	}
	if !actionErr.Kind.Valid() {
		actionErr.Kind, _ = BuiltinErrorKind(actionErr.Code)
	}
	// Context always describes this request. Clear every cloned field before
	// selectively copying validated values so an enriched Error can be safely
	// reused without retaining another request's identity or scope.
	actionErr.ActionID = ""
	actionErr.RequiredPermission = ""
	actionErr.ActorID = ""
	actionErr.Scope = nil
	actionErr.RequestID = ""
	if ValidIdentifier(request.ActionID) {
		actionErr.ActionID = request.ActionID
	}
	if ValidIdentifier(permission) {
		actionErr.RequiredPermission = permission
	}
	if identity.ValidateActorID(request.Actor.ID) == nil {
		actionErr.ActorID = request.Actor.ID
	}
	if request.Scope.Validate() == nil {
		execution := request.Scope
		actionErr.Scope = &execution
	}
	if validRequestContextID(request.RequestID) {
		actionErr.RequestID = request.RequestID
	}
	return actionErr
}

func validRequestContextID(value string) bool {
	return value != "" && utf8.ValidString(value) && strings.TrimSpace(value) == value &&
		utf8.RuneCountInString(value) <= audit.MaxRequestIDRunes &&
		!strings.ContainsFunc(value, func(character rune) bool {
			return unicode.IsControl(character) || character == '\u2028' || character == '\u2029'
		})
}

func findActionError(err error) (actionErr *Error, ok bool) {
	return safeerr.Find[*Error](err)
}

func safeErrorIs(err, target error) (matched bool) {
	return safeerr.Is(err, target)
}
