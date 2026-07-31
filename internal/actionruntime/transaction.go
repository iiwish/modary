package actionruntime

import (
	"context"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/internal/callbackcontract"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/internal/safeerr"
	"github.com/iiwish/modary/internal/transactionoutcome"
)

type transactionOperationError struct {
	cause          error
	invalidContext bool
	panicked       bool
}

type transactionOutcomeError struct{ cause error }

type transactionContractError struct {
	manager   error
	operation error
}

type transactionPanicEvidence struct{}

func (r *Engine) withinTransaction(ctx context.Context, operation func(context.Context) error) error {
	var guard callbackcontract.ExactlyOnce
	var operationFailure *transactionOperationError
	panicEvidence := &transactionPanicEvidence{}
	guardedOperation := func(transactionContext context.Context) error {
		return guard.Invoke(func() (operationErr error) {
			returned := false
			defer func() {
				if !returned {
					_ = recover()
					operationFailure = &transactionOperationError{
						cause:    panicEvidence,
						panicked: true,
					}
					operationErr = operationFailure
				}
			}()
			if isNilDependency(transactionContext) {
				operationFailure = &transactionOperationError{
					cause:          runtimecontrol.ErrTransactionManagerContract,
					invalidContext: true,
				}
				returned = true
				return operationFailure
			}
			operationErr = operation(transactionContext)
			if operationErr == nil {
				returned = true
				return nil
			}
			operationFailure = &transactionOperationError{cause: operationErr}
			returned = true
			return operationFailure
		})
	}

	managerErr, panicked := invokeTransactionManager(func() error {
		return r.tx.WithinTransaction(ctx, guardedOperation)
	})
	if panicked {
		managerErr = &action.CallbackPanicError{Operation: "run action transaction"}
	}
	proof, hasProof := transactionoutcome.Root(managerErr)
	snapshot := guard.CloseAndWait()

	switch snapshot.Outcome() {
	case callbackcontract.OutcomeNotInvoked:
		if managerErr == nil || hasProof {
			return newTransactionContractViolation(nil, nil)
		}
		return newTransactionManagerFailure(managerErr)
	case callbackcontract.OutcomeReturned:
		if operationFailure == nil {
			if snapshot.Result != nil {
				return newTransactionContractViolation(managerErr, snapshot.Result)
			}
			if managerErr == nil {
				return nil
			}
			if hasProof {
				return newTransactionAtomicityFailure(managerErr)
			}
			return newTransactionContractViolation(managerErr, nil)
		}
		if operationFailure.invalidContext ||
			!safeerr.Is(snapshot.Result, operationFailure) || panicked ||
			!hasProof || !safeerr.Is(proof.Operation, operationFailure) {
			return newTransactionContractViolation(managerErr, operationFailure)
		}
		switch proof.State {
		case transactionoutcome.StateRolledBack:
			if operationFailure.panicked {
				return &action.Error{
					Code:    action.CodeInternal,
					Message: "action transaction operation panicked",
					Cause:   &transactionOutcomeError{cause: managerErr},
				}
			}
			return newTransactionOperationFailure(operationFailure.cause, managerErr)
		case transactionoutcome.StateRollbackPending, transactionoutcome.StateRollbackFailed:
			return newTransactionAtomicityFailure(managerErr)
		default:
			return newTransactionContractViolation(managerErr, operationFailure)
		}
	case callbackcontract.OutcomePanicked:
		return newTransactionContractViolation(managerErr, operationFailure)
	default:
		return newTransactionContractViolation(managerErr, operationFailure)
	}
}

func newTransactionAtomicityFailure(cause error) error {
	return &action.Error{
		Code:    action.CodeInternal,
		Message: "action transaction did not complete atomically",
		Cause:   &transactionOutcomeError{cause: cause},
	}
}

func invokeTransactionManager(callback func() error) (err error, panicked bool) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			panicked = true
		}
	}()
	err = callback()
	returned = true
	return err, false
}

func newTransactionManagerFailure(cause error) error {
	return &action.Error{
		Code:    action.CodeInternal,
		Message: "action transaction manager failed",
		Cause:   &dependencyError{operation: "run action transaction", cause: cause},
	}
}

func newTransactionContractViolation(manager, operation error) error {
	return &action.Error{
		Code:    action.CodeInternal,
		Message: "transaction manager violated callback contract",
		Cause:   &transactionContractError{manager: manager, operation: operation},
	}
}

func newTransactionOperationFailure(operation, manager error) error {
	code := action.CodeInternal
	message := "action transaction operation failed"
	if operationErr, ok := operation.(*action.Error); ok && operationErr != nil {
		if operationErr.Code != "" {
			code = operationErr.Code
		}
		if operationErr.Message != "" {
			message = operationErr.Message
		}
	}
	return &action.Error{
		Code:    code,
		Kind:    action.ErrorKindOf(operation),
		Message: message,
		Cause:   &transactionOutcomeError{cause: manager},
	}
}

func (*transactionOperationError) Error() string { return "action transaction operation failed" }

func (err *transactionOperationError) Unwrap() error {
	if err == nil {
		return nil
	}
	return safeerr.Opaque(err.cause)
}

func (*transactionOutcomeError) Error() string { return "action transaction operation failed" }

func (err *transactionOutcomeError) Unwrap() error {
	if err == nil {
		return nil
	}
	return safeerr.Opaque(err.cause)
}

func (*transactionContractError) Error() string {
	return "action transaction manager contract violation"
}

func (err *transactionContractError) Unwrap() []error {
	if err == nil {
		return nil
	}
	causes := []error{runtimecontrol.ErrTransactionManagerContract}
	if err.manager != nil {
		causes = append(causes, safeerr.Opaque(err.manager))
	}
	if err.operation != nil {
		causes = append(causes, safeerr.Opaque(err.operation))
	}
	return causes
}

func (*transactionPanicEvidence) Error() string {
	return "action transaction operation panicked"
}

func (*transactionPanicEvidence) Unwrap() error { return action.ErrCallbackPanic }
