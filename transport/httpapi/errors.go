package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/internal/safeerr"
	"github.com/iiwish/modary/scope"
)

func wrapDependencyError(operation string, cause error) error {
	if cause == nil {
		return nil
	}
	// Opaque must be applied before formatting so a caller-owned Error method is
	// never evaluated while constructing the transport diagnostic.
	return fmt.Errorf("%s failed: %w", operation, safeerr.Opaque(cause))
}

func classifyContextFailure(ctx context.Context, err error) (status int, message string, ok bool) {
	if isTypedNil(err) {
		return 0, "", false
	}
	if ctx != nil {
		switch ctx.Err() {
		case context.DeadlineExceeded:
			return http.StatusGatewayTimeout, "request timed out", true
		case context.Canceled:
			return http.StatusServiceUnavailable, "request was canceled", true
		}
	}
	switch {
	case safeerr.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "request timed out", true
	case safeerr.Is(err, context.Canceled):
		return http.StatusServiceUnavailable, "request was canceled", true
	case safeerr.Is(err, appkit.ErrApplicationUnavailable):
		return http.StatusServiceUnavailable, "application is unavailable", true
	default:
		return 0, "", false
	}
}

type errorEnvelope struct {
	Error     publicError `json:"error"`
	RequestID string      `json:"request_id"`
}

type publicError struct {
	Code               string           `json:"error_code"`
	Kind               action.ErrorKind `json:"error_kind"`
	Message            string           `json:"human_readable_reason"`
	ActionID           string           `json:"action_id,omitempty"`
	RequiredPermission string           `json:"required_permission,omitempty"`
	ActorID            string           `json:"actor_id,omitempty"`
	Scope              *scope.Execution `json:"scope,omitempty"`
	RequestID          string           `json:"request_id,omitempty"`
}

func writeActionError(writer http.ResponseWriter, requestID string, ctx context.Context, err error) {
	if isTypedNil(err) {
		writePublicError(writer, http.StatusInternalServerError, requestID, action.CodeInternal, "internal server error")
		return
	}
	if ctx != nil {
		switch ctx.Err() {
		case context.DeadlineExceeded:
			writePublicError(writer, http.StatusGatewayTimeout, requestID, action.CodeUnavailable, "request timed out")
			return
		case context.Canceled:
			writePublicError(writer, http.StatusServiceUnavailable, requestID, action.CodeUnavailable, "request was canceled")
			return
		}
	}
	public, ok := normalizedPublicActionError(err)
	if !ok {
		writePublicError(writer, http.StatusInternalServerError, requestID, action.CodeInternal, "internal server error")
		return
	}
	public.RequestID = requestID
	writeJSON(writer, actionErrorKindStatus(public.Kind), errorEnvelope{Error: public, RequestID: requestID})
}

func actionErrorStatus(code string) int {
	kind, ok := action.BuiltinErrorKind(code)
	if !ok {
		return http.StatusInternalServerError
	}
	return actionErrorKindStatus(kind)
}

func actionErrorKindStatus(kind action.ErrorKind) int {
	switch kind {
	case action.ErrorKindValidation:
		return http.StatusBadRequest
	case action.ErrorKindDenied:
		return http.StatusForbidden
	case action.ErrorKindNotFound:
		return http.StatusNotFound
	case action.ErrorKindPrecondition:
		return http.StatusPreconditionFailed
	case action.ErrorKindPreconditionRequired:
		return http.StatusPreconditionRequired
	case action.ErrorKindConflict:
		return http.StatusConflict
	case action.ErrorKindLimit:
		return http.StatusUnprocessableEntity
	case action.ErrorKindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func writePublicError(writer http.ResponseWriter, status int, requestID, code, message string) {
	kind, ok := action.BuiltinErrorKind(code)
	if !ok || !action.ValidErrorMessage(message) {
		code = action.CodeInternal
		kind = action.ErrorKindInternal
		message = "internal server error"
		status = http.StatusInternalServerError
	}
	writeJSON(writer, status, errorEnvelope{
		Error:     publicError{Code: code, Kind: kind, Message: message, RequestID: requestID},
		RequestID: requestID,
	})
}

func normalizedPublicActionError(err error) (publicError, bool) {
	actionErr, ok := safeerr.Find[*action.Error](err)
	if !ok || actionErr == nil || !action.ValidErrorMessage(actionErr.Message) {
		return publicError{}, false
	}
	kind := action.ErrorKindOf(actionErr)
	if kind == action.ErrorKindInternal {
		return publicError{}, false
	}
	if _, builtin := action.BuiltinErrorKind(actionErr.Code); !builtin && !action.ValidCustomErrorCode(actionErr.Code) {
		return publicError{}, false
	}
	return publicError{
		Code:               actionErr.Code,
		Kind:               kind,
		Message:            actionErr.Message,
		ActionID:           actionErr.ActionID,
		RequiredPermission: actionErr.RequiredPermission,
		ActorID:            actionErr.ActorID,
		Scope:              actionErr.Scope,
		RequestID:          actionErr.RequestID,
	}, true
}
