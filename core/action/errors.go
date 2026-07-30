package action

import (
	"errors"
	"fmt"
)

const (
	CodeActionNotFound      = "ACTION_NOT_FOUND"
	CodeValidationFailed    = "VALIDATION_FAILED"
	CodeAuthzDenied         = "AUTHZ_DENIED"
	CodePreconditionFailed  = "PRECONDITION_FAILED"
	CodePlanRequired        = "PLAN_REQUIRED"
	CodePlanNotFound        = "PLAN_NOT_FOUND"
	CodePlanStale           = "PLAN_STALE"
	CodeLimitExceeded       = "LIMIT_EXCEEDED"
	CodeIdempotencyRequired = "IDEMPOTENCY_REQUIRED"
	CodeIdempotencyConflict = "IDEMPOTENCY_CONFLICT"
	CodeIdempotencyProgress = "IDEMPOTENCY_IN_PROGRESS"
	CodeInternal            = "INTERNAL_ERROR"
)

type Error struct {
	Code               string `json:"error_code"`
	Message            string `json:"human_readable_reason"`
	ActionID           string `json:"action_id,omitempty"`
	RequiredPermission string `json:"required_permission,omitempty"`
	ActorID            string `json:"actor_id,omitempty"`
	WorkspaceID        string `json:"workspace_id,omitempty"`
	RequestID          string `json:"request_id,omitempty"`
	Cause              error  `json:"-"`
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func ErrorCode(err error) string {
	var actionErr *Error
	if errors.As(err, &actionErr) {
		return actionErr.Code
	}
	return CodeInternal
}

func WithRequest(err error, request Request, permission string) error {
	var actionErr *Error
	if !errors.As(err, &actionErr) {
		actionErr = &Error{Code: CodeInternal, Message: "action execution failed", Cause: err}
	} else {
		clone := *actionErr
		actionErr = &clone
	}
	actionErr.ActionID = request.ActionID
	actionErr.RequiredPermission = permission
	actionErr.ActorID = request.Actor.ID
	actionErr.WorkspaceID = request.WorkspaceID
	actionErr.RequestID = request.RequestID
	return actionErr
}
